// gzip response compression for the JSON API and the manager SPA.
//
// # WHY
//
// The manager ships ~430 KB of JS (already code-split) and the history
// endpoints return large JSON arrays; none of it was compressed. A 2 MB
// /chat/history response transferred raw is ~2 MB on the wire, and the panel's
// first load pays for every byte. gzip at the default level typically removes
// 60–80% of JSON and 65–75% of JS, and clients already advertise
// Accept-Encoding: gzip — the server just never used it.
//
// # WHY HAND-WRITTEN, NO DEPENDENCY
//
// Same reasoning as the rate limiter: this is a small middleware, the package
// deliberately avoids pulling in gin-contrib/gzip, and the semantics we want
// (an allowlist of compressible types, a size floor, WebSocket- and
// media-safe) are specific enough that a wrapper would be more configuration
// than code.
//
// # DESIGN
//
//   - One sync.Pool of gzip.Writers. A writer is expensive to allocate (a 32 KB
//     window plus hash tables); reusing them per request — never across
//     requests, which would race — keeps the per-request cost to a Reset.
//   - The handler's status, headers and body are recorded and the complete body
//     is encoded once, after the handler returns. That covers c.JSON, c.String,
//     c.Data, c.File and gin.Static's file serving without touching any route.
//   - The recorder buffers up to gzipMaxBuffer bytes. Past that it flushes what
//     it holds and streams the rest straight through, uncompressed. This is
//     what keeps /chat/media/... (a file, possibly a large video, served with
//     http.ServeContent and Range support) from being read into memory.
//   - Already-encoded payloads (images, gzip), partial (206) responses and
//     bodies below a floor are sent identity: re-compressing incompressible
//     bytes only burns CPU.
//   - WebSocket upgrades fall through untouched. The upgrade hijacks the
//     connection and writes the 101 directly to it, bypassing this writer; the
//     middleware detects the hijack and writes nothing.
//
// # TRANSPARENCY
//
// Gin's `writer` interface is unexported, so this middleware cannot append to
// the *gin.ResponseWriter's internal body. It wraps the writer with a recorder
// instead and writes the finished, therefore valid, gzip member afterwards.
// WriteHeader is deferred until the handler returns so the encoding headers are
// correct. The trade-off is a buffered response; this API is not a streaming
// one (nothing calls Flush/SSEvent), and the buffer is bounded by
// gzipMaxBuffer.
//
// # BREACH (informational)
//
// Compressing a response that reflects attacker-influenced input alongside a
// secret can leak the secret through compressed size (BREACH). This API is
// admin/instance-authenticated and no handler currently echoes caller input in
// the same response as a secret, so the risk is low; it is noted here so a
// future endpoint that does both is deliberately reviewed rather than
// overlooked. Compression can also be turned off with HTTP_COMPRESSION=false.
package httpguard

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

const (
	// gzipMinBytes is the smallest response worth compressing. Below this the
	// gzip header/trailer and CPU cost outweigh any saving, and small JSON
	// error bodies are the common case.
	gzipMinBytes = 1024

	// gzipMaxBuffer bounds how much of a response is held in memory before the
	// writer gives up on compressing and streams the rest. One megabyte covers
	// a large history page while keeping the worst-case memory per in-flight
	// media download small.
	gzipMaxBuffer = 1 << 20

	// gzipDefaultLevel is compress/gzip.DefaultCompression (-1): the level the
	// standard library tunes as the best CPU/ratio trade-off for a server, and
	// what gin-contrib/gzip uses.
	gzipDefaultLevel = gzip.DefaultCompression
)

// gzipWriterPool reuses compressors across requests. New gzip writers share no
// mutable state, so a pooled instance is safe to Reset and reuse.
var gzipWriterPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(discardWriter{}, gzipDefaultLevel)
		return w
	},
}

