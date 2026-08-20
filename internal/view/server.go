// Package view provides the loopback read-only viewer served by `zbrain view`.
package view

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	bundled "github.com/therealtinhtute/zbrain/assets/view"
	"github.com/therealtinhtute/zbrain/internal/runtime"
)

// Server serves the embedded viewer over loopback.
type Server struct {
	Stdout io.Writer
	Stderr io.Writer
	Paths  runtime.Paths
	Port   int
	URL    string

	listener net.Listener
}

// Listen binds loopback on an ephemeral port and records the bound address.
// It must be called before Serve.
func (s *Server) Listen() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.listener = listener
	s.Port = listener.Addr().(*net.TCPAddr).Port
	s.URL = fmt.Sprintf("http://127.0.0.1:%d", s.Port)
	return nil
}

// Serve serves the embedded viewer on the bound listener and blocks until the
// server stops.
func (s *Server) Serve() error {
	if s.listener == nil {
		return errors.New("view: Serve called before Listen")
	}
	return http.Serve(s.listener, s.handler())
}

// Run binds loopback, prints the bound URL to Stdout, and blocks until the
// server stops.
func (s *Server) Run() error {
	if err := s.Listen(); err != nil {
		return err
	}
	stdout := s.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	fmt.Fprintf(stdout, "viewer: %s\n", s.URL)
	return s.Serve()
}

// Close stops the server.
func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject mutation methods — 405 for every non-GET/HEAD.
		switch r.Method {
		case http.MethodPut, http.MethodPost, http.MethodDelete, http.MethodPatch:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Strict CSP: no scripts, no objects.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'none'; object-src 'none'")
		// Nosniff: prevent MIME-type sniffing.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// No CORS headers — loopback only.

		// Resolve the embedded asset path.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		data, err := bundled.FS.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, path, time.Time{}, bytes.NewReader(data))
	})
}