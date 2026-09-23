package server_handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"github.com/gin-gonic/gin"
)

type fakeOverview struct {
	ov    *whatsmeow_service.InstanceOverview
	err   error
	names map[string]string
}

func (f fakeOverview) GetInstanceOverview(string) (*whatsmeow_service.InstanceOverview, error) {
	return f.ov, f.err
}

func (f fakeOverview) ResolveChatNames(users []string) map[string]string {
	out := make(map[string]string, len(users))
	for _, u := range users {
		if n, ok := f.names[u]; ok {
			out[u] = n
		}
	}
	return out
}

// fakeMessageRepo implements just enough of MessageRepository for the handler.
type fakeMessageRepo struct {
	byInstance int64
	chats      int64
	stats      *message_repository.MessageStats
}

func (fakeMessageRepo) InsertMessage(message_model.Message) error             { return nil }
func (fakeMessageRepo) GetMessageByID(string) (*message_model.Message, error) { return nil, nil }
func (fakeMessageRepo) DeleteAllMessages() (int64, error)                     { return 0, nil }
func (fakeMessageRepo) GetLatestMessageID(string) (string, string, error)     { return "", "", nil }
func (f fakeMessageRepo) GetStats() (*message_repository.MessageStats, error) { return f.stats, nil }
func (f fakeMessageRepo) CountByInstance(string) (int64, error)               { return f.byInstance, nil }
func (f fakeMessageRepo) CountChatsByInstance(string) (int64, error)          { return f.chats, nil }

func TestInstanceOverviewHandlerReturnsProviderData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{
		overview: fakeOverview{ov: &whatsmeow_service.InstanceOverview{
			Connected:     true,
			ProfileName:   "Numero CLARO",
			ProfilePicURL: "https://pps.whatsapp.net/x.jpg",
			ContactsCount: 346,
		}},
		messageRepo: fakeMessageRepo{byInstance: 42, chats: 9},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "instanceId", Value: "abc"}}
	h.InstanceOverview(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var body struct {
		Data whatsmeow_service.InstanceOverview `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Data.ContactsCount != 346 || !body.Data.Connected || body.Data.ProfilePicURL == "" {
		t.Fatalf("unexpected payload: %+v", body.Data)
	}
	if body.Data.MessagesCount != 42 {
		t.Fatalf("messagesCount = %d, want 42", body.Data.MessagesCount)
	}
	if body.Data.ChatsCount != 9 {
		t.Fatalf("chatsCount = %d, want 9", body.Data.ChatsCount)
	}
}

func TestInstanceOverviewHandlerValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Missing instanceId -> 400.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	(&serverHandler{}).InstanceOverview(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing id: status = %d, want 400", w.Code)
	}

	// No provider wired -> 503.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "instanceId", Value: "abc"}}
	(&serverHandler{}).InstanceOverview(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("no provider: status = %d, want 503", w.Code)
	}
}

func TestStatsIncludesVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{version: "1.2.3-fork", startTime: time.Now()}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.Stats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		System map[string]any `json:"system"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.System["version"] != "1.2.3-fork" {
		t.Fatalf("system.version = %v, want 1.2.3-fork", body.System["version"])
	}
}

func TestStatsResolvesTopSourceNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{
		version:   "1.2.3-fork",
		startTime: time.Now(),
		messageRepo: fakeMessageRepo{stats: &message_repository.MessageStats{
			Total: 170,
			TopSources: []message_repository.StatKV{
				{Key: "269182931329179", Count: 153},
				{Key: "status", Count: 5},
			},
		}},
		overview: fakeOverview{names: map[string]string{
			"269182931329179": "+5514981170846",
			"status":          "Status",
		}},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.Stats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Messages struct {
			TopSources []struct {
				Key   string `json:"key"`
				Name  string `json:"name"`
				Count int64  `json:"count"`
			} `json:"topSources"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Messages.TopSources) != 2 {
		t.Fatalf("topSources len = %d, want 2", len(body.Messages.TopSources))
	}
	if body.Messages.TopSources[0].Key != "269182931329179" || body.Messages.TopSources[0].Name != "+5514981170846" {
		t.Fatalf("source[0] = %+v, want key 269182931329179 name +5514981170846", body.Messages.TopSources[0])
	}
	if body.Messages.TopSources[0].Count != 153 {
		t.Fatalf("source[0].count = %d, want 153", body.Messages.TopSources[0].Count)
	}
	if body.Messages.TopSources[1].Name != "Status" {
		t.Fatalf("source[1].name = %q, want Status", body.Messages.TopSources[1].Name)
	}
}

// Without a provider the raw keys must still be returned (frontend falls back to
// "+<key>"), so the endpoint stays usable when no instance is connected.
func TestStatsTopSourcesWithoutProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{
		version:   "1.2.3-fork",
		startTime: time.Now(),
		messageRepo: fakeMessageRepo{stats: &message_repository.MessageStats{
			TopSources: []message_repository.StatKV{{Key: "5514981170846", Count: 12}},
		}},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.Stats(c)

	var body struct {
		Messages struct {
			TopSources []struct {
				Key  string `json:"key"`
				Name string `json:"name"`
			} `json:"topSources"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Messages.TopSources) != 1 || body.Messages.TopSources[0].Key != "5514981170846" {
		t.Fatalf("unexpected topSources: %+v", body.Messages.TopSources)
	}
	if body.Messages.TopSources[0].Name != "" {
		t.Fatalf("name = %q, want empty without provider", body.Messages.TopSources[0].Name)
	}
}
