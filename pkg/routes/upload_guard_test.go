package routes

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func guardedEngine(limit int64, reached *bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.POST("/media", uploadGuard(limit), func(c *gin.Context) {
		*reached = true
		c.JSON(http.StatusOK, gin.H{"number": c.PostForm("number")})
	})
	return eng
}

func multipartBody(t *testing.T, size int) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("number", "1"); err != nil {
		t.Fatal(err)
	}
	part, err := form.CreateFormFile("file", "a.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(make([]byte, size)); err != nil {
		t.Fatal(err)
	}
	form.Close()
	return &body, form.FormDataContentType()
}

func TestUploadGuardLetsAFileThrough(t *testing.T) {
	var reached bool
	body, kind := multipartBody(t, 100)
	req := httptest.NewRequest(http.MethodPost, "/media", body)
	req.Header.Set("Content-Type", kind)
	w := httptest.NewRecorder()

	guardedEngine(1000, &reached).ServeHTTP(w, req)

	if !reached || w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"number":"1"`) {
		t.Fatalf("reached %v, status %d: %s", reached, w.Code, w.Body)
	}
}

func TestUploadGuardRefusesByDeclaredLength(t *testing.T) {
	var reached bool
	body, kind := multipartBody(t, 2000)
	req := httptest.NewRequest(http.MethodPost, "/media", body)
	req.Header.Set("Content-Type", kind)
	w := httptest.NewRecorder()

	guardedEngine(1000, &reached).ServeHTTP(w, req)

	if reached || w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), "MAX_UPLOAD_BYTES") {
		t.Fatalf("reached %v, status %d: %s", reached, w.Code, w.Body)
	}
}

// A chunked body declares no length, so only reading it can find the limit. Without
// the check the request would read as having no fields.
func TestUploadGuardRefusesAChunkedBodyOverTheLimit(t *testing.T) {
	var reached bool
	body, kind := multipartBody(t, 2000)
	req := httptest.NewRequest(http.MethodPost, "/media", io.MultiReader(body))
	req.ContentLength = -1
	req.Header.Set("Content-Type", kind)
	w := httptest.NewRecorder()

	guardedEngine(1000, &reached).ServeHTTP(w, req)

	if reached || w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("reached %v, status %d: %s", reached, w.Code, w.Body)
	}
}

func TestUploadGuardRefusesABrokenForm(t *testing.T) {
	var reached bool
	req := httptest.NewRequest(http.MethodPost, "/media", strings.NewReader("not a form"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	w := httptest.NewRecorder()

	guardedEngine(1000, &reached).ServeHTTP(w, req)

	if reached || w.Code != http.StatusBadRequest {
		t.Fatalf("reached %v, status %d: %s", reached, w.Code, w.Body)
	}
}

func TestUploadGuardLeavesJSONToTheHandler(t *testing.T) {
	var reached bool
	req := httptest.NewRequest(http.MethodPost, "/media", strings.NewReader(`{"url":"https://x.test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	guardedEngine(1000, &reached).ServeHTTP(w, req)

	if !reached {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
}

// Through a real server, with a read timeout shorter than the upload: the guard has
// to extend it, or the connection dies mid-file.
func TestUploadGuardOutlivesTheServerTimeouts(t *testing.T) {
	var reached bool
	server := httptest.NewUnstartedServer(guardedEngine(1<<20, &reached))
	server.Config.ReadTimeout = 100 * time.Millisecond
	server.Config.WriteTimeout = 100 * time.Millisecond
	server.Start()
	t.Cleanup(server.Close)

	body, kind := multipartBody(t, 1000)
	reader, writer := io.Pipe()
	go func() {
		half := body.Len() / 2
		_, _ = writer.Write(body.Next(half))
		time.Sleep(400 * time.Millisecond)
		_, _ = writer.Write(body.Bytes())
		writer.Close()
	}()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/media", reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", kind)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload died: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK || !reached {
		t.Fatalf("status %d, reached %v", resp.StatusCode, reached)
	}
}
