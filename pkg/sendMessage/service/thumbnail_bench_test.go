package send_service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func benchImageBytes(b *testing.B) []byte {
	b.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4000, 3000))
	// Fill with a gradient so the encoder has real work to do.
	for y := 0; y < 3000; y++ {
		for x := 0; x < 4000; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x ^ y), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		b.Fatal(err)
	}
	return buf.Bytes()
}

// BenchmarkMakeJPEGThumbnail measures generating a 72px thumbnail from a
// 4000x3000 source, the size a phone photo has.
func BenchmarkMakeJPEGThumbnail(b *testing.B) {
	data := benchImageBytes(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if thumb := makeJPEGThumbnail(data, 72); thumb == nil {
			b.Fatal("nil thumbnail")
		}
	}
}
