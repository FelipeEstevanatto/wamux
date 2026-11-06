package media

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const (
	previewSuffix  = ".preview.jpg"
	previewWidth   = 400
	previewTimeout = 30 * time.Second
)

// ErrPreviewUnavailable means a preview cannot be made here, which the caller shows
// as no preview rather than as a failure.
var ErrPreviewUnavailable = errors.New("preview unavailable")

var (
	// pdftoppmPath is a variable so tests can run without poppler-utils.
	pdftoppmPath = func() string {
		path, err := exec.LookPath("pdftoppm")
		if err != nil {
			return ""
		}
		return path
	}()

	// previewSlots limits how many renders run at once. Opening a chat full of PDFs
	// must not start one process per file.
	previewSlots = make(chan struct{}, 2)
)

// OpenPDFPreview returns a JPEG of a stored PDF's first page, rendering it the first
// time it is asked for. Nothing is spent on PDFs nobody opens, and files stored before
// this existed get a preview too. It sits next to the PDF, so retention removes it.
func OpenPDFPreview(instanceID, messageID string) (*os.File, fs.FileInfo, error) {
	s := current()
	if s == nil {
		return nil, nil, ErrNotConfigured
	}
	if !safeName(instanceID) || !safeName(messageID) {
		return nil, nil, errors.New("invalid media id")
	}
	source := filepath.Join(s.root, instanceID, messageID)
	preview := source + previewSuffix

	if file, info, err := openPreview(preview); err == nil {
		return file, info, nil
	}
	if pdftoppmPath == "" {
		return nil, nil, ErrPreviewUnavailable
	}

	previewSlots <- struct{}{}
	defer func() { <-previewSlots }()

	// Another request may have finished it while this one waited for a slot.
	if file, info, err := openPreview(preview); err == nil {
		return file, info, nil
	}
	if err := renderPDFPreview(source, preview); err != nil {
		return nil, nil, err
	}
	return openPreview(preview)
}

func openPreview(path string) (*os.File, fs.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

// renderPDFPreview writes next to the target and renames, so a reader never sees half
// a file, and two renders of one PDF cannot corrupt each other.
func renderPDFPreview(source, target string) error {
	temp, err := os.MkdirTemp(filepath.Dir(target), ".preview-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)

	ctx, cancel := context.WithTimeout(context.Background(), previewTimeout)
	defer cancel()
	prefix := filepath.Join(temp, "page")
	cmd := exec.CommandContext(ctx, pdftoppmPath,
		"-jpeg", "-singlefile", "-f", "1", "-l", "1",
		"-scale-to-x", strconv.Itoa(previewWidth), "-scale-to-y", "-1",
		source, prefix,
	)
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %v", ErrPreviewUnavailable, err)
	}
	return os.Rename(prefix+".jpg", target)
}
