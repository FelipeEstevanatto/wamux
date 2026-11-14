package idempotency

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	"github.com/gin-gonic/gin"
)

type memStore struct {
	mu  sync.Mutex
	ops map[string]*message_model.SendOperation
}

func newMemStore() *memStore {
	return &memStore{ops: map[string]*message_model.SendOperation{}}
}

func (s *memStore) BeginSendOperation(_ context.Context, instanceId, key, hash string) (*message_model.SendOperation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := instanceId + ":" + key
	if op, ok := s.ops[id]; ok {
		return op, false, nil
	}
	op := &message_model.SendOperation{
		Id: id, InstanceId: instanceId, IdempotencyKey: key, PayloadHash: hash,
		State: "pending", PlannedMessageID: "planned-" + key,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.ops[id] = op
	return op, true, nil
}

func (s *memStore) CompleteSendOperation(_ context.Context, id, state string, status int, response []byte, messageID, errText string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, op := range s.ops {
		if op.Id == id {
			op.State, op.StatusCode, op.Response, op.MessageID, op.Error = state, status, response, messageID, errText
			return nil
		}
	}
	return nil
}

func (s *memStore) GetMessageByIDForInstance(_, _ string) (*message_model.Message, error) {
	return nil, nil
}

func (s *memStore) ListSendOperations(_ context.Context, instanceId, state string, limit int) ([]message_model.SendOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]message_model.SendOperation, 0, len(s.ops))
	for _, op := range s.ops {
		if op.InstanceId != instanceId || (state != "" && op.State != state) {
			continue
		}
		out = append(out, *op)
		if len(out) >= limit && limit > 0 {
			break
		}
	}
	return out, nil
}

func (s *memStore) GetSendOperation(_ context.Context, instanceId, id string) (*message_model.SendOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, op := range s.ops {
		if op.Id == id && op.InstanceId == instanceId {
			return op, nil
		}
	}
	return nil, nil
}

func newRouter(store Store, calls *int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("instance", &instance_model.Instance{Id: "inst-1"})
	})
	h := New(store, nil)
	r.POST("/send/text", h.Middleware(), func(c *gin.Context) {
		*calls++
		c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"Info": gin.H{"ID": "msg-1"}}})
	})
	r.GET("/send/operations", h.Operations)
	r.GET("/send/operations/:operationId", h.Operation)
	return r
}

func post(r *gin.Engine, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/send/text", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(KeyHeader, key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestReplaysSameKeyWithoutSendingAgain(t *testing.T) {
	store := newMemStore()
	calls := 0
	r := newRouter(store, &calls)

	first := post(r, "abc", `{"number":"5511","text":"hi"}`)
	second := post(r, "abc", `{"number":"5511","text":"hi"}`)

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("statuses = %d/%d", first.Code, second.Code)
	}
	if calls != 1 {
		t.Fatalf("handler ran %d times, want 1", calls)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("replay body differs:\n%s\n%s", first.Body, second.Body)
	}
	if first.Header().Get(OperationHeader) == "" {
		t.Fatal("first response has no operation id")
	}
}

func TestReplaysSemanticallyEqualJSON(t *testing.T) {
	store := newMemStore()
	calls := 0
	r := newRouter(store, &calls)

	first := post(r, "abc", `{"number":"5511","text":"hi"}`)
	// Same JSON meaning, different key order and whitespace. A retry through
	// an SDK or proxy must replay, not conflict.
	second := post(r, "abc", `{ "text": "hi", "number": "5511" }`)

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("statuses = %d/%d, want 200/200", first.Code, second.Code)
	}
	if calls != 1 {
		t.Fatalf("handler ran %d times, want 1", calls)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("replay body differs:\n%s\n%s", first.Body, second.Body)
	}
}

