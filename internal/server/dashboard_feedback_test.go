package server

import (
	"context"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestDashboardSaveFeedbackInBrowser drives the dashboard forms in Chromium
// with the bundled htmx. It needs Node and Playwright, so it runs only when
// PERMITPAL_PLAYWRIGHT_MODULE names the Playwright module to require.
func TestDashboardSaveFeedbackInBrowser(t *testing.T) {
	if os.Getenv("PERMITPAL_PLAYWRIGHT_MODULE") == "" {
		t.Skip("PERMITPAL_PLAYWRIGHT_MODULE is not set; set it to a Playwright module path to run the browser test")
	}
	t.Chdir("../..") // The page loads htmx from ./static.
	ts := newTestServer(t)
	defer ts.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, "node", "internal/server/testdata/dashboard_feedback.cjs", ts.URL).CombinedOutput()
	if err != nil {
		t.Fatalf("browser check failed: %v\n%s", err, output)
	}
}

func TestDashboardScriptsCarryTheCSPNonce(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	client := ts.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	closeBody(t, doPostForm(t, client, ts.URL+"/login", url.Values{"username": {"aiden"}, "password": {"test-password"}}))
	res := doGet(t, client, ts.URL+"/")
	csp := res.Header.Get("Content-Security-Policy")
	body := readBody(t, res)
	_, rest, ok := strings.Cut(csp, "'nonce-")
	nonce, _, _ := strings.Cut(rest, "'")
	if !ok || nonce == "" {
		t.Fatalf("CSP has no nonce: %q", csp)
	}
	// Scripts with a src must be same-origin; every inline script must carry
	// this response's nonce.
	scriptTag := regexp.MustCompile(`(?i)<script\b([^>]*)>`)
	attr := regexp.MustCompile(`([a-zA-Z-]+)(?:="([^"]*)")?`)
	inline := 0
	for _, tag := range scriptTag.FindAllStringSubmatch(body, -1) {
		attrs := map[string]string{}
		for _, a := range attr.FindAllStringSubmatch(tag[1], -1) {
			attrs[strings.ToLower(a[1])] = a[2]
		}
		if src, ok := attrs["src"]; ok {
			if !strings.HasPrefix(src, "/") || strings.HasPrefix(src, "//") {
				t.Fatalf("script %s does not load from this origin", tag[0])
			}
			continue
		}
		inline++
		if attrs["nonce"] != nonce {
			t.Fatalf("inline script %s does not carry nonce %q", tag[0], nonce)
		}
	}
	if inline == 0 {
		t.Fatal("dashboard rendered no inline scripts to check")
	}
}
