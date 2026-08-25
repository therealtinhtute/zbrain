// Package view provides the loopback read-only viewer served by `zbrain view`.
package view

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	bundled "github.com/therealtinhtute/zbrain/assets/view"
	"github.com/therealtinhtute/zbrain/internal/runtime"
)

const securityPolicy = "default-src 'self'; script-src 'none'; object-src 'none'"

// Server serves the embedded viewer over loopback.
type Server struct {
	Stdout io.Writer
	Stderr io.Writer
	Paths  runtime.Paths
	Port   int
	URL    string

	mu       sync.Mutex
	listener net.Listener
}

// Listen binds loopback on an ephemeral port and records the bound address.
// It must be called before Serve.
func (s *Server) Listen() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = listener
	s.Port = listener.Addr().(*net.TCPAddr).Port
	s.URL = fmt.Sprintf("http://127.0.0.1:%d", s.Port)
	s.mu.Unlock()
	return nil
}

// Serve serves the embedded viewer on the bound listener and blocks until the
// server stops.
func (s *Server) Serve() error {
	s.mu.Lock()
	listener := s.listener
	s.mu.Unlock()
	if listener == nil {
		return errors.New("view: Serve called before Listen")
	}
	return http.Serve(listener, s.handler())
}

// Run binds loopback, prints the bound URL to Stdout, and blocks until the
// server stops.
func (s *Server) Run() error {
	if err := s.Listen(); err != nil {
		return err
	}
	s.mu.Lock()
	url := s.URL
	s.mu.Unlock()
	stdout := s.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	fmt.Fprintf(stdout, "viewer: %s\n", url)
	return s.Serve()
}

// Close stops the server.
func (s *Server) Close() error {
	s.mu.Lock()
	listener := s.listener
	s.mu.Unlock()
	if listener != nil {
		return listener.Close()
	}
	return nil
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/workspace", s.handleWorkspace)
	mux.HandleFunc("/api/claims", s.handleClaims)
	mux.HandleFunc("/api/claim/", s.handleClaim)
	mux.HandleFunc("/api/evidence/", s.handleEvidence)
	mux.HandleFunc("/style.css", s.handleStatic)
	mux.HandleFunc("/app.js", s.handleStatic)
	mux.HandleFunc("/", s.handlePage)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject every method except the read-only GET/HEAD allow-list.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			setSecurityHeaders(w)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Strict CSP, nosniff, and no CORS headers — loopback only.
		setSecurityHeaders(w)
		mux.ServeHTTP(w, r)
	})
}

// setSecurityHeaders applies the strict CSP and nosniff headers shared by
// every viewer response.
func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", securityPolicy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// pageData is the model rendered into the viewer page.
type pageData struct {
	Workspace string
	Claims    []claimView
}

type claimView struct {
	Claim    runtime.Claim
	Evidence []evidenceView
}

type evidenceView struct {
	Evidence runtime.Evidence
	Content  string
}

// evidenceResponse is the JSON body for GET /api/evidence/{id}.
type evidenceResponse struct {
	Evidence runtime.Evidence `json:"evidence"`
	Content  string           `json:"content"`
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	body, err := s.renderPage()
	if err != nil {
		http.Error(w, "viewer: page unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

// renderPage renders the viewer page. All user content is escaped by
// html/template; no raw Markdown or evidence bytes are emitted.
func (s *Server) renderPage() ([]byte, error) {
	current, err := runtime.ResolveCurrentWorkspace(s.Paths)
	data := pageData{}
	if err == nil {
		scan, scanErr := (runtime.ClaimStore{Paths: s.Paths}).ScanWorkspaceForTrust(current.Workspace)
		if scanErr != nil {
			return nil, scanErr
		}
		data.Workspace = current.Workspace
		data.Claims = s.approvedClaims(current.Workspace, scan.Claims)
	}
	tmpl, err := template.ParseFS(bundled.FS, "index.html")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *Server) approvedClaims(workspace string, claims []runtime.Claim) []claimView {
	evidenceStore := runtime.EvidenceStore{Paths: s.Paths}
	views := make([]claimView, 0, len(claims))
	for _, claim := range claims {
		if claim.Status != runtime.ClaimStatusApproved {
			continue
		}
		cv := claimView{Claim: claim}
		for _, id := range claim.EvidenceIDs {
			evidence, err := evidenceStore.Read(workspace, id)
			if err != nil {
				continue
			}
			content, err := evidenceStore.ReadRaw(workspace, id)
			if err != nil {
				continue
			}
			cv.Evidence = append(cv.Evidence, evidenceView{Evidence: evidence, Content: string(content)})
		}
		views = append(views, cv)
	}
	return views
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "index.html" {
		// index.html is the server-rendered page template; never serve it raw.
		http.NotFound(w, r)
		return
	}
	data, err := bundled.FS.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, path, time.Time{}, bytes.NewReader(data))
}

func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	current, err := runtime.ResolveCurrentWorkspace(s.Paths)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"workspace": current.Workspace})
}

func (s *Server) handleClaims(w http.ResponseWriter, r *http.Request) {
	current, err := runtime.ResolveCurrentWorkspace(s.Paths)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	scan, err := (runtime.ClaimStore{Paths: s.Paths}).ScanWorkspaceForTrust(current.Workspace)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	claims := make([]runtime.Claim, 0, len(scan.Claims))
	for _, claim := range scan.Claims {
		if claim.Status == runtime.ClaimStatusApproved {
			claims = append(claims, claim)
		}
	}
	writeJSON(w, http.StatusOK, claims)
}

func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/claim/")
	if id == "" || strings.Contains(id, "/") {
		writeJSONError(w, http.StatusNotFound, "claim not found")
		return
	}
	current, err := runtime.ResolveCurrentWorkspace(s.Paths)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	claim, err := (runtime.ClaimStore{Paths: s.Paths}).Read(current.Workspace, id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "claim not found")
		return
	}
	writeJSON(w, http.StatusOK, claim)
}

func (s *Server) handleEvidence(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/evidence/")
	if id == "" || strings.Contains(id, "/") {
		writeJSONError(w, http.StatusNotFound, "evidence not found")
		return
	}
	current, err := runtime.ResolveCurrentWorkspace(s.Paths)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	evidenceStore := runtime.EvidenceStore{Paths: s.Paths}
	evidence, err := evidenceStore.Read(current.Workspace, id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "evidence not found")
		return
	}
	content, err := evidenceStore.ReadRaw(current.Workspace, id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, evidenceResponse{Evidence: evidence, Content: string(content)})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
