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

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

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

// TestProtocolRevisionMatrix pins dual-era wire behavior on raw stdio
// frames: legacy initialize negotiation (2025-06-18 and the 2025-11-25 cap),
// the stateless 2026-07-28 era (server/discover, per-request _meta tool
// calls), and its failure modes (-32602 missing capabilities, -32022
// unsupported version). Each subtest runs a fresh subprocess session.
func TestProtocolRevisionMatrix(t *testing.T) {
	if os.Getenv("ZBRAIN_MCP_HELPER") == "1" {
		runPurityHelper(t)
		return
	}
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	home := filepath.Join(tmp, ".zbrain")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("MkdirAll(project) error = %v", err)
	}
	paths, err := zruntime.ResolvePaths(zruntime.Options{CWD: project, RuntimeDir: home})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	if _, err := zruntime.EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	if _, err := zruntime.ExtractBundledAssets(paths); err != nil {
		t.Fatalf("ExtractBundledAssets() error = %v", err)
	}
	if err := zruntime.CreateWorkspace(paths, "research", now); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	const (
		legacyV1 = "2025-06-18"
		legacyV2 = "2025-11-25"
		modernV  = "2026-07-28"
	)

	newHelper := func(t *testing.T) (io.WriteCloser, io.Reader, func() string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestProtocolRevisionMatrix$")
		cmd.Dir = project
		cmd.Env = append(os.Environ(), "ZBRAIN_MCP_HELPER=1", "ZBRAIN_HOME="+home)
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
		t.Cleanup(func() {
			_ = stdin.Close()
			_ = cmd.Wait()
		})
		return stdin, stdout, stderr.String
	}

	exchange := func(t *testing.T, stdin io.WriteCloser, stdout io.Reader, request map[string]any) map[string]any {
		t.Helper()
		if err := json.NewEncoder(stdin).Encode(request); err != nil {
			t.Fatalf("encode %v: %v", request["method"], err)
		}
		wantID, _ := request["id"].(int)
		lines := make(chan string, 64)
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				lines <- scanner.Text()
			}
			close(lines)
		}()
		timeout := time.After(15 * time.Second)
		for {
			select {
			case line, ok := <-lines:
				if !ok {
					t.Fatalf("stdout closed before response id %d", wantID)
				}
				var frame map[string]any
				if err := json.Unmarshal([]byte(line), &frame); err != nil {
					continue // purity is asserted by TestStdoutPurity
				}
				id, _ := frame["id"].(float64)
				if int(id) == wantID && (frame["result"] != nil || frame["error"] != nil) {
					return frame
				}
			case <-timeout:
				t.Fatalf("timeout waiting for response id %d (%v)", wantID, request["method"])
			}
		}
	}

	initializeRequest := func(id int, version string) map[string]any {
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": version,
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": "zbrain-test", "version": "0.0.0"},
			},
		}
	}

	eraMeta := func(version string, withCapabilities bool) map[string]any {
		meta := map[string]any{"io.modelcontextprotocol/protocolVersion": version}
		if withCapabilities {
			meta["io.modelcontextprotocol/clientCapabilities"] = map[string]any{}
		}
		return meta
	}

	t.Run("LegacyNegotiates20250618", func(t *testing.T) {
		stdin, stdout, stderrFor := newHelper(t)
		frame := exchange(t, stdin, stdout, initializeRequest(1, legacyV1))
		res, ok := frame["result"].(map[string]any)
		if !ok {
			t.Fatalf("initialize result type = %T (%v)", frame["result"], stderrFor())
		}
		if res["protocolVersion"] != legacyV1 {
			t.Errorf("negotiated protocolVersion = %v, want %q", res["protocolVersion"], legacyV1)
		}
	})

	t.Run("LegacyCapsAt20251125", func(t *testing.T) {
		stdin, stdout, stderrFor := newHelper(t)
		frame := exchange(t, stdin, stdout, initializeRequest(1, legacyV2))
		res, ok := frame["result"].(map[string]any)
		if !ok {
			t.Fatalf("initialize result type = %T (%v)", frame["result"], stderrFor())
		}
		if res["protocolVersion"] != legacyV2 {
			t.Errorf("negotiated protocolVersion = %v, want %q", res["protocolVersion"], legacyV2)
		}
	})

	t.Run("DiscoverAdvertisesModernVersion", func(t *testing.T) {
		stdin, stdout, stderrFor := newHelper(t)
		frame := exchange(t, stdin, stdout, map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "server/discover",
			"params":  map[string]any{"_meta": eraMeta(modernV, true)},
		})
		res, ok := frame["result"].(map[string]any)
		if !ok {
			t.Fatalf("discover result type = %T, error = %v (%v)", frame["result"], frame["error"], stderrFor())
		}
		versions, ok := res["supportedVersions"].([]any)
		if !ok {
			t.Fatalf("supportedVersions type = %T (%v)", res["supportedVersions"], stderrFor())
		}
		sawModern := false
		for _, v := range versions {
			if v == modernV {
				sawModern = true
			}
		}
		if !sawModern {
			t.Errorf("supportedVersions = %v, want it to include %q", versions, modernV)
		}
		caps, ok := res["capabilities"].(map[string]any)
		if !ok || caps == nil {
			t.Errorf("capabilities missing from discover result: %v", res)
		}
	})

	t.Run("StatelessToolCallWithoutHandshake", func(t *testing.T) {
		stdin, stdout, stderrFor := newHelper(t)
		frame := exchange(t, stdin, stdout, map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]any{
				"name":      "workspace_current",
				"arguments": map[string]any{},
				"_meta":     eraMeta(modernV, true),
			},
		})
		res, ok := frame["result"].(map[string]any)
		if !ok {
			t.Fatalf("tools/call result type = %T, error = %v (%v)", frame["result"], frame["error"], stderrFor())
		}
		if isError, _ := res["isError"].(bool); isError {
			t.Errorf("workspace_current isError on stateless path; %v", res)
		}
	})

	t.Run("StatelessRequiresClientCapabilities", func(t *testing.T) {
		stdin, stdout, stderrFor := newHelper(t)
		frame := exchange(t, stdin, stdout, map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/list",
			"params":  map[string]any{"_meta": eraMeta(modernV, false)},
		})
		errObj, ok := frame["error"].(map[string]any)
		if !ok {
			t.Fatalf("missing-capabilities request returned %v, want error (%v)", frame, stderrFor())
		}
		code, _ := errObj["code"].(float64)
		if int(code) != jsonrpc.CodeInvalidParams {
			t.Errorf("missing-capabilities code = %d, want %d", int(code), jsonrpc.CodeInvalidParams)
		}
	})

	t.Run("StatelessRejectsFutureVersion", func(t *testing.T) {
		stdin, stdout, stderrFor := newHelper(t)
		frame := exchange(t, stdin, stdout, map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/list",
			"params":  map[string]any{"_meta": eraMeta("2999-01-01", true)},
		})
		errObj, ok := frame["error"].(map[string]any)
		if !ok {
			t.Fatalf("future-version request returned %v, want error (%v)", frame, stderrFor())
		}
		code, _ := errObj["code"].(float64)
		if int(code) != mcp.CodeUnsupportedProtocolVersion {
			t.Errorf("future-version code = %d, want %d", int(code), mcp.CodeUnsupportedProtocolVersion)
		}
	})
}
