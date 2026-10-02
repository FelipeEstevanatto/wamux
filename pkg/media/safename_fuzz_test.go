package media

import (
	"strings"
	"testing"
)

// FuzzSafeName probes the path-traversal guard that keeps a crafted media id
// from escaping the store directory. Invariant: anything safeName accepts must
// be free of path separators and traversal components.
func FuzzSafeName(f *testing.F) {
	for _, s := range []string{
		"", ".", "..", "...", "ok", "a/b", "a\\b", "../etc/passwd",
		"3EB0A1B2C3D4E5F6A7B8C9", "a..b", "..a", "a/../b",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, name string) {
		if !safeName(name) {
			return
		}
		if name == "" || name == "." || name == ".." {
			t.Fatalf("safeName accepted %q", name)
		}
		if strings.ContainsAny(name, `/\`) {
			t.Fatalf("safeName accepted a path separator in %q", name)
		}
	})
}
