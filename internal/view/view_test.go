package view

import (
	"encoding/json"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	tmp := t.TempDir()
	paths, err := zruntime.ResolvePaths(zruntime.Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	srv := &Server{
		Stdout: io.Discard,
		Stderr: io.Discard,
		Paths:  paths,
	}
	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve()
	}()
	t.Cleanup(func() {
		_ = srv.Close()
		<-errCh
	})
	return srv
}

func TestViewHeaders(t *testing.T) {
	srv := newTestServer(t)

	// Loopback-only bind.
	addr, err := net.ResolveTCPAddr("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("ResolveTCPAddr() error = %v", err)
	}
	if !addr.IP.IsLoopback() {
		t.Fatalf("server bound to %v, want loopback", addr.IP)
	}

	// Root serves index.html.
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", resp.StatusCode)
	}
	checkStrictHeaders(t, resp.Header)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !strings.Contains(string(body), "zbrain") {
		t.Fatalf("index.html body does not mention zbrain")
	}
}

func TestViewServesEmbeddedAssets(t *testing.T) {
	srv := newTestServer(t)

	for _, path := range []string{"/style.css", "/app.js"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s error = %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, resp.StatusCode)
		}
		checkStrictHeaders(t, resp.Header)
		resp.Body.Close()
	}
}

func TestViewRejectsMutations(t *testing.T) {
	srv := newTestServer(t)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req, err := http.NewRequest(method, srv.URL+"/", nil)
		if err != nil {
			t.Fatalf("NewRequest(%s) error = %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s / error = %v", method, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s / status = %d, want 405", method, resp.StatusCode)
		}
	}
}

func TestViewNoCORS(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	defer resp.Body.Close()
	for name := range resp.Header {
		if strings.HasPrefix(strings.ToLower(name), "access-control-") {
			t.Errorf("unexpected CORS header %q", name)
		}
	}
}

func TestViewNotFound(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/missing.html")
	if err != nil {
		t.Fatalf("GET /missing.html error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /missing.html status = %d, want 404", resp.StatusCode)
	}
}

func checkStrictHeaders(t *testing.T, header http.Header) {
	t.Helper()
	if got := header.Get("Content-Security-Policy"); got != "default-src 'self'; script-src 'none'; object-src 'none'" {
		t.Errorf("Content-Security-Policy = %q, want strict CSP", got)
	}
	if got := header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestViewEscaping(t *testing.T) {
	tmp := t.TempDir()
	paths, err := zruntime.ResolvePaths(zruntime.Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if _, err := zruntime.EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	now := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	if err := zruntime.CreateWorkspace(paths, "research", now); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	// Add evidence whose raw content contains HTML.
	source := filepath.Join(tmp, "source.html")
	rawHTML := "<h1>Evidence header</h1>\n<p>raw <b>bold</b> text</p>"
	if err := os.WriteFile(source, []byte(rawHTML), 0o600); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	evidenceStore := zruntime.EvidenceStore{Paths: paths, Now: func() time.Time { return now }}
	evidence, err := evidenceStore.AddFile("research", source, "file://source.html", "text/html")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}

	// Draft and approve a claim whose body carries a script tag.
	claimStore := zruntime.ClaimStore{Paths: paths, Now: func() time.Time { return now }}
	claim := zruntime.Claim{
		Type:        zruntime.OKFClaimType,
		ID:          "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Tier:        "projects",
		Status:      zruntime.ClaimStatusDraft,
		Title:       "Escaping claim",
		Basis:       zruntime.ClaimBasisEvidence,
		CreatedAt:   now.Format(time.RFC3339),
		CreatedBy:   "owner",
		Body:        "Safe text <script>alert('xss')</script>\n<img src=x onerror=alert(1)>",
		EvidenceIDs: []string{evidence.ID},
	}
	if _, err := claimStore.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := claimStore.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	srv := &Server{Stdout: io.Discard, Stderr: io.Discard, Paths: paths}
	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()
	t.Cleanup(func() { _ = srv.Close(); <-errCh })

	// The rendered page escapes both the claim body and the evidence content.
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", resp.StatusCode)
	}
	page, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll(page) error = %v", err)
	}
	pageText := string(page)

	escapedBody := html.EscapeString(claim.Body)
	if !strings.Contains(pageText, escapedBody) {
		t.Errorf("claim body not escaped in rendered page; want substring %q", escapedBody)
	}
	if strings.Contains(pageText, "<script>alert('xss')</script>") {
		t.Errorf("raw claim script tag present in rendered page")
	}
	escapedEvidence := html.EscapeString(rawHTML)
	if !strings.Contains(pageText, escapedEvidence) {
		t.Errorf("evidence content not escaped in rendered page; want substring %q", escapedEvidence)
	}
	if strings.Contains(pageText, "<h1>Evidence header</h1>") {
		t.Errorf("raw evidence HTML present in rendered page")
	}

	// API endpoints return valid JSON.
	for _, path := range []string{"/api/workspace", "/api/claims", "/api/claim/" + claim.ID, "/api/evidence/" + evidence.ID} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s error = %v", path, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			t.Fatalf("ReadAll(%s) error = %v", path, readErr)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, resp.StatusCode)
		}
		if !json.Valid(body) {
			t.Errorf("GET %s returned invalid JSON: %s", path, body)
		}
	}

	// Workspace JSON reports the configured workspace.
	wsResp, err := http.Get(srv.URL + "/api/workspace")
	if err != nil {
		t.Fatalf("GET /api/workspace error = %v", err)
	}
	defer wsResp.Body.Close()
	var ws struct {
		Workspace string `json:"workspace"`
	}
	if err := json.NewDecoder(wsResp.Body).Decode(&ws); err != nil {
		t.Fatalf("Decode(/api/workspace) error = %v", err)
	}
	if ws.Workspace != "research" {
		t.Errorf("workspace = %q, want %q", ws.Workspace, "research")
	}

	// Mutations are still rejected.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req, err := http.NewRequest(method, srv.URL+"/", nil)
		if err != nil {
			t.Fatalf("NewRequest(%s) error = %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s / error = %v", method, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s / status = %d, want 405", method, resp.StatusCode)
		}
	}
}