// discardWriter is a throwaway sink so a pooled writer can be constructed
// without a real destination; Reset replaces it before use.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// compressibleContentType reports whether a Content-Type is worth gzipping. It
// is an allowlist rather than a blocklist, so anything unknown (a binary we
// forgot about) is sent identity, never doubled.
func compressibleContentType(ct string) bool {
	if ct == "" {
		return false
	}
	// Drop parameters: "application/json; charset=utf-8" -> "application/json".
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.ToLower(strings.TrimSpace(ct))

	switch ct {
	case "application/json",
		"application/javascript",
		"application/xml",
		"application/xhtml+xml",
		"application/rss+xml",
		"application/atom+xml",
		"application/wasm",
		"application/vnd.api+json",
		"image/svg+xml",
		"text/css",
		"text/csv",
		"text/html",
		"text/javascript",
		"text/plain",
		"text/xml":
		return true
	}
	return strings.HasSuffix(ct, "+json") || strings.HasSuffix(ct, "+xml")
}

// acceptsGzip reports whether the client's Accept-Encoding allows gzip. Only
// gzip is offered; the token is valid even alongside other encodings and is
// ignored when explicitly rejected with q=0.
func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(token), "gzip") {
			continue
		}
		// "gzip;q=0" (exactly zero, any spelling) means "do not use gzip".
		// Parse numerically so "q=0.5" is not mistaken for "q=0".
		for _, p := range strings.Split(params, ";") {
			key, val, ok := strings.Cut(strings.TrimSpace(p), "=")
			if !ok || !strings.EqualFold(key, "q") {
				continue
			}
			if q, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil && q <= 0 {
				return false
			}
		}
		return true
	}
	return false
}

// compressWriter records the handler's status, headers and body so the complete
// response can be encoded once, after the handler returns. It is also the
// escape hatch for large responses: past gzipMaxBuffer it flushes and streams.
type compressWriter struct {
	gin.ResponseWriter
	body        *bytes.Buffer
	status      int
	hijacked    bool
	passthrough bool
}

func newCompressWriter(inner gin.ResponseWriter) *compressWriter {
	return &compressWriter{ResponseWriter: inner, body: bytes.NewBuffer(make([]byte, 0, gzipMinBytes))}
}

func (w *compressWriter) WriteHeader(code int) {
	// Deferred until the middleware runs, so the encoding headers can still be
	// set. Mirror gin's own semantics — a later WriteHeader may still correct an
	// earlier one until the body is written — because gin.Static pre-writes 404
	// (listings disabled) and then lets http.ServeContent write the real 200.
	// Locking in the first status here would turn every served asset into a 404.
	if code > 0 && w.status != code && !w.Written() {
		w.status = code
	}
}

func (w *compressWriter) WriteHeaderNow() {}

func (w *compressWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.passthrough {
		return w.ResponseWriter.Write(b)
	}
	if w.body.Len()+len(b) > gzipMaxBuffer {
		if err := w.flushBuffered(); err != nil {
			return 0, err
		}
		return w.ResponseWriter.Write(b)
	}
	return w.body.Write(b)
}

