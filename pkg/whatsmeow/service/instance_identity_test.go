package whatsmeow_service

import (
	"testing"

	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
)

func TestEventIdentityCanOmitToken(t *testing.T) {
	mycli := &MyClient{
		config:   &config.Config{WebhookIncludeToken: false},
		token:    "instance-secret",
		userID:   "instance-id",
		Instance: &instance_model.Instance{Name: "sales"},
	}
	payload := map[string]interface{}{"event": "Message"}
	mycli.addInstanceIdentity(payload)

	if _, found := payload["instanceToken"]; found {
		t.Fatal("instanceToken was included with WebhookIncludeToken=false")
	}
	if payload["instanceId"] != "instance-id" || payload["instanceName"] != "sales" {
		t.Fatalf("identity fields = %+v", payload)
	}

	mycli.config.WebhookIncludeToken = true
	mycli.addInstanceIdentity(payload)
	if payload["instanceToken"] != "instance-secret" {
		t.Fatalf("instanceToken = %v", payload["instanceToken"])
	}
}
