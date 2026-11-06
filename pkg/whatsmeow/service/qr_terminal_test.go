package whatsmeow_service

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felipeestevanatto/wamux/pkg/config"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow/types/events"
)

// A pairing code is about this long.
var testPairingCode = "2@" + strings.Repeat("abcdefghij", 16)

func captureQROutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var out bytes.Buffer
	previous := qrTerminalOutput
	qrTerminalOutput = &out
	t.Cleanup(func() { qrTerminalOutput = previous })
	return &out
}

// modules reads a drawing back: a dark module is the absence of a block.
func modules(t *testing.T, art string) [][]bool {
	t.Helper()
	var grid [][]bool
	for _, line := range strings.Split(strings.TrimRight(art, "\n"), "\n") {
		top, bottom := []bool{}, []bool{}
		for _, r := range line {
			switch r {
			case ' ':
				top, bottom = append(top, true), append(bottom, true)
			case '█':
				top, bottom = append(top, false), append(bottom, false)
			case '▀':
				top, bottom = append(top, false), append(bottom, true)
			case '▄':
				top, bottom = append(top, true), append(bottom, false)
			default:
				t.Fatalf("unexpected %q", r)
			}
		}
		grid = append(grid, top, bottom)
	}
	return grid
}

func TestRenderQRMatchesTheCode(t *testing.T) {
	art, err := renderQR(testPairingCode)
	if err != nil {
		t.Fatal(err)
	}
	qr, err := qrcode.New(testPairingCode, qrcode.Low)
	if err != nil {
		t.Fatal(err)
	}

	want, got := qr.Bitmap(), modules(t, art)
	for y := range want {
		for x := range want[y] {
			if got[y][x] != want[y][x] {
				t.Fatalf("module %d,%d differs", x, y)
			}
		}
	}
	if width := len(want); width > 80 {
		t.Errorf("%d columns do not fit a terminal", width)
	}
}

func TestQRReachesTheTerminalAndTheInstanceLog(t *testing.T) {
	out := captureQROutput(t)
	h := newEventHarness(t)
	h.cli.WAClient.Store.ID = nil
	h.cli.config.QrcodeTerminal = true
	h.cli.qrcodeCount = 2
	logs := t.TempDir()
	h.cli.loggerWrapper = logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: logs})

	h.cli.logQRCode(testPairingCode)

	art, _ := renderQR(testPairingCode)
	if !strings.Contains(out.String(), art) || !strings.Contains(out.String(), "Instance test, QR code #2") {
		t.Fatalf("terminal: %q", out.String())
	}

	logger := h.cli.loggerWrapper.GetLogger(testInstanceID)
	logger.Flush()
	t.Cleanup(func() { _ = logger.Close() })
	logged, err := os.ReadFile(filepath.Join(logs, testInstanceID, "instance.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "QR code #2") || !strings.Contains(string(logged), "█") {
		t.Fatalf("instance log: %s", logged)
	}
}

func TestQRIsDrawnWhenThePairingCodeArrives(t *testing.T) {
	out := captureQROutput(t)
	h := newEventHarness(t)
	h.cli.WAClient.Store.ID = nil
	h.cli.config.QrcodeTerminal = true
	h.cli.config.QrcodeMaxCount = 2

	h.cli.processEvent(&events.QR{Codes: []string{testPairingCode}})
	h.deliveries(t, 1)

	if !strings.Contains(out.String(), "QR code #1") {
		t.Fatalf("terminal: %q", out.String())
	}
}

func TestQRStaysOffTheTerminal(t *testing.T) {
	for name, set := range map[string]func(h *eventHarness){
		"turned off": func(h *eventHarness) {},
		"json logs": func(h *eventHarness) {
			h.cli.config.QrcodeTerminal = true
			h.cli.config.LogType = "json"
		},
	} {
		t.Run(name, func(t *testing.T) {
			out := captureQROutput(t)
			h := newEventHarness(t)
			set(h)

			h.cli.logQRCode(testPairingCode)

			if out.Len() != 0 {
				t.Fatalf("printed %q", out.String())
			}
		})
	}
}
