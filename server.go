package main

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
)

//go:embed ui
var uiFS embed.FS

// Server holds the state of one review session: the rendered page, the
// source lines (for narrowing comment ranges), and the reviewer's draft
// comments. State lives in memory only; the process serves exactly one
// review and exits after submit.
type Server struct {
	page  []byte
	lines []string
	done  chan Review
	once  sync.Once

	mu       sync.Mutex
	nextID   int
	comments []Comment

	// tunnelHost and token are set by EnableTunnel when the review is
	// exposed remotely via a Cloudflare quick tunnel. token is empty
	// (token mode off) unless EnableTunnel has been called; in that
	// state every request must present it, and tunnelHost is additionally
	// accepted alongside loopback by the origin/host guard.
	tunnelHost string
	token      string
}

func NewServer(title string, doc template.HTML, source []byte) (*Server, error) {
	tmpl, err := template.ParseFS(uiFS, "ui/index.html")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	data := struct {
		Title string
		Doc   template.HTML
	}{title, doc}
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return &Server{
		page:     buf.Bytes(),
		lines:    strings.Split(string(source), "\n"),
		done:     make(chan Review, 1),
		nextID:   1,
		comments: []Comment{},
	}, nil
}

// Wait blocks until the reviewer submits and returns the review.
func (s *Server) Wait() Review { return <-s.done }

// EnableTunnel switches the server into token-auth mode for a review
// exposed remotely at hostname (the Cloudflare quick tunnel's
// *.trycloudflare.com host): it generates a random session token that every
// request must then present (query param or cookie, enforced by
// tokenGuard), and additionally accepts hostname alongside loopback in the
// origin/host guard. Returns the generated token, which the caller embeds
// in the URLs it prints. Must be called before Handler().
func (s *Server) EnableTunnel(hostname string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s.token = hex.EncodeToString(b)
	s.tunnelHost = hostname
	return s.token, nil
}

func (s *Server) Handler() http.Handler {
	static, _ := fs.Sub(uiFS, "ui")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /api/comments", s.listComments)
	mux.HandleFunc("POST /api/comments", s.createComment)
	mux.HandleFunc("PUT /api/comments/{id}", s.updateComment)
	mux.HandleFunc("DELETE /api/comments/{id}", s.deleteComment)
	mux.HandleFunc("POST /api/submit", s.submit)
	var h http.Handler = mux
	if s.token != "" {
		h = s.tokenGuard(h)
	}
	return originHostGuard(h, s.tunnelHost)
}

const tokenCookieName = "mdreview_token"

// tokenGuard enforces token-mode auth: a request must present the session
// token either as the "t" query param or as the mdreview_token cookie. A
// valid "?t=" sets the cookie and redirects to the same URL with the token
// stripped, so it doesn't linger in the address bar or browser history. A
// valid cookie passes through unchanged. Anything else is rejected with 403.
func (s *Server) tokenGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query().Get("t"); q != "" {
			if !validToken(q, s.token) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     tokenCookieName,
				Value:    s.token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
			stripped := *r.URL
			vals := stripped.Query()
			vals.Del("t")
			stripped.RawQuery = vals.Encode()
			http.Redirect(w, r, stripped.RequestURI(), http.StatusFound)
			return
		}
		if c, err := r.Cookie(tokenCookieName); err == nil && validToken(c.Value, s.token) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	})
}

// validToken reports whether candidate matches want, in constant time.
func validToken(candidate, want string) bool {
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(want)) == 1
}

// originHostGuard blocks cross-origin browser requests and DNS-rebinding
// attacks against the review server. The page is normally only ever served
// from a 127.0.0.1 loopback origin, so any request naming a different Host,
// or carrying an Origin header for a different host, cannot be a legitimate
// same-origin request from the served page and is rejected. Requests with
// no Origin header (curl, other non-browser clients) are allowed through
// as long as the Host is loopback, so the documented curl smoke test keeps
// working.
//
// When tunnelHost is non-empty (tunnel mode), it names an additional host
// that is accepted alongside loopback: the Cloudflare quick tunnel serves
// the page over https at that hostname, so both its Host header and its
// "https://<tunnelHost>" Origin must pass too.
func originHostGuard(next http.Handler, tunnelHost string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) && !(tunnelHost != "" && hostMatches(r.Host, tunnelHost)) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" &&
			!isLoopbackOrigin(origin) && !(tunnelHost != "" && isTunnelOrigin(origin, tunnelHost)) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackOrigin reports whether origin (a browser Origin header, e.g.
// "http://127.0.0.1:4999") names a loopback host over plain http.
func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}
	return isLoopbackHost(u.Host)
}

// isTunnelOrigin reports whether origin is "https://<tunnelHost>" (any
// port, though the quick tunnel never uses one).
func isTunnelOrigin(origin, tunnelHost string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" {
		return false
	}
	return hostMatches(u.Host, tunnelHost)
}

// isLoopbackHost reports whether host (a Host header or URL host, with or
// without a port) names 127.0.0.1, ::1, or localhost.
func isLoopbackHost(host string) bool {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	} else {
		hostname = strings.Trim(hostname, "[]")
	}
	switch hostname {
	case "127.0.0.1", "::1", "localhost":
		return true
	default:
		return false
	}
}

// hostMatches reports whether host (a Host header or URL host, with or
// without a port) names the given hostname.
func hostMatches(host, name string) bool {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	return hostname == name
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(s.page)
}

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.comments)
}

func (s *Server) createComment(w http.ResponseWriter, r *http.Request) {
	var c Comment
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil ||
		strings.TrimSpace(c.Body) == "" || c.StartLine < 1 || c.EndLine < c.StartLine {
		http.Error(w, "invalid comment", http.StatusBadRequest)
		return
	}
	c.StartLine, c.EndLine = refineLines(s.lines, c.StartLine, c.EndLine, c.Quote)
	s.mu.Lock()
	c.ID = s.nextID
	s.nextID++
	s.comments = append(s.comments, c)
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) updateComment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Body) == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.comments {
		if s.comments[i].ID == id {
			s.comments[i].Body = in.Body
			writeJSON(w, http.StatusOK, s.comments[i])
			return
		}
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.comments = slices.DeleteFunc(s.comments, func(c Comment) bool { return c.ID == id })
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Verdict string `json:"verdict"`
		Overall string `json:"overall"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil ||
		(in.Verdict != "APPROVE" && in.Verdict != "REQUEST_CHANGES") {
		http.Error(w, "invalid submit", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	comments := slices.Clone(s.comments)
	s.mu.Unlock()
	slices.SortStableFunc(comments, func(a, b Comment) int {
		return cmp.Compare(a.StartLine, b.StartLine)
	})
	// Respond before signaling: main exits right after Wait returns,
	// and the browser should still receive the 204. WriteHeader only
	// queues the response, so flush explicitly before unblocking main.
	w.WriteHeader(http.StatusNoContent)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	s.once.Do(func() {
		s.done <- Review{Verdict: in.Verdict, Overall: in.Overall, Comments: comments}
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
