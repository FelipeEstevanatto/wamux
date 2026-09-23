package server_handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	chats map[string]whatsmeow_service.ChatIdentity
}

func (f fakeOverview) GetInstanceOverview(string) (*whatsmeow_service.InstanceOverview, error) {
	return f.ov, f.err
}

func (f fakeOverview) ResolveChats(users []string) map[string]whatsmeow_service.ChatIdentity {
	out := make(map[string]whatsmeow_service.ChatIdentity, len(users))
	for _, u := range users {
		if c, ok := f.chats[u]; ok {
			out[u] = c
		}
	}
	return out
}

// fakeMessageRepo implements just enough of MessageRepository for the handler.
type fakeMessageRepo struct {
	byInstance int64
	chats      int64
	stats      *message_repository.MessageStats
	dbTotal    int64
	dbMessages int64
}

func (fakeMessageRepo) InsertMessage(message_model.Message) error             { return nil }
func (fakeMessageRepo) GetMessageByID(string) (*message_model.Message, error) { return nil, nil }
func (fakeMessageRepo) DeleteAllMessages() (int64, error)                     { return 0, nil }
func (fakeMessageRepo) GetLatestMessageID(string) (string, string, error)     { return "", "", nil }
func (f fakeMessageRepo) GetStats() (*message_repository.MessageStats, error) {
	if f.stats == nil {
		return &message_repository.MessageStats{
			ByStatus:   []message_repository.StatKV{},
			ByDay:      []message_repository.StatKV{},
			TopSources: []message_repository.StatKV{},
		}, nil
	}
	return f.stats, nil
}
func (f fakeMessageRepo) CountByInstance(string) (int64, error)      { return f.byInstance, nil }
func (f fakeMessageRepo) CountChatsByInstance(string) (int64, error) { return f.chats, nil }
func (f fakeMessageRepo) DatabaseSizeBytes() (int64, int64, error) {
	return f.dbTotal, f.dbMessages, nil
}
func (fakeMessageRepo) DeleteMessagesOlderThan(string) (int64, error) { return 0, nil }

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

func TestStatsResolvesAndMergesTopSources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{
		version:   "1.2.3-fork",
		startTime: time.Now(),
		messageRepo: fakeMessageRepo{stats: &message_repository.MessageStats{
			Total: 170,
			TopSources: []message_repository.StatKV{
				{Key: "269182931329179", Count: 153}, // LID
				{Key: "5514981170846", Count: 12},    // same contact, by phone
				{Key: "status", Count: 5},
			},
		}},
		overview: fakeOverview{chats: map[string]whatsmeow_service.ChatIdentity{
			"269182931329179": {Name: "Evogo Saved Contact", Phone: "5514981170846"},
			"5514981170846":   {Name: "Evogo Saved Contact", Phone: "5514981170846"},
			"status":          {Name: "Status"},
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
				Phone string `json:"phone"`
				Count int64  `json:"count"`
			} `json:"topSources"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// The LID and phone rows are the same conversation and must collapse into one.
	if len(body.Messages.TopSources) != 2 {
		t.Fatalf("topSources len = %d, want 2 (merged): %+v", len(body.Messages.TopSources), body.Messages.TopSources)
	}
	first := body.Messages.TopSources[0]
	if first.Key != "5514981170846" || first.Name != "Evogo Saved Contact" || first.Phone != "5514981170846" {
		t.Fatalf("merged source = %+v, want key/phone 5514981170846 name Evogo Saved Contact", first)
	}
	if first.Count != 165 {
		t.Fatalf("merged count = %d, want 165 (153+12)", first.Count)
	}
	if body.Messages.TopSources[1].Name != "Status" || body.Messages.TopSources[1].Count != 5 {
		t.Fatalf("status source = %+v, want name Status count 5", body.Messages.TopSources[1])
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

func TestStatsIncludesStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.log"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := &serverHandler{
		version:      "1.2.3-fork",
		startTime:    time.Now(),
		dataDir:      dir,
		mediaBackend: "minio:evolution-media",
		messageRepo:  fakeMessageRepo{dbTotal: 2 * 1024 * 1024, dbMessages: 1024 * 1024},
		dirUsage:     dirUsage{interval: time.Minute},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.Stats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Storage struct {
			DataDir      string  `json:"dataDir"`
			DataUsedMB   float64 `json:"dataUsedMB"`
			DataFiles    int64   `json:"dataFiles"`
			DBTotalMB    float64 `json:"dbTotalMB"`
			DBMessagesMB float64 `json:"dbMessagesMB"`
			MediaEnabled bool    `json:"mediaEnabled"`
			MediaBackend string  `json:"mediaBackend"`
		} `json:"storage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Storage.DataDir != dir || body.Storage.DataFiles != 1 || body.Storage.DataUsedMB <= 0 {
		t.Fatalf("data usage = %+v, want dir %s, 1 file, >0 MB", body.Storage, dir)
	}
	if body.Storage.DBTotalMB != 2 || body.Storage.DBMessagesMB != 1 {
		t.Fatalf("db usage = %+v, want 2 MB total / 1 MB messages", body.Storage)
	}
	if !body.Storage.MediaEnabled || body.Storage.MediaBackend != "minio:evolution-media" {
		t.Fatalf("media = %+v, want enabled minio:evolution-media", body.Storage)
	}
}

// The storage panel is best-effort: with nothing wired it must still return 200
// and an empty (but present) object rather than failing the whole endpoint.
func TestStatsStorageWithoutData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{version: "1.2.3-fork", startTime: time.Now()}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.Stats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Storage map[string]any `json:"storage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Storage == nil {
		t.Fatal("storage key missing")
	}
	if enabled, ok := body.Storage["mediaEnabled"].(bool); !ok || enabled {
		t.Fatalf("mediaEnabled = %v, want false", body.Storage["mediaEnabled"])
	}
}
