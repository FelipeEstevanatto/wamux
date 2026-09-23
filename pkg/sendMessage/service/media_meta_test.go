package send_service

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
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

func TestIsGIF(t *testing.T) {
	if !isGIF([]byte("GIF89a....")) {
		t.Error("GIF89a must be detected")
	}
	if !isGIF([]byte("GIF87a....")) {
		t.Error("GIF87a must be detected")
	}
	if isGIF([]byte("not a gif")) {
		t.Error("non-GIF must not be detected")
	}
	if isGIF(nil) {
		t.Error("nil must not be detected")
	}
}

// buildTestGIF returns a tiny two-frame animated GIF.
func buildTestGIF(t *testing.T) []byte {
	t.Helper()
	pal := color.Palette{color.RGBA{0, 0, 0, 255}, color.RGBA{255, 255, 255, 255}, color.RGBA{255, 0, 0, 255}}
	f1 := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
	f2 := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
	for i := range f1.Pix {
		f1.Pix[i] = uint8(i % 3)
		f2.Pix[i] = uint8((i + 1) % 3)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{f1, f2}, Delay: []int{10, 10}}); err != nil {
		t.Fatalf("gif encode: %v", err)
	}
	return buf.Bytes()
}

func TestConvertGifToMP4(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	raw := buildTestGIF(t)
	if !isGIF(raw) {
		t.Fatal("test fixture is not a GIF")
	}

	mp4, err := convertGifToMP4(raw)
	if err != nil {
		t.Fatalf("convertGifToMP4: %v", err)
	}
	if len(mp4) < 12 || string(mp4[4:8]) != "ftyp" {
		t.Fatalf("output is not an MP4 (first bytes: %q)", mp4[:min(12, len(mp4))])
	}
	// The MP4 must probe as a video.
	if _, ok := probeVideo(mp4); !ok {
		t.Error("converted MP4 does not probe as video")
	}
}