func TestSameKeyDifferentPayloadConflicts(t *testing.T) {
	store := newMemStore()
	calls := 0
	r := newRouter(store, &calls)

	if w := post(r, "abc", `{"text":"one"}`); w.Code != http.StatusOK {
		t.Fatalf("first status = %d", w.Code)
	}
	w := post(r, "abc", `{"text":"two"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("second status = %d, want 409", w.Code)
	}
	if calls != 1 {
		t.Fatalf("handler ran %d times, want 1", calls)
	}
}

func TestPendingOperationReturns202(t *testing.T) {
	store := newMemStore()
	hash := payloadHash(http.MethodPost, "/send/text", []byte(`{"text":"one"}`))
	store.ops["inst-1:pending"] = &message_model.SendOperation{
		Id: "op-1", InstanceId: "inst-1", IdempotencyKey: "pending",
		PayloadHash: hash, State: "pending",
	}
	calls := 0
	r := newRouter(store, &calls)

	w := post(r, "pending", `{"text":"one"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if calls != 0 {
		t.Fatalf("handler ran %d times, want 0", calls)
	}
}

func TestMiddlewareInjectsPlannedMessageID(t *testing.T) {
	store := newMemStore()
	seen := ""
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("instance", &instance_model.Instance{Id: "inst-1"})
	})
	h := New(store, nil)
	r.POST("/send/text", h.Middleware(), func(c *gin.Context) {
		var body map[string]any
		_ = c.ShouldBindJSON(&body)
		seen, _ = body["id"].(string)
		c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"Info": gin.H{"ID": seen}}})
	})

	if w := post(r, "abc", `{"text":"one"}`); w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body)
	}
	if seen != "planned-abc" {
		t.Fatalf("handler saw id %q, want planned-abc", seen)
	}
}

func TestNoKeyDoesNotTouchStore(t *testing.T) {
	store := newMemStore()
	calls := 0
	r := newRouter(store, &calls)

	if w := post(r, "", `{"text":"one"}`); w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if calls != 1 {
		t.Fatalf("handler ran %d times, want 1", calls)
	}
	if len(store.ops) != 0 {
		t.Fatalf("store got %d rows without a key", len(store.ops))
	}
}

// A pending operation whose message is already in history must become sent,
// not stay pending forever after a crash.
type reconcilingStore struct {
	*memStore
	message *message_model.Message
}

func (s *reconcilingStore) GetMessageByIDForInstance(_, _ string) (*message_model.Message, error) {
	return s.message, nil
}

func TestPendingOperationReconcilesFromStoredMessage(t *testing.T) {
	base := newMemStore()
	base.ops["inst-1:op"] = &message_model.SendOperation{
		Id: "op", InstanceId: "inst-1", IdempotencyKey: "op",
		State: "pending", MessageID: "planned-1",
	}
	store := &reconcilingStore{
		memStore: base,
		message:  &message_model.Message{MessageID: "planned-1", InstanceId: "inst-1"},
	}
	r := newRouter(store, new(int))

	req := httptest.NewRequest(http.MethodGet, "/send/operations/op", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body)
	}
	var body struct {
		Data struct {
			State string `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.State != "sent" {
		t.Fatalf("state = %q, want sent after reconciliation", body.Data.State)
	}
}

func TestOperationsListFiltersByState(t *testing.T) {
	store := newMemStore()
	calls := 0
	r := newRouter(store, &calls)
	post(r, "abc", `{"text":"one"}`)

	req := httptest.NewRequest(http.MethodGet, "/send/operations?state=sent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body)
	}
	var body struct {
		Data []message_model.SendOperation `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].State != "sent" {
		t.Fatalf("operations = %+v", body.Data)
	}
}

func TestOperationEndpoint(t *testing.T) {
	store := newMemStore()
	calls := 0
	r := newRouter(store, &calls)
	first := post(r, "abc", `{"text":"one"}`)
	id := first.Header().Get(OperationHeader)

	req := httptest.NewRequest(http.MethodGet, "/send/operations/"+id, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body)
	}
	var body struct {
		Data struct {
			State     string `json:"state"`
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.State != "sent" || body.Data.MessageID != "msg-1" {
		t.Fatalf("operation = %+v", body.Data)
	}
}
