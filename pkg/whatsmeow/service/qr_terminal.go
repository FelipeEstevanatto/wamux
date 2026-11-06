package whatsmeow_service

import (
	"fmt"
	"io"
	"os"

	"github.com/skip2/go-qrcode"
)

// qrTerminalOutput is where the process prints a QR, for `docker logs`.
var qrTerminalOutput io.Writer = os.Stdout

// renderQR draws a code with half-height blocks, so it fits a terminal. The blocks are
// the light modules, which is what a dark terminal needs to be scanned. Low correction
// keeps it small; a screen has no damage to correct.
func renderQR(code string) (string, error) {
	qr, err := qrcode.New(code, qrcode.Low)
	if err != nil {
		return "", err
	}
	return qr.ToSmallString(false), nil
}

// logQRCode shows a pairing code in the instance log and on the terminal. A code
// expires within a minute and only pairs while this instance is waiting for it.
func (mycli *MyClient) logQRCode(code string) {
	if !mycli.config.QrcodeTerminal {
		return
	}
	logger := mycli.loggerWrapper.GetLogger(mycli.userID)
	art, err := renderQR(code)
	if err != nil {
		logger.LogWarn("[%s] Failed to draw the QR code: %v", mycli.userID, err)
		return
	}

	title := fmt.Sprintf("Instance %s, QR code #%d", mycli.Instance.Name, mycli.qrcodeCount)
	logger.LogInfo("[%s] %s\n\n%s", mycli.userID, title, art)
	// JSON logs are read by machines, and a drawing would break the stream.
	if mycli.config.LogType != "json" {
		_, _ = fmt.Fprintf(qrTerminalOutput, "%s\n%s\n", title, art)
	}
}
