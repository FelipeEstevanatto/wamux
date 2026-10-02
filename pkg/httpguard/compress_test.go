package httpguard

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// doCompress serves one GET /x behind the compression middleware and returns
// the recorder.
func doCompress(acceptEncoding string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Compression())
	engine.GET("/x", handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	engine.ServeHTTP(w, req)
	return w
}

// gunzip decodes a gzip response body.
func gunzip(t *testing.T, b []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	return out
}

func TestCompressionAppliesToLargeJSON(t *testing.T) {
	payload := strings.Repeat(`{"hello":"world"}`, 200) // ~3.4 KB, compressible
	w := doCompress("gzip", func(c *gin.Context) { c.String(http.StatusOK, payload) })

	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content-encoding = %q, want gzip", got)
	}
	if got := gunzip(t, w.Body.Bytes()); string(got) != payload {
		t.Fatalf("decoded body mismatch: %d bytes, want %d", len(got), len(payload))
	}
	if w.Body.Len() >= len(payload) {
		t.Fatalf("compressed body (%d) not smaller than original (%d)", w.Body.Len(), len(payload))
	}
	if got := w.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Fatalf("Vary = %q, want to include Accept-Encoding", got)
	}
}

func TestCompressionSkippedWithoutAcceptEncoding(t *testing.T) {
	payload := strings.Repeat("x", 4096)
	w := doCompress("", func(c *gin.Context) { c.String(http.StatusOK, payload) })

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("content-encoding = %q, want empty", got)
	}
	if w.Body.String() != payload {
		t.Fatalf("body was altered without Accept-Encoding")
	}
}

func TestCompressionSkippedWhenGzipRejected(t *testing.T) {
	payload := strings.Repeat("x", 4096)
	w := doCompress("gzip;q=0", func(c *gin.Context) { c.String(http.StatusOK, payload) })

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("content-encoding = %q, want empty for q=0", got)
	}
	if w.Body.String() != payload {
		t.Fatalf("body was altered when gzip was rejected")
	}
}

func TestCompressionAcceptsGzipAmongOthers(t *testing.T) {
	payload := strings.Repeat("x", 4096)
	w := doCompress("br, gzip;q=0.8, deflate", func(c *gin.Context) { c.String(http.StatusOK, payload) })
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content-encoding = %q, want gzip", got)
	}
}

func TestCompressionSkippedForSmallBody(t *testing.T) {
	payload := "tiny"
	w := doCompress("gzip", func(c *gin.Context) { c.String(http.StatusOK, payload) })

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("content-encoding = %q, want empty for body below the floor", got)
	}
	if w.Body.String() != payload {
		t.Fatalf("body = %q, want %q", w.Body.String(), payload)
	}
}

func TestCompressionSkippedForIncompressibleType(t *testing.T) {
	payload := bytes.Repeat([]byte{0xFF, 0xD8}, 4096) // JPEG magic, large enough
	w := doCompress("gzip", func(c *gin.Context) {
		c.Data(http.StatusOK, "image/jpeg", payload)
	})

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("content-encoding = %q, want empty for image/jpeg", got)
	}
	if !bytes.Equal(w.Body.Bytes(), payload) {
		t.Fatalf("image body was altered")
	}
}

func TestCompressionDoesNotDoubleEncode(t *testing.T) {
	payload := strings.Repeat("already", 1000)
	w := doCompress("gzip", func(c *gin.Context) {
		c.Header("Content-Encoding", "gzip")
		c.String(http.StatusOK, payload) // pretend the handler encoded it
	})

	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content-encoding = %q, want gzip (unchanged)", got)
	}
	// The body must be exactly what the handler wrote, not gzip(gzip(...)).
	if w.Body.String() != payload {
		t.Fatalf("middleware re-encoded an already-encoded body")
	}
}

func TestCompressionSkipsBodylessStatuses(t *testing.T) {
	for _, code := range []int{http.StatusNoContent, http.StatusNotModified} {
		w := doCompress("gzip", func(c *gin.Context) { c.Status(code) })
		if w.Code != code {
			t.Fatalf("status = %d, want %d", w.Code, code)
		}
		if w.Body.Len() != 0 {
			t.Fatalf("status %d produced a body of %d bytes", code, w.Body.Len())
		}
		if got := w.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("status %d: content-encoding = %q, want empty", code, got)
		}
	}
}

func TestCompressionSkipsPartialContent(t *testing.T) {
	payload := strings.Repeat("range", 1000)
	w := doCompress("gzip", func(c *gin.Context) {
		c.Header("Content-Range", "bytes 0-4999/10000")
		c.Data(http.StatusPartialContent, "application/octet-stream", []byte(payload))
	})

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("content-encoding = %q, want empty for 206", got)
	}
	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", w.Code)
	}
}

// A body larger than the buffer ceiling must stream through uncompressed rather
// than be held in memory (the media-download case).
func TestCompressionPassesThroughOversizedBody(t *testing.T) {
	payload := strings.Repeat("a", gzipMaxBuffer+4096)
	w := doCompress("gzip", func(c *gin.Context) { c.String(http.StatusOK, payload) })

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("content-encoding = %q, want empty for an oversized body", got)
	}
	if body := w.Body.String(); body != payload {
		t.Fatalf("oversized body altered: got %d bytes, want %d", len(body), len(payload))
	}
}

