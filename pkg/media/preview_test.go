package media

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRenderer stands in for pdftoppm and counts its runs in a file.
func fakeRenderer(t *testing.T, script string) (runs func() int) {
	t.Helper()
	dir := t.TempDir()
	counter := filepath.Join(dir, "runs")
	path := filepath.Join(dir, "pdftoppm")
	body := "#!/bin/sh\necho x >> " + counter + "\n" + script + "\n"
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	previous := pdftoppmPath
	pdftoppmPath = path
	t.Cleanup(func() { pdftoppmPath = previous })
	return func() int {
		data, _ := os.ReadFile(counter)
		return strings.Count(string(data), "x")
	}
}

const writesAPreview = `for last; do :; done; printf preview > "$last.jpg"`

func storedPDF(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "media")
	if err := Configure(root); err != nil {
		t.Fatal(err)
	}
	if err := Save("inst", "doc", []byte("%PDF-1.4")); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPreviewIsRenderedOnceAndReused(t *testing.T) {
	storedPDF(t)
	runs := fakeRenderer(t, writesAPreview)

	for range 3 {
		file, _, err := OpenPDFPreview("inst", "doc")
		if err != nil {
			t.Fatalf("OpenPDFPreview: %v", err)
		}
		content, _ := io.ReadAll(file)
		file.Close()
		if string(content) != "preview" {
			t.Fatalf("content = %q", content)
		}
	}

	if got := runs(); got != 1 {
		t.Fatalf("rendered %d times", got)
	}
}

func TestPreviewFailureLeavesNothingBehind(t *testing.T) {
	root := storedPDF(t)
	fakeRenderer(t, "exit 1")

	_, _, err := OpenPDFPreview("inst", "doc")

	if !errors.Is(err, ErrPreviewUnavailable) {
		t.Fatalf("err = %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "inst"))
	if len(entries) != 1 {
		t.Fatalf("left behind: %v", entries)
	}
}

func TestPreviewWithoutTheToolIsUnavailable(t *testing.T) {
	storedPDF(t)
	previous := pdftoppmPath
	pdftoppmPath = ""
	t.Cleanup(func() { pdftoppmPath = previous })

	if _, _, err := OpenPDFPreview("inst", "doc"); !errors.Is(err, ErrPreviewUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestPreviewRefusesUnsafeIDs(t *testing.T) {
	storedPDF(t)
	runs := fakeRenderer(t, writesAPreview)

	for _, id := range []string{"../doc", "a/b", "..", ""} {
		if _, _, err := OpenPDFPreview("inst", id); err == nil {
			t.Errorf("%q: want an error", id)
		}
	}
	if runs() != 0 {
		t.Fatal("rendered an unsafe id")
	}
}

func TestRemoveTakesThePreviewToo(t *testing.T) {
	root := storedPDF(t)
	fakeRenderer(t, writesAPreview)
	file, _, err := OpenPDFPreview("inst", "doc")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()

	if err := Remove("inst", "doc"); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(filepath.Join(root, "inst"))
	if len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
}

// With poppler-utils installed, a real file: a broken PDF must fail cleanly.
func TestRealRendererRefusesABrokenPDF(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm is not installed")
	}
	storedPDF(t)

	if _, _, err := OpenPDFPreview("inst", "doc"); !errors.Is(err, ErrPreviewUnavailable) {
		t.Fatalf("err = %v", err)
	}
}
