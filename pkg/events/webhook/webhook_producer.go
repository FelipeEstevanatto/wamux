package webhook_producer

import (
	"bytes"
	"errors"
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
) error {
	splitQueue := strings.Split(queueName, ".")

	if len(splitQueue) < 2 {
		return nil
	}

	if p.url != "" {
		go sendWebhookWithRetry(p.url, payload, 5, 30*time.Second)
	}
	if webhookUrl != "" {
		go sendWebhookWithRetry(webhookUrl, payload, 5, 30*time.Second)
	}

	return nil
}

func sendWebhookWithRetry(url string, body []byte, maxRetries int, retryInterval time.Duration) {
	for i := 0; i < maxRetries; i++ {
		err := sendWebhook(url, body)
		if err == nil {
			logger.LogInfo("webhook sent successfully", "url", url)
			return
		}
		logger.LogWarn("webhook failed", "url", url, "attempt", i+1, "error", err)

		time.Sleep(retryInterval)
	}
	logger.LogError("webhook failed after maximum retries", "url", url)
}

func sendWebhook(url string, body []byte) error {
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("received non-2xx response: " + resp.Status)
	}

	logger.LogInfo("webhook sent", "url", url, "status", resp.Status)
	return nil
}
