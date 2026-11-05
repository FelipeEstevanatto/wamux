package whatsmeow_service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	localmedia "github.com/felipeestevanatto/wamux/pkg/media"
	storage_interfaces "github.com/felipeestevanatto/wamux/pkg/storage/interfaces"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

type fakeObjectStore struct {
	storage_interfaces.MediaStorage
	stored []byte
	err    error
}

func (f *fakeObjectStore) StoreReader(_ context.Context, r io.Reader, size int64, _ string, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if int64(len(data)) != size {
		return "", errors.New("size mismatch")
	}
	f.stored = data
	return "https://cdn.example/file", nil
}

type fakeMediaService struct {
	WhatsmeowService
	storage  storage_interfaces.MediaStorage
	delivery string
}

func (f fakeMediaService) MediaStorageFor(*instance_model.Instance) (storage_interfaces.MediaStorage, string) {
	return f.storage, f.delivery
}

func streamingClient(t *testing.T, localStore bool, svc fakeMediaService) *MyClient {
	t.Helper()
	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	t.Cleanup(func() { _ = logs.GetLogger("inst").Close() })
	if localStore {
		if err := localmedia.Configure(t.TempDir()); err != nil {
			t.Fatal(err)
		}
	}
	return &MyClient{
		userID:        "inst",
		service:       svc,
		config:        &config.Config{MediaLocalStore: localStore},
		loggerWrapper: logs,
	}
}

func writes(content string) func(context.Context, *os.File) error {
	return func(_ context.Context, f *os.File) error {
		_, err := f.WriteString(content)
		return err
	}
}

var videoSource = &mediaSource{media: &waE2E.VideoMessage{}, extension: ".mp4", mimeType: "video/mp4"}

func TestStreamMediaToLocalStore(t *testing.T) {
	mycli := streamingClient(t, true, fakeMediaService{delivery: "base64"})

	got := mycli.streamMedia("msg", videoSource, writes("bytes"))

	if !got.local || got.url != localmedia.URLPath("msg") || got.objectURL != "" {
		t.Fatalf("result = %+v", got)
	}
	file, info, err := localmedia.Open("inst", "msg")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if info.Size() != 5 {
		t.Fatalf("stored %d bytes", info.Size())
	}
}

// The file is read for the object store and then moved, so both must get it whole.
func TestStreamMediaToBothStores(t *testing.T) {
	object := &fakeObjectStore{}
	mycli := streamingClient(t, true, fakeMediaService{storage: object, delivery: "both"})

	got := mycli.streamMedia("msg", videoSource, writes("bytes"))

	if got.url != "https://cdn.example/file" || !got.local {
		t.Fatalf("result = %+v", got)
	}
	if !bytes.Equal(object.stored, []byte("bytes")) {
		t.Fatalf("object store got %q", object.stored)
	}
	if !localmedia.Exists("inst", "msg") {
		t.Fatal("local copy missing")
	}
}

// With nowhere to keep it, downloading would only burn bandwidth.
func TestStreamMediaSkipsDownloadWithNoStore(t *testing.T) {
	mycli := streamingClient(t, false, fakeMediaService{delivery: "base64"})

	called := false
	got := mycli.streamMedia("msg", videoSource, func(context.Context, *os.File) error {
		called = true
		return nil
	})

	if called || got.url != "" {
		t.Fatalf("called=%v result=%+v", called, got)
	}
}

func TestStreamMediaFailedDownloadStoresNothing(t *testing.T) {
	mycli := streamingClient(t, true, fakeMediaService{delivery: "base64"})

	got := mycli.streamMedia("msg", videoSource, func(context.Context, *os.File) error {
		return errors.New("network")
	})

	if got.url != "" || got.local || localmedia.Exists("inst", "msg") {
		t.Fatalf("result = %+v", got)
	}
}

// An object store outage must not lose the local copy.
func TestStreamMediaKeepsLocalCopyWhenObjectStoreFails(t *testing.T) {
	object := &fakeObjectStore{err: errors.New("down")}
	mycli := streamingClient(t, true, fakeMediaService{storage: object, delivery: "both"})

	got := mycli.streamMedia("msg", videoSource, writes("bytes"))

	if got.objectURL != "" || !got.local || got.url != localmedia.URLPath("msg") {
		t.Fatalf("result = %+v", got)
	}
}

func TestStreamableMedia(t *testing.T) {
	size := func(n uint64) *uint64 { return &n }
	cases := []struct {
		name string
		msg  *waE2E.Message
		want bool
	}{
		{"nil", nil, false},
		{"text", &waE2E.Message{Conversation: new(string)}, false},
		{"at the limit", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{FileLength: size(100)}}, false},
		{"over the limit", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{FileLength: size(101)}}, true},
		{"unknown size stays in memory", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}, false},
		{"document", &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{FileLength: size(101)}}, true},
		{"image", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{FileLength: size(101)}}, true},
		{"audio", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{FileLength: size(101)}}, true},
		{"sticker needs conversion", &waE2E.Message{StickerMessage: &waE2E.StickerMessage{FileLength: size(101)}}, false},
		{"child message", &waE2E.Message{AssociatedChildMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{VideoMessage: &waE2E.VideoMessage{FileLength: size(101)}},
		}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := streamableMedia(tc.msg, 100) != nil; got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
