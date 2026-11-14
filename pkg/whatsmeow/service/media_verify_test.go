package whatsmeow_service

import (
	"crypto/sha256"
	"testing"
)

func TestVerifyDownloadedMedia(t *testing.T) {
	data := []byte("exact bytes")
	sum := sha256.Sum256(data)

	if err := verifyDownloadedMedia(data, int64(len(data)), sum[:]); err != nil {
		t.Fatalf("valid media rejected: %v", err)
	}
	if err := verifyDownloadedMedia(data, int64(len(data)+1), sum[:]); err == nil {
		t.Fatal("size mismatch accepted")
	}
	bad := sum
	bad[0] ^= 0xff
	if err := verifyDownloadedMedia(data, int64(len(data)), bad[:]); err == nil {
		t.Fatal("sha mismatch accepted")
	}
	// Missing metadata must not reject otherwise valid bytes.
	if err := verifyDownloadedMedia(data, 0, nil); err != nil {
		t.Fatalf("missing metadata rejected: %v", err)
	}
}
