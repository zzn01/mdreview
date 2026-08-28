package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testSource is a fabricated 9-line document backing the fixture server.
// Its lines deliberately avoid the short "q1"/"q2"/"q3" quotes used by
// the CRUD test, so refineLines is a no-op there; TestCreateCommentNarrowsLineRange
// uses a quote that does pin a single source line.
var testSource = []byte(strings.Join([]string{
	"Line one text",
	"Line two text",
	"Line three text",
	"Line four text",
	"Line five text",
	"Line six text",
	"Line seven text",
	"Line eight text",
	"Line nine text",
}, "\n"))

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv, err := NewServer("plan.md", template.HTML(`<div class="block" data-lines="1-1"><h1>T</h1></div>`), testSource)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func do(t *testing.T, method, url string, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(data)
}

func TestIndexServesRenderedDoc(t *testing.T) {
	_, ts := newTestServer(t)
	res, body := do(t, "GET", ts.URL+"/", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if !strings.Contains(body, `data-lines="1-1"`) || !strings.Contains(body, "plan.md") {
		t.Fatalf("page missing doc or title:\n%s", body)
	}
}

func TestCommentCRUDAndSubmit(t *testing.T) {
	srv, ts := newTestServer(t)

	// empty list is a JSON array, not null
	_, body := do(t, "GET", ts.URL+"/api/comments", "")
	if strings.TrimSpace(body) != "[]" {
		t.Fatalf("want [], got %q", body)
	}

	// create two comments, out of source order
	res, body := do(t, "POST", ts.URL+"/api/comments",
		`{"startLine":5,"endLine":6,"quote":"q2","body":"second"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create: status %d body %s", res.StatusCode, body)
	}
	var c2 Comment
	if err := json.Unmarshal([]byte(body), &c2); err != nil || c2.ID == 0 {
		t.Fatalf("create returned bad comment: %s", body)
	}
	_, body = do(t, "POST", ts.URL+"/api/comments",
		`{"startLine":1,"endLine":1,"quote":"q1","body":"frist"}`)
	var c1 Comment
	if err := json.Unmarshal([]byte(body), &c1); err != nil {
		t.Fatal(err)
	}

	// update the typo, delete nothing yet
	res, body = do(t, "PUT", ts.URL+"/api/comments/"+itoa(c1.ID), `{"body":"first"}`)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "first") {
		t.Fatalf("update: status %d body %s", res.StatusCode, body)
	}

	// create-and-delete a third
	_, body = do(t, "POST", ts.URL+"/api/comments",
		`{"startLine":9,"endLine":9,"quote":"q3","body":"drop me"}`)
	var c3 Comment
	if err := json.Unmarshal([]byte(body), &c3); err != nil {
		t.Fatal(err)
	}
	res, _ = do(t, "DELETE", ts.URL+"/api/comments/"+itoa(c3.ID), "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d", res.StatusCode)
	}

	// invalid verdict is rejected
	res, _ = do(t, "POST", ts.URL+"/api/submit", `{"verdict":"MAYBE"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad verdict: status %d", res.StatusCode)
	}

	// submit; Wait returns comments sorted by start line
	res, _ = do(t, "POST", ts.URL+"/api/submit",
		`{"verdict":"REQUEST_CHANGES","overall":"overall note"}`)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("submit: status %d", res.StatusCode)
	}
	review := srv.Wait()
	if review.Verdict != "REQUEST_CHANGES" || review.Overall != "overall note" {
		t.Fatalf("bad review: %+v", review)
	}
	if len(review.Comments) != 2 ||
		review.Comments[0].Body != "first" || review.Comments[1].Body != "second" {
		t.Fatalf("bad comments: %+v", review.Comments)
	}
}

// The frontend sends a block-granular range (the whole block's data-lines),
// but when the quote pins one line unambiguously the server should narrow
// the stored range to that line.
func TestCreateCommentNarrowsLineRange(t *testing.T) {
	_, ts := newTestServer(t)
	res, body := do(t, "POST", ts.URL+"/api/comments",
		`{"startLine":5,"endLine":6,"quote":"Line six text","body":"note"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create: status %d body %s", res.StatusCode, body)
	}
	var c Comment
	if err := json.Unmarshal([]byte(body), &c); err != nil {
		t.Fatal(err)
	}
	if c.StartLine != 6 || c.EndLine != 6 {
		t.Fatalf("want narrowed range 6-6, got %d-%d", c.StartLine, c.EndLine)
	}
}

func TestCreateCommentRejectsEmptyBody(t *testing.T) {
	_, ts := newTestServer(t)
	res, _ := do(t, "POST", ts.URL+"/api/comments",
		`{"startLine":1,"endLine":1,"quote":"q","body":"  "}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", res.StatusCode)
	}
}

// A cross-origin page (e.g. one open in another tab) could POST to
// /api/submit with a forged Origin to auto-approve a review the reviewer
// never saw. The guard must reject it before it reaches the handler.
func TestSubmitRejectsForgedOrigin(t *testing.T) {
	srv, ts := newTestServer(t)

	req, err := http.NewRequest("POST", ts.URL+"/api/submit", strings.NewReader(`{"verdict":"APPROVE"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "http://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", res.StatusCode)
	}

	select {
	case <-srv.done:
		t.Fatal("submit signaled despite forged Origin")
	case <-time.After(50 * time.Millisecond):
	}
}

// DNS rebinding lets an attacker-controlled page resolve a hostname to
// 127.0.0.1 after the fact, so the browser's same-origin check passes while
// the Host header still names the attacker's domain. The guard must reject
// any Host that is not loopback, independent of Origin.
func TestRejectsNonLoopbackHost(t *testing.T) {
	_, ts := newTestServer(t)

	req, err := http.NewRequest("GET", ts.URL+"/api/comments", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "evil.example"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", res.StatusCode)
	}
}

// The served page's own fetch calls carry an Origin matching the server's
// loopback address exactly; those must keep working.
func TestAcceptsMatchingOrigin(t *testing.T) {
	_, ts := newTestServer(t)

	req, err := http.NewRequest("GET", ts.URL+"/api/comments", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", ts.URL)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}
}

// Non-browser clients (curl, the documented smoke test) send no Origin
// header at all; those must be accepted as long as the Host is loopback.
func TestAcceptsRequestWithNoOrigin(t *testing.T) {
	_, ts := newTestServer(t)
	res, _ := do(t, "GET", ts.URL+"/api/comments", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}
}

const tunnelHostname = "review-abc.trycloudflare.com"

func newTunnelTestServer(t *testing.T) (*Server, *httptest.Server, string) {
	t.Helper()
	srv, err := NewServer("plan.md", template.HTML(`<div class="block" data-lines="1-1"><h1>T</h1></div>`), testSource)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	token, err := srv.EnableTunnel(tunnelHostname)
	if err != nil {
		t.Fatalf("EnableTunnel: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts, token
}

// In tunnel mode, a request with no token at all must be rejected: an
// unauthenticated visitor who merely guesses the trycloudflare.com hostname
// must not get access.
func TestTunnelNoTokenForbidden(t *testing.T) {
	_, ts, _ := newTunnelTestServer(t)
	res, _ := do(t, "GET", ts.URL+"/api/comments", "")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", res.StatusCode)
	}
}

// A wrong token (query param or cookie) must be rejected just like no token
// at all — otherwise the guard would leak whether a guessed token is "close".
func TestTunnelWrongTokenForbidden(t *testing.T) {
	_, ts, _ := newTunnelTestServer(t)
	res, _ := do(t, "GET", ts.URL+"/?t=0000000000000000000000000000000000", "")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", res.StatusCode)
	}

	req, err := http.NewRequest("GET", ts.URL+"/api/comments", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: tokenCookieName, Value: "not-the-token"})
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", res.StatusCode)
	}
}

// A correct "?t=" must set the cookie and redirect to a URL with the token
// stripped, so it doesn't linger in the address bar or browser history.
func TestTunnelQueryTokenRedirectsAndSetsCookie(t *testing.T) {
	_, ts, token := newTunnelTestServer(t)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := client.Get(ts.URL + "/?t=" + token)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status %d, want 302", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if strings.Contains(loc, "t=") {
		t.Fatalf("location still carries the token: %s", loc)
	}
	cookies := res.Cookies()
	if len(cookies) != 1 || cookies[0].Name != tokenCookieName || cookies[0].Value != token {
		t.Fatalf("bad Set-Cookie: %+v", cookies)
	}
}

// Once the cookie is set (as the redirect above does), subsequent requests
// must be let through without the query param.
func TestTunnelCookieAllowsAccess(t *testing.T) {
	_, ts, token := newTunnelTestServer(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	res, err := client.Get(ts.URL + "/?t=" + token) // follows the redirect, storing the cookie
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}

	res, err = client.Get(ts.URL + "/api/comments") // cookie-only, no query param
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}
}

// The tunnel hostname must be accepted as Host/Origin in tunnel mode (once
// authenticated), but a plain (non-tunnel) server must keep rejecting it —
// tunnel mode must not weaken the guard for reviewers who never opted in.
func TestTunnelHostAndOriginAcceptedOnlyInTunnelMode(t *testing.T) {
	_, tunnelTS, token := newTunnelTestServer(t)
	req, err := http.NewRequest("GET", tunnelTS.URL+"/api/comments", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = tunnelHostname
	req.Header.Set("Origin", "https://"+tunnelHostname)
	req.AddCookie(&http.Cookie{Name: tokenCookieName, Value: token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("tunnel mode: status %d, want 200", res.StatusCode)
	}

	_, plainTS := newTestServer(t)
	req2, err := http.NewRequest("GET", plainTS.URL+"/api/comments", nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Host = tunnelHostname
	req2.Header.Set("Origin", "https://"+tunnelHostname)
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.StatusCode != http.StatusForbidden {
		t.Fatalf("normal mode: status %d, want 403", res2.StatusCode)
	}
}

func itoa(n int) string {
	var b bytes.Buffer
	json.NewEncoder(&b).Encode(n)
	return strings.TrimSpace(b.String())
}
