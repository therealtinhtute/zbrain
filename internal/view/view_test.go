package view

import (
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

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
