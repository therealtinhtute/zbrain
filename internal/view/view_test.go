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
	"sync"
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
	defer func() { _ = resp.Body.Close() }()
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
		_ = resp.Body.Close()
	}
}

func TestViewRejectsMutations(t *testing.T) {
	srv := newTestServer(t)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions, http.MethodTrace, http.MethodConnect, "BREW"} {
		req, err := http.NewRequest(method, srv.URL+"/", nil)
		if err != nil {
			t.Fatalf("NewRequest(%s) error = %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s / error = %v", method, err)
		}
		_ = resp.Body.Close()
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
	defer func() { _ = resp.Body.Close() }()
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
	_ = resp.Body.Close()
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
	defer func() { _ = resp.Body.Close() }()
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
		_ = resp.Body.Close()
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
	defer func() { _ = wsResp.Body.Close() }()
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
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions, http.MethodTrace, http.MethodConnect, "BREW"} {
		req, err := http.NewRequest(method, srv.URL+"/", nil)
		if err != nil {
			t.Fatalf("NewRequest(%s) error = %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s / error = %v", method, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s / status = %d, want 405", method, resp.StatusCode)
		}
	}
}

type signalBuffer struct {
	mu   sync.Mutex
	buf  strings.Builder
	ch   chan struct{}
	once sync.Once
}

func newSignalBuffer() *signalBuffer {
	return &signalBuffer{ch: make(chan struct{})}
}

func (b *signalBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, err := b.buf.Write(p)
	b.once.Do(func() { close(b.ch) })
	return n, err
}

