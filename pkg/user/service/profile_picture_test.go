package user_service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pictureServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	// The real client refuses local addresses.
	previous := profilePictureHTTPClient
	profilePictureHTTPClient = srv.Client()
	t.Cleanup(func() { profilePictureHTTPClient = previous })
	return srv.URL
}

func TestFetchProfilePicture(t *testing.T) {
	url := pictureServer(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("image")) })
	got, err := fetchProfilePicture(url)
	if err != nil || string(got) != "image" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestFetchProfilePictureRefusesOversizedImage(t *testing.T) {
	url := pictureServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write(bytes.Repeat([]byte{1}, maxProfilePictureBytes+1))
	})
	if _, err := fetchProfilePicture(url); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v", err)
	}
}

// An error page must not be uploaded as the profile picture.
func TestFetchProfilePictureRefusesErrorStatus(t *testing.T) {
	url := pictureServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("<html>not found</html>"))
	})
	if _, err := fetchProfilePicture(url); err == nil {
		t.Fatal("expected an error")
	}
}
