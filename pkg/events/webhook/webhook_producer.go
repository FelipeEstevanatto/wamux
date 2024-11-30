package webhook_producer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	producer_interfaces "github.com/EvolutionAPI/evolution-go/pkg/events/interfaces"
	"github.com/gomessguii/logger"
)

type webhookProducer struct {
	url string
}

func NewWebhookProducer(
	url string,
) producer_interfaces.Producer {
	return &webhookProducer{
		url: url,
	}
}

func (p *webhookProducer) Produce(
	queueName string,
	payload []byte,
	webhookUrl string,
	userID string,
) error {
	splitQueue := strings.Split(queueName, ".")

	if len(splitQueue) < 2 {
		return nil
	}

	if p.url != "" {
		go sendWebhookWithRetry(p.url, payload, 5, 30*time.Second, userID)
	}
	if webhookUrl != "" {
		go sendWebhookWithRetry(webhookUrl, payload, 5, 30*time.Second, userID)
	}

	return nil
}

func sendWebhookWithRetry(url string, body []byte, maxRetries int, retryInterval time.Duration, userID string) {
	for i := 0; i < maxRetries; i++ {
		err := sendWebhook(url, body, userID)
		if err == nil {
			logger.LogInfo("[%s] webhook sent successfully", userID, "url", url)
			return
		}
		logger.LogWarn("[%s] webhook failed", userID, "url", url, "attempt", i+1, "error", err)

		time.Sleep(retryInterval)
	}
	logger.LogError("[%s] webhook failed after maximum retries", userID, "url", url)
}

func sendWebhook(url string, body []byte, userID string) error {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("erro ao ler resposta: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("received non-2xx response: " + resp.Status)
	}

	logger.LogInfo("[%s] webhook sent", userID, "url", url, "status", resp.Status, "response", string(responseBody))
	return nil
}
