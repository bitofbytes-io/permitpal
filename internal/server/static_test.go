package server

import (
	"encoding/json"
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

func TestLoginPageLinksTheWebAppManifestAndIcons(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	body := readBody(t, doGet(t, http.DefaultClient, ts.URL+"/login"))
	for _, want := range []string{
		`<link rel="manifest" href="/static/site.webmanifest"`,
		`<link rel="apple-touch-icon" href="/static/icons/apple-touch-icon.png"`,
		`<meta name="theme-color" content="#`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("login page is missing %q: %s", want, body)
		}
	}
}

func TestWebAppManifestIsServedWithItsIcons(t *testing.T) {
	t.Chdir("../..") // The server serves ./static, as in the image.
	ts := newTestServer(t)
	defer ts.Close()

	res := doGet(t, http.DefaultClient, ts.URL+"/static/site.webmanifest")
	raw := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("manifest status = %d, want 200", res.StatusCode)
	}
	if got, _, _ := strings.Cut(res.Header.Get("Content-Type"), ";"); got != "application/manifest+json" {
		t.Fatalf("manifest Content-Type = %q, want application/manifest+json", got)
	}

	var manifest struct {
		Name       string `json:"name"`
		StartURL   string `json:"start_url"`
		Display    string `json:"display"`
		ThemeColor string `json:"theme_color"`
		Icons      []struct {
			Src   string `json:"src"`
			Sizes string `json:"sizes"`
			Type  string `json:"type"`
		} `json:"icons"`
	}
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if manifest.Name == "" || manifest.StartURL != "/" || manifest.Display != "standalone" || manifest.ThemeColor == "" {
		t.Fatalf("manifest is missing installability fields: %+v", manifest)
	}
	sizes := map[string]bool{}
	for _, icon := range manifest.Icons {
		sizes[icon.Sizes] = true
		res := doGet(t, http.DefaultClient, ts.URL+icon.Src)
		_ = readBody(t, res)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != icon.Type {
			t.Fatalf("GET %s: status=%d type=%q, want 200 %q", icon.Src, res.StatusCode, res.Header.Get("Content-Type"), icon.Type)
		}
	}
	if !sizes["192x192"] || !sizes["512x512"] {
		t.Fatalf("manifest needs 192x192 and 512x512 icons, got %v", sizes)
	}

	res = doGet(t, http.DefaultClient, ts.URL+"/static/icons/apple-touch-icon.png")
	_ = readBody(t, res)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("apple-touch-icon: status=%d type=%q", res.StatusCode, res.Header.Get("Content-Type"))
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
