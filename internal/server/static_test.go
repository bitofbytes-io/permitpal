package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestPageScriptsCarryTheCSPNonce(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	res := doGet(t, http.DefaultClient, ts.URL+"/login")
	csp := res.Header.Get("Content-Security-Policy")
	body := readBody(t, res)
	_, rest, ok := strings.Cut(csp, "'nonce-")
	nonce, _, _ := strings.Cut(rest, "'")
	if !ok || nonce == "" {
		t.Fatalf("CSP has no nonce: %q", csp)
	}
	// The username Tab handler and the Umami loader are the page's only scripts.
	if got := strings.Count(body, "<script"); got != 2 || strings.Count(body, `<script nonce="`+nonce+`">`) != 2 {
		t.Fatalf("want 2 inline scripts with nonce %q, got %d: %s", nonce, got, body)
	}
}

func TestStaticFilesServeWithoutDirectoryListings(t *testing.T) {
	t.Chdir("../..") // The server serves ./static, as in the image.
	ts := newTestServer(t)
	defer ts.Close()
	for path, want := range map[string]int{
		"/static/styles.css":                          http.StatusOK,
		"/static/assets/permitpal-logo-mark.png":      http.StatusOK,
		"/static/":                                    http.StatusNotFound,
		"/static/assets/":                             http.StatusNotFound,
		"/static/assets":                              http.StatusNotFound,
		"/static/assets/permitpal-logo-mark.png/oops": http.StatusNotFound,
	} {
		res := doGet(t, http.DefaultClient, ts.URL+path)
		body := readBody(t, res)
		if res.StatusCode != want || strings.Contains(body, "<a href=") {
			t.Fatalf("GET %s: status=%d, want %d; body=%.200s", path, res.StatusCode, want, body)
		}
	}
}
