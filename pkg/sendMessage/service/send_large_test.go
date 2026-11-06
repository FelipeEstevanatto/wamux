package send_service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	localmedia "github.com/felipeestevanatto/wamux/pkg/media"
)

// mp4Header is enough for the type to be detected as MP4.
var mp4Header = []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")

func largeFile(t *testing.T, content []byte) *os.File {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "upload"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	return file
}

// useLocalStore points the package-wide store at this test's own directory.
func useLocalStore(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "media")
	if err := localmedia.Configure(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// recordStream replaces the streaming upload and returns what it read.
func recordStream(h *sendHarness) *bytes.Buffer {
	var read bytes.Buffer
	h.uploadReader = func(_ context.Context, _ *whatsmeow.Client, r io.Reader, kind whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		h.uploads = append(h.uploads, kind)
		size, err := io.Copy(&read, r)
		return whatsmeow.UploadResponse{URL: "https://mmg.test/big", DirectPath: "/big", MediaKey: []byte{1}, FileLength: uint64(size)}, err
	}
	return &read
}

func TestStreamDocument(t *testing.T) {
	h := newSendHarness(t)
	read := recordStream(h)
	content := []byte("a large document")

	got, err := h.SendMediaStream(&MediaStruct{Number: testNumber, Type: "document", Filename: "big.txt", FormatJid: raw()}, largeFile(t, content), h.instance)
	if err != nil {
		t.Fatalf("SendMediaStream: %v", err)
	}

	// The type check reads the start of the file, so the upload must rewind.
	if !bytes.Equal(read.Bytes(), content) {
		t.Fatalf("uploaded %q", read.Bytes())
	}
	if h.uploads[0] != whatsmeow.MediaDocument {
		t.Errorf("uploads = %v", h.uploads)
	}
	doc := h.only(t).msg.GetDocumentMessage()
	if doc.GetFileName() != "big.txt" || doc.GetFileLength() != uint64(len(content)) || doc.GetURL() == "" {
		t.Errorf("document = %+v", doc)
	}
	if got.MediaData != nil {
		t.Error("the file ended up in memory")
	}
}

func TestStreamDocumentWithCaption(t *testing.T) {
	h := newSendHarness(t)
	recordStream(h)

	_, err := h.SendMediaStream(&MediaStruct{Number: testNumber, Type: "document", Filename: "a", Caption: "read", FormatJid: raw()}, largeFile(t, []byte("x")), h.instance)
	if err != nil {
		t.Fatal(err)
	}

	msg := h.only(t).msg
	if msg.GetDocumentMessage() != nil {
		t.Fatal("a captioned document must be wrapped")
	}
	if got := msg.GetDocumentWithCaptionMessage().GetMessage().GetDocumentMessage().GetCaption(); got != "read" {
		t.Fatalf("caption = %q", got)
	}
}

func TestStreamVideo(t *testing.T) {
	h := newSendHarness(t)
	recordStream(h)

	_, err := h.SendMediaStream(&MediaStruct{Number: testNumber, Type: "video", Caption: "clip", ViewOnce: true, FormatJid: raw()}, largeFile(t, mp4Header), h.instance)
	if err != nil {
		t.Fatalf("SendMediaStream: %v", err)
	}

	video := h.only(t).msg.GetVideoMessage()
	if video.GetMimetype() != "video/mp4" || video.GetCaption() != "clip" || !video.GetViewOnce() {
		t.Fatalf("video = %+v", video)
	}
	if h.uploads[0] != whatsmeow.MediaVideo {
		t.Errorf("uploads = %v", h.uploads)
	}
}

func TestStreamRealVideoHasPreviewAndSize(t *testing.T) {
	file := largeFile(t, sampleVideo(t))
	h := newSendHarness(t)
	recordStream(h)

	if _, err := h.SendMediaStream(&MediaStruct{Number: testNumber, Type: "video", FormatJid: raw()}, file, h.instance); err != nil {
		t.Fatalf("SendMediaStream: %v", err)
	}

	video := h.only(t).msg.GetVideoMessage()
	if video.GetWidth() != 64 || video.GetSeconds() != 2 || len(video.GetJPEGThumbnail()) == 0 {
		t.Fatalf("video = %+v", video)
	}
}

func TestStreamRefusesWhatNeedsMemory(t *testing.T) {
	for name, media := range map[string]MediaStruct{
		"image":        {Type: "image"},
		"audio":        {Type: "audio"},
		"not an mp4":   {Type: "video"},
		"unknown type": {Type: "banana"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)
			recordStream(h)
			media.Number, media.FormatJid = testNumber, raw()

			_, err := h.SendMediaStream(&media, largeFile(t, []byte("plain text")), h.instance)
			if !errors.Is(err, ErrLargeMediaType) {
				t.Fatalf("err = %v", err)
			}
			if len(h.uploads) != 0 || len(h.sent) != 0 {
				t.Fatalf("uploaded %d, sent %d", len(h.uploads), len(h.sent))
			}
		})
	}
}

