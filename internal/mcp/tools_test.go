package mcp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

// testOptions builds an isolated runtime with one workspace, one evidence
// record, and one approved claim so every tool exercises real content.
func testOptions(t *testing.T) Options {
	t.Helper()
	tmp := t.TempDir()
	paths, err := zruntime.ResolvePaths(zruntime.Options{
		CWD:        filepath.Join(tmp, "project"),
		HomeDir:    tmp,
		RuntimeDir: filepath.Join(tmp, ".zbrain"),
	})
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
	source := filepath.Join(tmp, "source.txt")
	if err := os.WriteFile(source, []byte("source bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	evidence, err := (zruntime.EvidenceStore{Paths: paths, Now: func() time.Time { return now }}).AddFile("research", source, "file://source.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}
	claimID, err := zruntime.NewClaimID()
	if err != nil {
		t.Fatalf("NewClaimID() error = %v", err)
	}
	claim := zruntime.Claim{
		Type:        zruntime.OKFClaimType,
		ID:          claimID,
		Tier:        "projects",
		Status:      zruntime.ClaimStatusDraft,
		Title:       "Evidence Claim",
		Basis:       zruntime.ClaimBasis("evidence"),
		CreatedAt:   now.UTC().Format(time.RFC3339),
		CreatedBy:   "test",
		EvidenceIDs: []string{evidence.ID},
		Body:        "Claim body",
	}
	if err := (zruntime.IndexStore{Paths: paths}).MarkDirty("research"); err != nil {
		t.Fatalf("MarkDirty() error = %v", err)
	}
	draft, err := (zruntime.ClaimStore{Paths: paths, Now: func() time.Time { return now }}).WriteDraft("research", claim)
	if err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := (zruntime.ClaimStore{Paths: paths, Now: func() time.Time { return now }}).Approve("research", draft.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if _, err := (zruntime.IndexStore{Paths: paths}).Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	return Options{Paths: paths, Now: func() time.Time { return now }, Version: "test", Stderr: &bytes.Buffer{}}
}

func connectClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	t1, t2 := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "zbrain-test-client", Version: "0.0.0"}, nil)
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()
	return cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
}

func resultText(res *mcp.CallToolResult) string {
	var parts []string
	for _, content := range res.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func TestToolSurface(t *testing.T) {
	opts := testOptions(t)
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)

	toolNames := map[string]bool{}
	for tool, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Tools() error = %v", err)
		}
		toolNames[tool.Name] = true
	}
	want := []string{"workspace_current", "memory_ask", "memory_status", "memory_reindex", "evidence_capture", "claim_draft", "claim_lifecycle"}
	for _, name := range want {
		if !toolNames[name] {
			t.Errorf("tools/list missing %s; got %v", name, toolNames)
		}
	}
	if len(toolNames) != len(want) {
		t.Errorf("tools/list count = %d, want %d", len(toolNames), len(want))
	}

	// workspace_current resolves the default workspace.
	res, err := callTool(t, cs, "workspace_current", nil)
	if err != nil {
		t.Fatalf("workspace_current error = %v", err)
	}
	if res.IsError {
		t.Fatalf("workspace_current isError; %s", resultText(res))
	}
	if !strings.Contains(resultText(res), `"workspace": "research"`) {
		t.Errorf("workspace_current missing workspace; %s", resultText(res))
	}

	// memory_ask returns trusted context containing the approved claim.
	res, err = callTool(t, cs, "memory_ask", map[string]any{"query": "Evidence Claim"})
	if err != nil {
		t.Fatalf("memory_ask error = %v", err)
	}
	if res.IsError {
		t.Fatalf("memory_ask isError; %s", resultText(res))
	}
	if !strings.Contains(resultText(res), "Evidence Claim") {
		t.Errorf("memory_ask result missing approved claim; %s", resultText(res))
	}

	// memory_status reports workspace health.
	res, err = callTool(t, cs, "memory_status", nil)
	if err != nil {
		t.Fatalf("memory_status error = %v", err)
	}
	if res.IsError {
		t.Fatalf("memory_status isError; %s", resultText(res))
	}
	if !strings.Contains(resultText(res), `"workspace": "research"`) {
		t.Errorf("memory_status missing workspace; %s", resultText(res))
	}

	// memory_reindex rebuilds the derived index.
	res, err = callTool(t, cs, "memory_reindex", nil)
	if err != nil {
		t.Fatalf("memory_reindex error = %v", err)
	}
	if res.IsError {
		t.Fatalf("memory_reindex isError; %s", resultText(res))
	}

	// evidence_capture snapshots a new source file.
	captureSource := filepath.Join(t.TempDir(), "capture.txt")
	if err := os.WriteFile(captureSource, []byte("capture bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile(capture) error = %v", err)
	}
	res, err = callTool(t, cs, "evidence_capture", map[string]any{"file": captureSource, "origin": "file://capture.txt"})
	if err != nil {
		t.Fatalf("evidence_capture error = %v", err)
	}
	if res.IsError {
		t.Fatalf("evidence_capture isError; %s", resultText(res))
	}
	if !strings.Contains(resultText(res), `"id": "evd_`) {
		t.Errorf("evidence_capture missing evidence id; %s", resultText(res))
	}

	// claim_draft creates a promotion-candidate draft.
	res, err = callTool(t, cs, "claim_draft", map[string]any{"tier": "projects", "title": "Draft via MCP", "basis": "evidence", "body": "draft body"})
	if err != nil {
		t.Fatalf("claim_draft error = %v", err)
	}
	if res.IsError {
		t.Fatalf("claim_draft isError; %s", resultText(res))
	}
	if !strings.Contains(resultText(res), `"status": "draft"`) {
		t.Errorf("claim_draft missing draft status; %s", resultText(res))
	}

	// claim_lifecycle fails closed (owner-pinned phase has not landed).
	res, err = callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "prepare", "claim_id": "clm_00000000000000000000000000000000"})
	if err != nil {
		t.Fatalf("claim_lifecycle prepare error = %v", err)
	}
	if !res.IsError {
		t.Errorf("claim_lifecycle prepare must fail closed with isError; %s", resultText(res))
	}

	// Invalid parameters map to -32602.
	_, err = callTool(t, cs, "memory_ask", map[string]any{})
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("missing-query memory_ask error type = %T, want *jsonrpc.Error (%v)", err, err)
	}
	if rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("missing-query memory_ask code = %d, want %d", rpcErr.Code, jsonrpc.CodeInvalidParams)
	}

	// Domain failures map to isError.
	res, err = callTool(t, cs, "memory_ask", map[string]any{"query": "x", "workspace": "nonexistent"})
	if err != nil {
		t.Fatalf("memory_ask(nonexistent workspace) error = %v", err)
	}
	if !res.IsError {
		t.Errorf("memory_ask with nonexistent workspace must be isError; %s", resultText(res))
	}
}
