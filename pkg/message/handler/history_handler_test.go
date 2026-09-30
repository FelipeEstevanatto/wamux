package message_handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
	message_service "github.com/evolution-foundation/evolution-go/pkg/message/service"
)

// stubMessageService records the calls the history handlers make. The embedded
// interface satisfies the rest of MessageService.
type stubMessageService struct {
	message_service.MessageService
	history  *message_service.HistoryQuery
	messages []message_model.Message
	chats    []message_repository.ChatSummary
	limit    int
}

func (s *stubMessageService) GetHistory(q *message_service.HistoryQuery, _ *instance_model.Instance) ([]message_model.Message, error) {
	s.history = q
	return s.messages, nil
}

func (s *stubMessageService) ListChats(_ *instance_model.Instance, limit int) ([]message_repository.ChatSummary, error) {
	s.limit = limit
	return s.chats, nil
}

func newMessageRouter(svc message_service.MessageService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("instance", &instance_model.Instance{Id: "inst-1"})
	})
	h := NewMessageHandler(svc)
	r.GET("/chat/history", h.GetHistory)
	r.GET("/chat/chats", h.ListChats)
	return r
}

func doGet(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGetHistoryBindsQueryAndReturnsMessages(t *testing.T) {
	svc := &stubMessageService{messages: []message_model.Message{{MessageID: "m1", TextContent: "hi"}}}
	r := newMessageRouter(svc)

	w := doGet(t, r, "/chat/history?chat=5511999999999&limit=10&before=2026-05-09%2010:00:00")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if svc.history == nil || svc.history.Chat != "5511999999999" || svc.history.Limit != 10 || svc.history.Before != "2026-05-09 10:00:00" {
		t.Fatalf("history query = %+v", svc.history)
	}

	var body struct {
		Message string                  `json:"message"`
		Data    []message_model.Message `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].MessageID != "m1" {
		t.Fatalf("response data = %+v", body.Data)
	}
}

func TestGetHistoryRequiresChat(t *testing.T) {
	svc := &stubMessageService{}
	r := newMessageRouter(svc)

	w := doGet(t, r, "/chat/history")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if svc.history != nil {
		t.Fatal("service must not be called without a chat")
	}
}

func TestListChatsReturnsConversations(t *testing.T) {
	svc := &stubMessageService{chats: []message_repository.ChatSummary{{ChatJid: "a@s.whatsapp.net", MessageCount: 3}}}
	r := newMessageRouter(svc)

	w := doGet(t, r, "/chat/chats?limit=25")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if svc.limit != 25 {
		t.Fatalf("limit = %d, want 25", svc.limit)
	}
	if !json.Valid(w.Body.Bytes()) {
		t.Fatalf("invalid JSON: %s", w.Body.String())
	}
}
