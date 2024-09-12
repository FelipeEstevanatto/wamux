package webhook_producer

import (
	"bytes"
	"net/http"
	"strings"

	producer_interfaces "github.com/Zapbox-API/evolution-go/pkg/events/interfaces"
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
		go sendWebhook(p.url, payload)
	}
	if webhookUrl != "" {
		go sendWebhook(webhookUrl, payload)
	}

	return nil
}

func sendWebhook(url string, body []byte) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return
	}

	logger.LogInfo("webhook sent", "url", url, "status", resp.Status)

	defer resp.Body.Close()
}
