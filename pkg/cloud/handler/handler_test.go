package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	cloud_model "github.com/felipeestevanatto/wamux/pkg/cloud/model"
	"github.com/felipeestevanatto/wamux/pkg/config"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testCloudHandler(t *testing.T) (*Handler, *config.Config) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/h.db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(&cloud_model.App{}, &cloud_model.Credentials{}, &cloud_model.Contact{},
		&cloud_model.Template{}, &cloud_model.ChatWindow{}, &cloud_model.WebhookEvent{},
		&cloud_model.MediaJob{}, &cloud_model.StatusEvent{}, &cloud_model.AccountEvent{},
		&cloud_model.UserPreference{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := &config.Config{
		DataEncryptionKey:    []byte("01234567890123456789012345678901"),
		CloudAPIGraphVersion: "v27.0", CloudAPIMaxWebhookBytes: 4 << 20,
	}
	svc := cloud_service.NewService(cloud_service.NewRepository(db), nil, cfg, nil, nil, nil, nil, nil)
	if err := svc.SaveApp("app-1", "secret", "verify", "v27.0"); err != nil {
		t.Fatalf("app: %v", err)
	}
	return New(svc, cfg), cfg
}

func TestWebhookVerifyAndSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := testCloudHandler(t)
	r := gin.New()
	r.GET("/webhook/cloud", h.WebhookVerify)
	r.POST("/webhook/cloud", h.WebhookReceive)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/webhook/cloud?hub.mode=subscribe&hub.verify_token=verify&hub.challenge=abc", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "abc" {
		t.Fatalf("verify = %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/webhook/cloud?hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=abc", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("bad verify = %d", w.Code)
	}

	body := []byte(`{"object":"whatsapp_business_account","entry":[]}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	req = httptest.NewRequest(http.MethodPost, "/webhook/cloud", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("signed post = %d %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/webhook/cloud", bytes.NewReader(body))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned post = %d", w.Code)
	}
}
