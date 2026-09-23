package send_service

import (
	"bytes"
	"io"
	"testing"
)

func TestReadAllLimitedUnderLimit(t *testing.T) {
	want := bytes.Repeat([]byte("x"), 1024)
	got, err := readAllLimited(bytes.NewReader(want))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("data changed")
	}
}

// infiniteReader yields n bytes without allocating them all up front.
type infiniteReader struct{ n int64 }

func (r *infiniteReader) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.n {
		p = p[:r.n]
	}
	for i := range p {
		p[i] = 'a'
	}
	r.n -= int64(len(p))
	return len(p), nil
}

func TestReadAllLimitedRejectsOversize(t *testing.T) {
	if _, err := readAllLimited(&infiniteReader{n: maxRemoteMediaBytes + 1}); err == nil {
		t.Fatal("expected an error for a body over the limit")
	}
}