func (b *signalBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *signalBuffer) WaitForWrite(timeout time.Duration) bool {
	select {
	case <-b.ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestViewRunAndServeErrors(t *testing.T) {
	tmp := t.TempDir()
	paths, err := zruntime.ResolvePaths(zruntime.Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	// Serve before Listen should error
	srv := &Server{Stdout: io.Discard, Stderr: io.Discard, Paths: paths}
	if err := srv.Serve(); err == nil || !strings.Contains(err.Error(), "Serve called before Listen") {
		t.Fatalf("Serve(before Listen) error = %v, want Serve before Listen", err)
	}
	// Close without Listen should be nil
	if err := srv.Close(); err != nil {
		t.Fatalf("Close(no listener) error = %v", err)
	}
	// Run with temp ZBRAIN_HOME should print URL and serve
	buf := newSignalBuffer()
	srv2 := &Server{Stdout: buf, Stderr: io.Discard, Paths: paths}
	errCh := make(chan error, 1)
	go func() { errCh <- srv2.Run() }()
	if !buf.WaitForWrite(2 * time.Second) {
		t.Fatalf("Run() stdout = %q, want viewer URL", buf.String())
	}
	s := buf.String()
	var urlStr string
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "viewer:") {
			urlStr = strings.TrimSpace(strings.TrimPrefix(line, "viewer:"))
			break
		}
	}
	if urlStr == "" {
		t.Fatalf("Run() stdout = %q, want viewer URL", s)
	}
	// Verify URL is loopback
	addr, err := net.ResolveTCPAddr("tcp", strings.TrimPrefix(urlStr, "http://"))
	if err != nil {
		t.Fatalf("ResolveTCPAddr() error = %v", err)
	}
	if !addr.IP.IsLoopback() {
		t.Fatalf("Run bound to %v, want loopback", addr.IP)
	}
	// GET / should succeed via Run server
	resp, err := http.Get(urlStr + "/")
	if err != nil {
		t.Fatalf("GET / via Run error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / via Run status = %d, want 200", resp.StatusCode)
	}
	_ = srv2.Close()
	<-errCh
	// Run with nil Stdout should fallback to os.Stdout without panic (test via not crashing)
	srv3 := &Server{Stdout: nil, Stderr: io.Discard, Paths: paths}
	errCh2 := make(chan error, 1)
	go func() { errCh2 <- srv3.Run() }()
	time.Sleep(50 * time.Millisecond)
	_ = srv3.Close()
	<-errCh2
}

func TestViewStaticAndNotFound(t *testing.T) {
	srv := newTestServer(t)
	// index.html should be blocked (never serve raw)
	resp, err := http.Get(srv.URL + "/index.html")
	if err != nil {
		t.Fatalf("GET /index.html error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /index.html status = %d, want 404", resp.StatusCode)
	}
	// missing static should be 404
	resp, err = http.Get(srv.URL + "/missing.css")
	if err != nil {
		t.Fatalf("GET /missing.css error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /missing.css status = %d, want 404", resp.StatusCode)
	}
	// HEAD should succeed for existing static
	req, err := http.NewRequest(http.MethodHead, srv.URL+"/style.css", nil)
	if err != nil {
		t.Fatalf("NewRequest(HEAD) error = %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HEAD /style.css error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("HEAD /style.css status = %d, want 200", resp.StatusCode)
	}
	checkStrictHeaders(t, resp.Header)
}

func TestViewAPIErrorsAndHappyPath(t *testing.T) {
	// Setup workspace with one approved claim for happy path
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
	claimStore := zruntime.ClaimStore{Paths: paths, Now: func() time.Time { return now }}
	claim := zruntime.Claim{
		Type:      zruntime.OKFClaimType,
		ID:        "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Tier:      "projects",
		Status:    zruntime.ClaimStatusDraft,
		Title:     "API test",
		Basis:     zruntime.ClaimBasisOwner,
		CreatedAt: now.Format(time.RFC3339),
		CreatedBy: "owner",
		Body:      "body",
	}
	if _, err := claimStore.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := claimStore.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	// Add evidence
	source := filepath.Join(tmp, "src.txt")
	if err := os.WriteFile(source, []byte("evidence content"), 0o600); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	evidenceStore := zruntime.EvidenceStore{Paths: paths, Now: func() time.Time { return now }}
	evidence, err := evidenceStore.AddFile("research", source, "file://src.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}

	srv := &Server{Stdout: io.Discard, Stderr: io.Discard, Paths: paths}
	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()
	t.Cleanup(func() { _ = srv.Close(); <-errCh })

	// Happy paths
	for _, path := range []string{"/api/workspace", "/api/claims"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s error = %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, resp.StatusCode)
		}
		if !json.Valid(body) {
			t.Fatalf("GET %s invalid JSON: %s", path, body)
		}
		checkStrictHeaders(t, resp.Header)
	}
	// handleClaim happy and 404s
	resp, err := http.Get(srv.URL + "/api/claim/" + claim.ID)
	if err != nil {
		t.Fatalf("GET /api/claim valid error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/claim valid status = %d, want 200", resp.StatusCode)
	}
	for _, bad := range []string{"/api/claim/", "/api/claim/invalid", "/api/claim/" + claim.ID + "/extra", "/api/claim/notfound_aaaaaaaaaaaaaaaaaaaaaaaa"} {
		resp, err := http.Get(srv.URL + bad)
		if err != nil {
			t.Fatalf("GET %s error = %v", bad, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404", bad, resp.StatusCode)
		}
		var errBody map[string]string
		if err := json.Unmarshal(body, &errBody); err != nil || errBody["error"] == "" {
			t.Errorf("GET %s error body = %s, want JSON error", bad, body)
		}
		checkStrictHeaders(t, resp.Header)
	}
	// handleEvidence happy and 404s
	resp, err = http.Get(srv.URL + "/api/evidence/" + evidence.ID)
	if err != nil {
		t.Fatalf("GET /api/evidence valid error = %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/evidence valid status = %d, want 200", resp.StatusCode)
	}
	if !json.Valid(body) {
		t.Fatalf("GET /api/evidence valid invalid JSON")
	}
	for _, bad := range []string{"/api/evidence/", "/api/evidence/invalid", "/api/evidence/" + evidence.ID + "/extra", "/api/evidence/evd_ffffffffffffffffffffffffffffffff"} {
		resp, err := http.Get(srv.URL + bad)
		if err != nil {
			t.Fatalf("GET %s error = %v", bad, err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404", bad, resp.StatusCode)
		}
		var eb map[string]string
		if err := json.Unmarshal(b, &eb); err != nil || eb["error"] == "" {
			t.Errorf("GET %s error body = %s, want JSON error", bad, b)
		}
	}
	// handleWorkspace and handleClaims error when no workspace (missing config)
	tmp2 := t.TempDir()
	badPaths, err := zruntime.ResolvePaths(zruntime.Options{CWD: tmp2, HomeDir: tmp2, RuntimeDir: filepath.Join(tmp2, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths(bad) error = %v", err)
	}
	srvBad := &Server{Stdout: io.Discard, Stderr: io.Discard, Paths: badPaths}
	if err := srvBad.Listen(); err != nil {
		t.Fatalf("Listen(bad) error = %v", err)
	}
	errChBad := make(chan error, 1)
	go func() { errChBad <- srvBad.Serve() }()
	t.Cleanup(func() { _ = srvBad.Close(); <-errChBad })
	for _, path := range []string{"/api/workspace", "/api/claims", "/api/claim/" + claim.ID, "/api/evidence/" + evidence.ID} {
		resp, err := http.Get(srvBad.URL + path)
		if err != nil {
			t.Fatalf("GET %s (bad workspace) error = %v", path, err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("GET %s (bad workspace) status = %d, want 500", path, resp.StatusCode)
		}
		var eb map[string]string
		if err := json.Unmarshal(b, &eb); err != nil || eb["error"] == "" {
			t.Errorf("GET %s (bad) error body = %s, want JSON error", path, b)
		}
	}
	// handlePage 404 for non-root
	resp, err = http.Get(srv.URL + "/notfound")
	if err != nil {
		t.Fatalf("GET /notfound error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /notfound status = %d, want 404", resp.StatusCode)
	}
	// HEAD should work for root and apis
	req2, err2 := http.NewRequest(http.MethodHead, srv.URL+"/", nil)
	if err2 != nil {
		t.Fatalf("NewRequest(HEAD /) error = %v", err2)
	}
	resp, err = http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("HEAD / error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("HEAD / status = %d, want 200", resp.StatusCode)
	}
}

func TestViewPageError(t *testing.T) {
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
	// Create a scenario where renderPage fails: make wiki unreadable to cause WalkDir error
	workspaceRoot, err := zruntime.ValidateWorkspace(paths, "research")
	if err != nil {
		t.Fatalf("ValidateWorkspace() error = %v", err)
	}
	wikiPath := filepath.Join(workspaceRoot, "wiki")
	if err := os.Chmod(wikiPath, 0); err != nil {
		t.Fatalf("Chmod(wiki) error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(wikiPath, 0o700) })
	srv := &Server{Stdout: io.Discard, Stderr: io.Discard, Paths: paths}
	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()
	t.Cleanup(func() { _ = srv.Close(); <-errCh })
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET / (broken workspace) error = %v", err)
	}
	_ = resp.Body.Close()
	// Should be 500 because ScanWorkspaceForTrust fails
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("GET / (broken workspace) status = %d, want 500", resp.StatusCode)
	}
	checkStrictHeaders(t, resp.Header)
}

func TestViewHandleStaticDirect(t *testing.T) {
	tmp := t.TempDir()
	paths, err := zruntime.ResolvePaths(zruntime.Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	srv := &Server{Stdout: io.Discard, Stderr: io.Discard, Paths: paths}
	// Directly test handleStatic with index.html blocking
	req, _ := http.NewRequest(http.MethodGet, "/index.html", nil)
	rec := &mockResponseWriter{header: make(http.Header)}
	srv.handleStatic(rec, req)
	if rec.status != http.StatusNotFound {
		t.Errorf("handleStatic(index.html) status = %d, want 404", rec.status)
	}
	// Test handleStatic with missing file
	req, _ = http.NewRequest(http.MethodGet, "/missing.txt", nil)
	rec = &mockResponseWriter{header: make(http.Header)}
	srv.handleStatic(rec, req)
	if rec.status != http.StatusNotFound {
		t.Errorf("handleStatic(missing) status = %d, want 404", rec.status)
	}
	// Test handleStatic with valid file via direct call
	req, _ = http.NewRequest(http.MethodGet, "/style.css", nil)
	rec = &mockResponseWriter{header: make(http.Header)}
	srv.handleStatic(rec, req)
	if rec.status != http.StatusOK {
		t.Errorf("handleStatic(style.css) status = %d, want 200", rec.status)
	}
}

type mockResponseWriter struct {
	header http.Header
	status int
	body   []byte
}

func (m *mockResponseWriter) Header() http.Header { return m.header }
func (m *mockResponseWriter) Write(b []byte) (int, error) {
	if m.status == 0 {
		m.status = http.StatusOK
	}
	m.body = append(m.body, b...)
	return len(b), nil
}
func (m *mockResponseWriter) WriteHeader(statusCode int) { m.status = statusCode }
