package send_handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	send_service "github.com/felipeestevanatto/wamux/pkg/sendMessage/service"
)

// fakeSender counts service calls. The embedded interface is nil, so a call to a
// method not listed here panics instead of passing silently.
type fakeSender struct {
	send_service.SendService
	calls int
	err   error
	media *send_service.MediaStruct
	file  []byte
}

func (f *fakeSender) done() (*send_service.MessageSendStruct, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &send_service.MessageSendStruct{}, nil
}

type inst = instance_model.Instance

func (f *fakeSender) SendText(*send_service.TextStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendLink(*send_service.LinkStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendMediaUrl(d *send_service.MediaStruct, _ *inst) (*send_service.MessageSendStruct, error) {
	f.media = d
	return f.done()
}
func (f *fakeSender) SendMediaFile(d *send_service.MediaStruct, file []byte, _ *inst) (*send_service.MessageSendStruct, error) {
	f.media, f.file = d, file
	return f.done()
}
func (f *fakeSender) SendPoll(*send_service.PollStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendSticker(*send_service.StickerStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendLocation(*send_service.LocationStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendContact(*send_service.ContactStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendButton(*send_service.ButtonStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendList(*send_service.ListStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendCarousel(*send_service.CarouselStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendEvent(*send_service.EventStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendProduct(*send_service.ProductStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendStatusText(*send_service.StatusTextStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}
func (f *fakeSender) SendStatusMediaUrl(*send_service.StatusMediaStruct, *inst) (*send_service.MessageSendStruct, error) {
	return f.done()
}

func router(f *fakeSender) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewSendHandler(f)
	r := gin.New()
	r.Use(gin.CustomRecovery(func(c *gin.Context, _ any) { c.AbortWithStatus(599) }))
	r.Use(func(c *gin.Context) { c.Set("instance", &inst{Id: "i"}) })
	r.POST("/text", h.SendText)
	r.POST("/link", h.SendLink)
	r.POST("/media", h.SendMedia)
	r.POST("/poll", h.SendPoll)
	r.POST("/sticker", h.SendSticker)
	r.POST("/location", h.SendLocation)
	r.POST("/contact", h.SendContact)
	r.POST("/button", h.SendButton)
	r.POST("/list", h.SendList)
	r.POST("/carousel", h.SendCarousel)
	r.POST("/event", h.SendEvent)
	r.POST("/product", h.SendProduct)
	r.POST("/status-text", h.SendStatusText)
	r.POST("/status-media", h.SendStatusMedia)
	return r
}

func post(r http.Handler, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

type m = map[string]any

// Each route with a valid body, and the fields it must refuse to go without.
var routes = []struct {
	path     string
	body     m
	required []string
}{
	{"/text", m{"number": "1", "text": "hi"}, []string{"number", "text"}},
	{"/link", m{"number": "1", "text": "hi"}, []string{"number", "text"}},
	{"/media", m{"number": "1", "url": "https://x.example/a.jpg", "type": "image"}, []string{"number", "url", "type"}},
	{"/poll", m{"number": "1", "question": "q", "options": []string{"a", "b"}}, []string{"number", "question", "options"}},
	{"/sticker", m{"number": "1", "sticker": "s"}, []string{"number", "sticker"}},
	{"/location", m{"number": "1", "latitude": 1.5, "longitude": 2.5, "address": "a", "name": "n"}, []string{"number", "latitude", "longitude", "address", "name"}},
	{"/contact", m{"number": "1", "vcard": m{"phone": "1", "fullName": "n"}}, []string{"number", "vcard"}},
	{"/button", m{"number": "1", "title": "t", "description": "d", "footer": "f"}, []string{"number", "title", "description", "footer"}},
	{"/list", m{"number": "1", "title": "t", "description": "d", "footerText": "f", "buttonText": "b"}, []string{"number", "title", "description", "footerText", "buttonText"}},
	{"/carousel", m{"number": "1", "cards": []m{{"body": m{"text": "c"}}}}, []string{"number", "cards"}},
	{"/event", m{"number": "1", "name": "n", "startTime": "2026-06-25T20:00:00-03:00"}, []string{"number", "name", "startTime"}},
	{"/product", m{"number": "1", "productId": "p", "title": "t", "currency": "BRL", "imageUrl": "https://x.example/a.jpg"}, []string{"number", "productId", "title", "currency", "imageUrl"}},
	{"/status-text", m{"text": "hi"}, []string{"text"}},
	{"/status-media", m{"url": "https://x.example/a.jpg", "type": "image"}, []string{"url", "type"}},
}

func encode(t *testing.T, body m) string {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestValidRequestReachesTheService(t *testing.T) {
	for _, tc := range routes {
		t.Run(tc.path, func(t *testing.T) {
			f := &fakeSender{}
			w := post(router(f), tc.path, encode(t, tc.body))
			if w.Code != http.StatusOK || f.calls != 1 {
				t.Fatalf("status %d, calls %d: %s", w.Code, f.calls, w.Body)
			}
		})
	}
}

func TestServiceFailureIsReported(t *testing.T) {
	for _, tc := range routes {
		t.Run(tc.path, func(t *testing.T) {
			f := &fakeSender{err: errors.New("not connected")}
			w := post(router(f), tc.path, encode(t, tc.body))
			if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "not connected") {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
		})
	}
}

func TestMissingFieldIsRefused(t *testing.T) {
	for _, tc := range routes {
		for _, field := range tc.required {
			t.Run(tc.path+"/"+field, func(t *testing.T) {
				body := m{}
				for k, v := range tc.body {
					if k != field {
						body[k] = v
					}
				}
				f := &fakeSender{}
				w := post(router(f), tc.path, encode(t, body))
				if w.Code != http.StatusBadRequest || f.calls != 0 {
					t.Fatalf("status %d, calls %d: %s", w.Code, f.calls, w.Body)
				}
			})
		}
	}
}

// `null` is valid JSON, so it gets past the decoder.
func TestUnusableBodiesAreRefused(t *testing.T) {
	for _, tc := range routes {
		for name, body := range map[string]string{"malformed": `{`, "null": `null`, "empty object": `{}`} {
			t.Run(tc.path+"/"+name, func(t *testing.T) {
				f := &fakeSender{}
				w := post(router(f), tc.path, body)
				if w.Code != http.StatusBadRequest || f.calls != 0 {
					t.Fatalf("status %d, calls %d: %s", w.Code, f.calls, w.Body)
				}
			})
		}
	}
}

func TestMediaUrlMustBeHTTP(t *testing.T) {
	f := &fakeSender{}
	w := post(router(f), "/media", `{"number":"1","type":"image","url":"file:///etc/passwd"}`)
	if w.Code != http.StatusBadRequest || f.calls != 0 {
		t.Fatalf("status %d, calls %d", w.Code, f.calls)
	}
}

func TestStatusMediaOnlyTakesImageOrVideo(t *testing.T) {
	f := &fakeSender{}
	w := post(router(f), "/status-media", `{"url":"https://x.example/a","type":"document"}`)
	if w.Code != http.StatusBadRequest || f.calls != 0 {
		t.Fatalf("status %d, calls %d", w.Code, f.calls)
	}
}

func multipartRequest(t *testing.T, fields map[string]string, file []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := form.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if file != nil {
		part, err := form.CreateFormFile("file", "a.jpg")
		if err != nil {
			t.Fatal(err)
		}
		part.Write(file)
	}
	form.Close()
	req := httptest.NewRequest(http.MethodPost, "/media", &buf)
	req.Header.Set("Content-Type", form.FormDataContentType())
	return req
}

func TestMultipartMediaUpload(t *testing.T) {
	f := &fakeSender{}
	w := httptest.NewRecorder()
	router(f).ServeHTTP(w, multipartRequest(t, map[string]string{
		"number": "1", "type": "image", "caption": "c", "delay": "5",
		"mentionAll": "true", "mentionedJid": "a, b", "quoted.messageId": "q",
	}, []byte("file bytes")))

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if string(f.file) != "file bytes" {
		t.Errorf("file = %q", f.file)
	}
	d := f.media
	if d.Number != "1" || d.Caption != "c" || d.Delay != 5 || !d.MentionAll || d.Quoted.MessageID != "q" {
		t.Errorf("data = %+v", d)
	}
	if len(d.MentionedJID) != 2 || d.MentionedJID[1] != "b" {
		t.Errorf("mentions = %v", d.MentionedJID)
	}
}

func TestMultipartMediaRefusals(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]string
		file   []byte
	}{
		{"no number", map[string]string{"type": "image"}, []byte("x")},
		{"no type", map[string]string{"number": "1"}, []byte("x")},
		{"no file", map[string]string{"number": "1", "type": "image"}, nil},
		{"bad delay", map[string]string{"number": "1", "type": "image", "delay": "soon"}, []byte("x")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeSender{}
			w := httptest.NewRecorder()
			router(f).ServeHTTP(w, multipartRequest(t, tc.fields, tc.file))
			if w.Code != http.StatusBadRequest || f.calls != 0 {
				t.Fatalf("status %d, calls %d: %s", w.Code, f.calls, w.Body)
			}
		})
	}
}

func TestReadUploadRefusesOversizedFile(t *testing.T) {
	if _, err := readUpload(bytes.NewReader(make([]byte, maxUploadBytes+1))); err == nil {
		t.Fatal("expected an error")
	}
}
