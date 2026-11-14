// Command wamux-replay-webhook replays messages from the webhook dead-letter
// queue. Dry-run is the default: it prints a plan and leaves the queue intact.
// Apply requires the exact plan hash printed by the dry run.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/felipeestevanatto/wamux/pkg/webhooksign"
)

type envelope struct {
	URL     string          `json:"url"`
	UserID  string          `json:"userID"`
	Payload json.RawMessage `json:"payload"`
}

type planItem struct {
	URL    string `json:"url"`
	UserID string `json:"userID"`
	Bytes  int    `json:"bytes"`
	Error  string `json:"error,omitempty"`
}

func main() {
	amqpURL := flag.String("amqp", os.Getenv("AMQP_URL"), "RabbitMQ URL (default AMQP_URL)")
	queue := flag.String("queue", "webhook_errors", "dead-letter queue name")
	limit := flag.Int("limit", 100, "maximum messages to inspect")
	apply := flag.Bool("apply", false, "replay the messages (default is a dry run)")
	confirm := flag.String("confirm", "", "plan hash printed by the dry run; required with -apply")
	hmacKey := flag.String("hmac-key", "", "optional global HMAC key for x-hmac-signature")
	timeout := flag.Duration("timeout", 30*time.Second, "per-delivery HTTP timeout")
	flag.Parse()

	if strings.TrimSpace(*amqpURL) == "" {
		fail(2, "-amqp or AMQP_URL is required")
	}
	if *limit <= 0 || *limit > 10000 {
		fail(2, "-limit must be between 1 and 10000")
	}

	conn, err := amqp.Dial(*amqpURL)
	if err != nil {
		fail(1, "connect: %v", err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		fail(1, "channel: %v", err)
	}
	defer ch.Close()

	if _, err := ch.QueueInspect(*queue); err != nil {
		fail(1, "queue %q is not available: %v", *queue, err)
	}

	deliveries := make([]amqp.Delivery, 0, *limit)
	for len(deliveries) < *limit {
		d, ok, err := ch.Get(*queue, false)
		if err != nil {
			nackAll(deliveries)
			fail(1, "get: %v", err)
		}
		if !ok {
			break
		}
		deliveries = append(deliveries, d)
	}

	hash := planHash(deliveries)
	items := make([]planItem, 0, len(deliveries))
	for _, d := range deliveries {
		item := planItem{Bytes: len(d.Body)}
		env, payload, err := parseEnvelope(d.Body)
		if err != nil {
			item.Error = err.Error()
		} else {
			item.URL, item.UserID = env.URL, env.UserID
			item.Bytes = len(payload)
		}
		items = append(items, item)
	}

	output := map[string]any{
		"mode":     "dry-run",
		"queue":    *queue,
		"count":    len(deliveries),
		"planHash": hash,
		"items":    items,
	}
	if !*apply {
		printJSON(output)
		nackAll(deliveries)
		return
	}
	if *confirm != hash {
		nackAll(deliveries)
		output["mode"] = "apply"
		output["refused"] = "plan hash mismatch: run the dry run again and pass its planHash"
		printJSON(output)
		os.Exit(2)
	}

	client := &http.Client{Timeout: *timeout}
	acked, failed := 0, 0
	for i, d := range deliveries {
		env, payload, err := parseEnvelope(d.Body)
		if err == nil {
			err = deliver(context.Background(), client, env.URL, payload, *hmacKey)
		}
		if err != nil {
			failed++
			items[i].Error = err.Error()
			_ = d.Nack(false, true)
			continue
		}
		acked++
		_ = d.Ack(false)
	}
	printJSON(map[string]any{
		"mode":     "apply",
		"queue":    *queue,
		"planHash": hash,
		"acked":    acked,
		"failed":   failed,
		"items":    items,
	})
	if failed > 0 {
		os.Exit(1)
	}
}

func parseEnvelope(body []byte) (envelope, []byte, error) {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return env, nil, fmt.Errorf("invalid dead-letter envelope: %w", err)
	}
	parsed, err := url.Parse(env.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return env, nil, errors.New("invalid webhook url")
	}
	payload := []byte(env.Payload)
	if len(payload) == 0 {
		return env, nil, errors.New("empty webhook payload")
	}
	// A non-JSON original is stored as a JSON string; turn it back into bytes.
	if payload[0] == '"' {
		var s string
		if err := json.Unmarshal(payload, &s); err == nil {
			payload = []byte(s)
		}
	}
	return env, payload, nil
}

func deliver(ctx context.Context, client *http.Client, target string, payload []byte, hmacKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "WaMux-Webhook-Replay/1.0")
	if hmacKey != "" {
		req.Header.Set(webhooksign.SignatureHeader, webhooksign.Sign([]byte(hmacKey), payload))
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	return nil
}

// planHash hashes the sorted per-message SHA-256 values, so the same set of
// messages produces the same plan even if RabbitMQ returns them in another
// order.
func planHash(deliveries []amqp.Delivery) string {
	hashes := make([]string, 0, len(deliveries))
	for _, d := range deliveries {
		sum := sha256.Sum256(d.Body)
		hashes = append(hashes, hex.EncodeToString(sum[:]))
	}
	sort.Strings(hashes)
	sum := sha256.Sum256([]byte(strings.Join(hashes, "\n")))
	return hex.EncodeToString(sum[:])
}

func nackAll(deliveries []amqp.Delivery) {
	for _, d := range deliveries {
		_ = d.Nack(false, true)
	}
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func fail(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "wamux-replay-webhook: "+format+"\n", args...)
	os.Exit(code)
}
