package instance_service

import (
	"errors"
	"testing"

	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/felipeestevanatto/wamux/pkg/safemap"
	whatsmeow_service "github.com/felipeestevanatto/wamux/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
)

type qrStubWhatsmeow struct {
	whatsmeow_service.WhatsmeowService
	startCalls int
	startErr   error
}

func (s *qrStubWhatsmeow) StartInstance(string) error {
	s.startCalls++
	return s.startErr
}

func newQRService(t *testing.T, wm whatsmeow_service.WhatsmeowService) instances {
	t.Helper()
	return instances{
		clientPointer:    safemap.New[*whatsmeow.Client](),
		whatsmeowService: wm,
		loggerWrapper:    logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()}),
	}
}

// A paired instance has nothing to scan: the old code answered "no QR code
// available" (a 400) while it tried to start a second client for an existing
// device, which made WhatsApp replace the first one. It must instead report the
// paired state and only resume the session once.
func TestGetQrPairedInstanceReportsStateAndResumesOnce(t *testing.T) {
	wm := &qrStubWhatsmeow{}
	svc := newQRService(t, wm)

	got, err := svc.GetQr(&instance_model.Instance{
		Id:  "inst-1",
		Jid: "5511999999999:11@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("GetQr: %v", err)
	}
	if !got.Paired {
		t.Fatal("Paired = false, want true")
	}
	if got.Connected {
		t.Fatal("Connected = true for an instance with no runtime client")
	}
	if got.Qrcode != "" {
		t.Fatalf("Qrcode = %q, want empty for a paired instance", got.Qrcode)
	}
	if wm.startCalls != 1 {
		t.Fatalf("StartInstance calls = %d, want 1 (resume the session only)", wm.startCalls)
	}
}

func TestGetQrPairedInstanceSurfacesResumeFailure(t *testing.T) {
	wm := &qrStubWhatsmeow{startErr: errors.New("boom")}
	svc := newQRService(t, wm)

	if _, err := svc.GetQr(&instance_model.Instance{
		Id:  "inst-1",
		Jid: "5511999999999:11@s.whatsapp.net",
	}); err == nil {
		t.Fatal("expected the StartInstance failure to be reported")
	}
}
