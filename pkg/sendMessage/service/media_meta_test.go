package send_service

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
)

func makeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 0x80, 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func TestImageDimensions(t *testing.T) {
	png := makeTestPNG(t, 320, 180)
	w, h, ok := imageDimensions(png)
	if !ok {
		t.Fatal("expected dimensions for a valid PNG")
	}
	if w != 320 || h != 180 {
		t.Errorf("got %dx%d, want 320x180", w, h)
	}

	if _, _, ok := imageDimensions([]byte("not an image")); ok {
		t.Error("garbage must not yield dimensions")
	}
	if _, _, ok := imageDimensions(nil); ok {
		t.Error("nil must not yield dimensions")
	}
}

func TestFetchLinkThumbnail(t *testing.T) {
	png := makeTestPNG(t, 600, 400)

	t.Run("converts og:image to jpeg with dimensions", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		}))
		defer srv.Close()

		thumb, w, h := fetchLinkThumbnail(srv.URL)
		if len(thumb) == 0 {
			t.Fatal("expected a thumbnail")
		}
		if w != 300 || h != 200 {
			t.Errorf("thumbnail dimensions = %dx%d, want 300x200 (capped at 300 wide)", w, h)
		}
		// Must be a real JPEG.
		if _, _, ok := imageDimensions(thumb); !ok {
			t.Error("thumbnail is not a decodable image")
		}
	})

	t.Run("non-2xx yields no thumbnail", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusNotFound)
		}))
		defer srv.Close()

		if thumb, _, _ := fetchLinkThumbnail(srv.URL); len(thumb) != 0 {
			t.Error("a 404 must not produce a thumbnail")
		}
	})

	t.Run("non-image body yields no thumbnail", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<html>not an image</html>"))
		}))
		defer srv.Close()

		if thumb, _, _ := fetchLinkThumbnail(srv.URL); len(thumb) != 0 {
			t.Error("an HTML body must not produce a thumbnail")
		}
	})
}

// probeVideo/makeVideoThumbnail shell out to ffprobe/ffmpeg. They are skipped
// when those are unavailable (e.g. a bare dev box) and exercised when present.
func TestProbeVideo(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	// Not real video bytes: the probe must fail cleanly rather than panic.
	if _, ok := probeVideo([]byte("not a video")); ok {
		t.Error("garbage must not probe as video")
	}
}