func TestStreamToAChannel(t *testing.T) {
	h := newSendHarness(t)
	h.uploadReader = func(context.Context, *whatsmeow.Client, io.Reader, whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		t.Error("used the encrypted upload")
		return whatsmeow.UploadResponse{}, nil
	}
	h.uploadNewsletterReader = func(context.Context, *whatsmeow.Client, io.ReadSeeker, whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		return whatsmeow.UploadResponse{URL: "https://mmg.test/n", Handle: "handle-3"}, nil
	}

	_, err := h.SendMediaStream(&MediaStruct{Number: testNewsletter, Type: "video", FormatJid: raw()}, largeFile(t, mp4Header), h.instance)
	if err != nil {
		t.Fatalf("SendMediaStream: %v", err)
	}

	sent := h.only(t)
	if sent.to.Server != types.NewsletterServer || sent.extra.MediaHandle != "handle-3" {
		t.Fatalf("sent %+v", sent)
	}
}

func TestStreamRefusesDocumentsInChannels(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendMediaStream(&MediaStruct{Number: testNewsletter, Type: "document", FormatJid: raw()}, largeFile(t, []byte("x")), h.instance)
	if err == nil || !strings.Contains(err.Error(), "canais") {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamReportsAFailedUpload(t *testing.T) {
	h := newSendHarness(t)
	h.uploadReader = func(context.Context, *whatsmeow.Client, io.Reader, whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		return whatsmeow.UploadResponse{}, errors.New("media server down")
	}

	_, err := h.SendMediaStream(&MediaStruct{Number: testNumber, Type: "document", FormatJid: raw()}, largeFile(t, []byte("x")), h.instance)
	if err == nil || !strings.Contains(err.Error(), "media server down") {
		t.Fatalf("err = %v", err)
	}
	if len(h.sent) != 0 {
		t.Fatal("sent a message for a file that never uploaded")
	}
}

func TestStreamKeepsALocalCopy(t *testing.T) {
	useLocalStore(t)
	h := newSendHarness(t)
	h.config.MediaLocalStore = true
	recordStream(h)

	got, err := h.SendMediaStream(&MediaStruct{Number: testNumber, Type: "document", Filename: "a", FormatJid: raw()}, largeFile(t, []byte("kept")), h.instance)
	if err != nil {
		t.Fatal(err)
	}

	row := h.savedRow(t)
	if row.MediaUrl == "" {
		t.Fatal("no media url on the stored row")
	}
	file, _, err := localmedia.Open(testInstanceID, got.Info.ID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()
	if content, _ := io.ReadAll(file); string(content) != "kept" {
		t.Fatalf("content = %q", content)
	}
}

func TestReadOrSpool(t *testing.T) {
	dir := useLocalStore(t)

	t.Run("small stays in memory", func(t *testing.T) {
		data, file, err := readOrSpool(strings.NewReader("small"), 1<<30)
		if err != nil || file != nil || string(data) != "small" {
			t.Fatalf("data %q, file %v, err %v", data, file, err)
		}
	})

	t.Run("large goes to disk whole", func(t *testing.T) {
		size := int64(MaxInMemoryMediaBytes + 10)
		data, file, err := readOrSpool(io.LimitReader(zeros{}, size), 1<<30)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(file.Name())
		defer file.Close()
		info, _ := file.Stat()
		if data != nil || info.Size() != size {
			t.Fatalf("in memory %d, on disk %d", len(data), info.Size())
		}
	})

	t.Run("over the limit is refused and cleaned up", func(t *testing.T) {
		_, file, err := readOrSpool(io.LimitReader(zeros{}, MaxInMemoryMediaBytes+10), MaxInMemoryMediaBytes+5)
		if !errors.Is(err, ErrMediaTooLarge) || file != nil {
			t.Fatalf("file %v, err %v", file, err)
		}
		if left, _ := os.ReadDir(dir); len(left) != 0 {
			t.Errorf("left behind: %v", left)
		}
	})
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestSendMediaUrlStreamsALargeFile(t *testing.T) {
	useLocalStore(t)
	size := int64(MaxInMemoryMediaBytes + 1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.Copy(w, io.LimitReader(zeros{}, size))
	}))
	t.Cleanup(server.Close)
	h := newSendHarness(t)
	h.config.MaxUploadBytes = 1 << 30
	read := recordStream(h)

	_, err := h.SendMediaUrl(&MediaStruct{Number: testNumber, Url: server.URL, Type: "document", Filename: "big.bin", FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendMediaUrl: %v", err)
	}

	if int64(read.Len()) != size {
		t.Fatalf("uploaded %d of %d bytes", read.Len(), size)
	}
}

func TestSendMediaUrlRefusesAFileOverTheLimit(t *testing.T) {
	useLocalStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.Copy(w, io.LimitReader(zeros{}, MaxInMemoryMediaBytes+1024))
	}))
	t.Cleanup(server.Close)
	h := newSendHarness(t)
	h.config.MaxUploadBytes = MaxInMemoryMediaBytes + 10
	recordStream(h)

	_, err := h.SendMediaUrl(&MediaStruct{Number: testNumber, Url: server.URL, Type: "document", FormatJid: raw()}, h.instance)
	if !errors.Is(err, ErrMediaTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if len(h.uploads) != 0 {
		t.Fatal("uploaded anyway")
	}
}