// WriteString must route through Write: the embedded gin writer's WriteString
// writes straight to the wrapped ResponseWriter, which would bypass the buffer.
func (w *compressWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// Flush sends whatever is buffered and then streams, matching the contract a
// caller expecting a flushed response understands. Nothing in this codebase
// flushes, but a future SSE route would need it.
func (w *compressWriter) Flush() {
	if w.hijacked {
		return
	}
	if !w.passthrough {
		_ = w.flushBuffered()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Status reports the handler's status, falling back to the underlying writer
// (which Gin sets through c.Status/c.Render even when our WriteHeader is not
// called, e.g. AbortWithStatusJSON).
func (w *compressWriter) Status() int {
	if w.status != 0 {
		return w.status
	}
	return w.ResponseWriter.Status()
}

func (w *compressWriter) Size() int {
	if w.passthrough {
		return w.ResponseWriter.Size()
	}
	return w.body.Len()
}

func (w *compressWriter) Written() bool {
	return w.passthrough || w.body.Len() > 0
}

// Hijack forwards to the underlying writer and remembers that the connection
// was taken over, so the middleware does not try to write a response afterwards.
func (w *compressWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true
	return w.ResponseWriter.Hijack()
}

// flushBuffered writes the deferred status and any buffered bytes to the real
// writer uncompressed, and switches to passthrough. It is idempotent.
func (w *compressWriter) flushBuffered() error {
	if w.passthrough {
		return nil
	}
	w.passthrough = true
	w.ResponseWriter.WriteHeader(w.Status())
	if w.body.Len() == 0 {
		return nil
	}
	_, err := w.ResponseWriter.Write(w.body.Bytes())
	w.body.Reset()
	return err
}

// addVaryAcceptEncoding appends Accept-Encoding to Vary when absent, so shared
// caches key on the encoding.
func addVaryAcceptEncoding(c *gin.Context) {
	vary := c.Writer.Header().Get("Vary")
	for _, part := range strings.Split(vary, ",") {
		if strings.EqualFold(strings.TrimSpace(part), "Accept-Encoding") {
			return
		}
	}
	if vary == "" {
		c.Writer.Header().Set("Vary", "Accept-Encoding")
		return
	}
	c.Writer.Header().Set("Vary", vary+", Accept-Encoding")
}

// Compression returns a Gin middleware that gzip-encodes responses when the
// client accepts it and the body is compressible and large enough. It is a
// no-op for every other response.
func Compression() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !acceptsGzip(c.GetHeader("Accept-Encoding")) {
			c.Next()
			return
		}

		w := newCompressWriter(c.Writer)
		c.Writer = w

		// A panic unwinds past this middleware, so the finalisation below never
		// runs and the buffered body is never flushed. The panic is caught by the
		// outer recovery middleware (gin.Default's gin.Recovery), which writes its
		// 500 through c.Writer — so restore the real writer first, otherwise that
		// write is swallowed by the discarded buffer and the client sees an empty
		// 200 instead of a 500.
		defer func() {
			if rec := recover(); rec != nil {
				c.Writer = w.ResponseWriter
				panic(rec)
			}
		}()

		c.Next()

		// A hijacked connection (WebSocket upgrade) is owned by its handler, and
		// a passthrough response has already been written in full.
		if w.hijacked || w.passthrough {
			return
		}

		status := w.Status()

		// A shared cache must key the stored response on Accept-Encoding, so a
		// gzip body is never served to a client that cannot decode it. Set on
		// every response this middleware could have compressed, not only the
		// ones it did.
		addVaryAcceptEncoding(c)

		// These statuses carry no body; write the deferred status and stop.
		if status == http.StatusNoContent || status == http.StatusNotModified {
			w.ResponseWriter.WriteHeader(status)
			return
		}

		body := w.body.Bytes()
		ct := w.Header().Get("Content-Type")
		compress := len(body) >= gzipMinBytes &&
			w.Header().Get("Content-Encoding") == "" &&
			status != http.StatusPartialContent && // 206 carries Content-Range
			compressibleContentType(ct)

		if !compress {
			w.ResponseWriter.WriteHeader(status)
			if len(body) > 0 {
				w.ResponseWriter.Write(body) //nolint:errcheck // mirrors net/http best effort
			}
			return
		}

		// The encoding headers must be in place BEFORE the first byte of the
		// gzip body: that first Write triggers WriteHeaderNow, which is when
		// Gin materialises the header table. Content-Length is dropped because
		// the encoded size differs from the original and gzip.Writer does not
		// know it up front (chunked transfer is used instead).
		h := w.Header()
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")

		// c.Status's deferred WriteHeader records the status on Gin's writer
		// without materialising it; the Write below materialises it.
		w.ResponseWriter.WriteHeader(status)

		gz := gzipWriterPool.Get().(*gzip.Writer)
		gz.Reset(w.ResponseWriter)
		gz.Write(body) //nolint:errcheck // gzip.Writer buffers; the destination write cannot fail on an in-memory body
		gz.Close()     //nolint:errcheck // flushes the trailer
		gzipWriterPool.Put(gz)
	}
}