// An oversized body written in many small chunks must still arrive intact.
func TestCompressionPassesThroughIncrementalOversizedBody(t *testing.T) {
	chunk := strings.Repeat("b", 64*1024)
	total := gzipMaxBuffer/len(chunk) + 4 // crosses the buffer ceiling partway
	w := doCompress("gzip", func(c *gin.Context) {
		c.Status(http.StatusOK)
		for i := 0; i < total; i++ {
			c.Writer.Write([]byte(chunk)) //nolint:errcheck
		}
	})

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("content-encoding = %q, want empty", got)
	}
	if got, want := w.Body.Len(), total*len(chunk); got != want {
		t.Fatalf("body length = %d, want %d", got, want)
	}
}

func TestCompressionPreservesStatusAndHeaders(t *testing.T) {
	payload := strings.Repeat("json", 1000)
	w := doCompress("gzip", func(c *gin.Context) {
		c.Header("X-Custom", "value")
		c.String(http.StatusCreated, payload)
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if got := w.Header().Get("X-Custom"); got != "value" {
		t.Fatalf("X-Custom = %q, want value", got)
	}
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content-encoding = %q, want gzip", got)
	}
	if got := w.Header().Get("Content-Length"); got != "" {
		t.Fatalf("content-length = %q, want empty (encoded size differs)", got)
	}
}

// A handler panic must still surface as a 500. gin.Recovery (from gin.Default)
// is registered *outside* this middleware, so its 500 is written after the
// middleware's finalisation is skipped; the middleware must restore the real
// writer before re-panicking or the response is lost (empty 200).
func TestCompressionPanicStillReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gin.Recovery()) // outer, exactly as gin.Default() does
	engine.Use(Compression())  // inner
	engine.GET("/panic", func(_ *gin.Context) { panic("boom") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panic recovered to status %d, want 500", w.Code)
	}
}

// gin.Static pre-writes 404 (directory listings disabled) and then http.ServeContent
// writes the real 200. The middleware must let the later status win, otherwise
// every served asset 404s for a client that sends Accept-Encoding: gzip.
func TestCompressionStaticAssetNot404(t *testing.T) {
	dir := t.TempDir()
	js := strings.Repeat("function f(){return 1;}\n", 400) // ~9 KB
	if err := os.WriteFile(dir+"/app-abc123.js", []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Compression())
	engine.Static("/assets", dir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (gin.Static pre-writes 404)", w.Code)
	}
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content-encoding = %q, want gzip", got)
	}
	if got := gunzip(t, w.Body.Bytes()); string(got) != js {
		t.Fatalf("decoded asset mismatch: %d bytes, want %d", len(got), len(js))
	}
}

func TestCompressionAbortLeavesBodyUncompressed(t *testing.T) {
	payload := strings.Repeat(`{"error":"not authorized"}`, 100) // > floor
	w := doCompress("gzip", func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": payload})
	})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	// A JSON error body above the floor is compressed, and must decode back.
	if got := w.Header().Get("Content-Encoding"); got == "gzip" {
		decoded := gunzip(t, w.Body.Bytes())
		if !strings.Contains(string(decoded), "not authorized") {
			t.Fatalf("decoded error body missing content: %s", decoded)
		}
		return
	}
	if !strings.Contains(w.Body.String(), "not authorized") {
		t.Fatalf("error body missing content")
	}
}

// The manager SPA is served through c.File (eng.Static -> http.ServeFile). This
// proves a content-hashed asset is compressed end to end, not just c.JSON.
func TestCompressionAppliesToServedAsset(t *testing.T) {
	js := strings.Repeat("function f(){return 1;}\n", 400) // ~9 KB of "JS"
	dir := t.TempDir()
	name := dir + "/app-abc123.js"
	if err := os.WriteFile(name, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Compression())
	engine.GET("/assets/:file", func(c *gin.Context) { c.File(name) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content-encoding = %q, want gzip (asset not compressed)", got)
	}
	if got := gunzip(t, w.Body.Bytes()); string(got) != js {
		t.Fatalf("decoded asset mismatch: %d bytes, want %d", len(got), len(js))
	}
}

func TestAcceptsGzip(t *testing.T) {
	cases := map[string]bool{
		"":                  false,
		"gzip":              true,
		"gzip, deflate, br": true,
		"br, gzip;q=0.5":    true,
		"gzip;q=0":          false,
		"gzip; q=0":         false,
		"deflate, br":       false,
		"GZIP":              true,
		"identity":          false,
		"*":                 false, // we only honour an explicit gzip token
	}
	for header, want := range cases {
		if got := acceptsGzip(header); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

func TestCompressibleContentType(t *testing.T) {
	yes := []string{
		"application/json",
		"application/json; charset=utf-8",
		"application/javascript",
		"text/html; charset=utf-8",
		"text/css",
		"image/svg+xml",
		"application/problem+json",
	}
	no := []string{
		"",
		"image/jpeg",
		"image/png",
		"video/mp4",
		"application/octet-stream",
		"application/zip",
	}
	for _, ct := range yes {
		if !compressibleContentType(ct) {
			t.Errorf("compressibleContentType(%q) = false, want true", ct)
		}
	}
	for _, ct := range no {
		if compressibleContentType(ct) {
			t.Errorf("compressibleContentType(%q) = true, want false", ct)
		}
	}
}

func BenchmarkCompressionJSON(b *testing.B) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Compression())
	payload := strings.Repeat(`{"id":"abc","value":123}`, 500)
	engine.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, payload) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
	}
}
