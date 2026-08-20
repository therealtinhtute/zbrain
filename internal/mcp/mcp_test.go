package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

// TestStdoutPurity asserts the stdio server writes only JSON-RPC frames to
// stdout and routes diagnostics to stderr. The test re-executes the test binary
// as a subprocess in helper mode so protocol bytes are captured on real pipes.
func TestStdoutPurity(t *testing.T) {
	if os.Getenv("ZBRAIN_MCP_HELPER") == "1" {
		runPurityHelper(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStdoutPurity$")
	cmd.Env = append(os.Environ(),
		"ZBRAIN_MCP_HELPER=1",
		"ZBRAIN_HOME="+filepath.Join(t.TempDir(), ".zbrain"),
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe() error = %v", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe() error = %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "zbrain-test", "version": "0.0.0"},
		},
	}
	if err := json.NewEncoder(stdin).Encode(request); err != nil {
		t.Fatalf("encode initialize: %v", err)
	}
	lines := scanLines(t, stdout, 5*time.Second)
	if err := waitForInitialize(lines); err != nil {
		t.Fatalf("initialize response not received: %v\nstdout so far:\n%s", err, strings.Join(lines, "\n"))
	}
	if err := stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	err = cmd.Wait()
	if err != nil {
		t.Fatalf("helper exited: %v\nstderr:\n%s", err, stderr.String())
	}
	for i, line := range lines {
		var frame map[string]any
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("stdout line %d is not JSON-RPC: %q\nstderr:\n%s", i, line, stderr.String())
		}
		if frame["jsonrpc"] != "2.0" {
			t.Fatalf("stdout line %d missing jsonrpc 2.0: %q", i, line)
		}
	}
	if !bytes.Contains(stderr.Bytes(), []byte("server run start")) {
		t.Fatalf("stderr missing diagnostic 'server run start'; got:\n%s", stderr.String())
	}
}

// runPurityHelper runs the server in helper mode and treats stdin EOF as a
// normal shutdown (the SDK surfaces it as "server is closing: EOF").
func runPurityHelper(t *testing.T) {
	paths, err := zruntime.ResolvePaths(zruntime.Options{})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	err = Serve(context.Background(), Options{Paths: paths, Version: "test", Stderr: os.Stderr})
	if err != nil && !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("Serve() error = %v", err)
	}
	os.Exit(0)
}

// scanLines reads frames from the helper's stdout until the initialize response
// arrives or the deadline passes, returning the frames in order.
func scanLines(t *testing.T, stdout io.Reader, timeout time.Duration) []string {
	t.Helper()
	lines := []string{}
	scanner := bufio.NewScanner(stdout)
	type result struct {
		line string
		err  error
		done bool
	}
	ch := make(chan result, 1)
	go func() {
		for scanner.Scan() {
			ch <- result{line: scanner.Text()}
		}
		ch <- result{err: scanner.Err(), done: true}
	}()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case r := <-ch:
			if r.done {
				return lines
			}
			lines = append(lines, r.line)
			if isInitializeResponse(r.line) {
				return lines
			}
		case <-deadline.C:
			return lines
		}
	}
}

func isInitializeResponse(line string) bool {
	var frame map[string]any
	if err := json.Unmarshal([]byte(line), &frame); err != nil {
		return false
	}
	if _, hasResult := frame["result"]; !hasResult {
		return false
	}
	if id, ok := frame["id"].(float64); ok && id == 1 {
		return true
	}
	return false
}

func waitForInitialize(lines []string) error {
	for _, line := range lines {
		if isInitializeResponse(line) {
			return nil
		}
	}
	return errors.New("no initialize response observed")
}
