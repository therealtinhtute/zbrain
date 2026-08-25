package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	// claim_lifecycle rejects an unknown claim as a domain failure.
	res, err = callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "prepare", "action": "approve", "claim_id": "clm_00000000000000000000000000000000"})
	if err != nil {
		t.Fatalf("claim_lifecycle prepare error = %v", err)
	}
	if !res.IsError {
		t.Errorf("claim_lifecycle prepare must fail closed with isError; %s", resultText(res))
	}

	// Structurally invalid input fails closed as a tool-level isError
	// (generated-schema validation), not a wire error.
	res, err = callTool(t, cs, "memory_ask", map[string]any{})
	if err != nil {
		t.Fatalf("missing-query memory_ask error = %v", err)
	}
	if !res.IsError {
		t.Errorf("missing-query memory_ask must fail closed with isError; %s", resultText(res))
	}

	// An empty query passes schema validation and is rejected by the guard.
	res, err = callTool(t, cs, "memory_ask", map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("empty-query memory_ask error = %v", err)
	}
	if !res.IsError {
		t.Errorf("empty-query memory_ask must be isError; %s", resultText(res))
	}

	// Unknown tools map to -32602.
	_, err = callTool(t, cs, "no_such_tool", map[string]any{})
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("unknown-tool error type = %T, want *jsonrpc.Error (%v)", err, err)
	}
	if rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("unknown-tool code = %d, want %d", rpcErr.Code, jsonrpc.CodeInvalidParams)
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

func lifecycleDraft(t *testing.T, opts Options, title string) zruntime.Claim {
	t.Helper()
	id, err := zruntime.NewClaimID()
	if err != nil {
		t.Fatalf("NewClaimID() error = %v", err)
	}
	claim := zruntime.Claim{
		Type:      zruntime.OKFClaimType,
		ID:        id,
		Tier:      "projects",
		Status:    zruntime.ClaimStatusDraft,
		Title:     title,
		Basis:     zruntime.ClaimBasisOwner,
		CreatedAt: opts.Now().UTC().Format(time.RFC3339),
		CreatedBy: "test",
		Body:      title + " body",
	}
	if err := (zruntime.IndexStore{Paths: opts.Paths}).MarkDirty("research"); err != nil {
		t.Fatalf("MarkDirty() error = %v", err)
	}
	created, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).WriteDraft("research", claim)
	if err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	return created
}

func lifecycleApprovedClaim(t *testing.T, opts Options) zruntime.Claim {
	t.Helper()
	scan, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).ScanWorkspaceForTrust("research")
	if err != nil {
		t.Fatalf("ScanWorkspaceForTrust() error = %v", err)
	}
	for _, claim := range scan.Claims {
		if claim.Status == zruntime.ClaimStatusApproved {
			return claim
		}
	}
	t.Fatal("test fixture has no approved claim")
	return zruntime.Claim{}
}

func lifecycleSupersedingDraft(t *testing.T, opts Options, approved zruntime.Claim) zruntime.Claim {
	t.Helper()
	id, err := zruntime.NewClaimID()
	if err != nil {
		t.Fatalf("NewClaimID() error = %v", err)
	}
	replacement := zruntime.Claim{
		Type:      zruntime.OKFClaimType,
		ID:        id,
		Tier:      approved.Tier,
		Status:    zruntime.ClaimStatusDraft,
		Title:     approved.Title + " replacement",
		Basis:     zruntime.ClaimBasisOwner,
		CreatedAt: opts.Now().UTC().Format(time.RFC3339),
		CreatedBy: "test",
		Body:      approved.Body + " replacement",
	}
	if err := (zruntime.IndexStore{Paths: opts.Paths}).MarkDirty("research"); err != nil {
		t.Fatalf("MarkDirty() error = %v", err)
	}
	created, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).WriteSupersedingDraft("research", approved.ID, replacement)
	if err != nil {
		t.Fatalf("WriteSupersedingDraft() error = %v", err)
	}
	return created
}

func lifecycleResultMap(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(resultText(res)), &out); err != nil {
		t.Fatalf("decode lifecycle result: %v; text=%s", err, resultText(res))
	}
	return out
}

func lifecycleString(t *testing.T, out map[string]any, key string) string {
	t.Helper()
	value, ok := out[key].(string)
	if !ok || value == "" {
		t.Fatalf("lifecycle result %s = %#v, want non-empty string", key, out[key])
	}
	return value
}

func grantLifecycleToken(t *testing.T, opts Options, challengeID string) string {
	t.Helper()
	store := zruntime.ChallengeStore{Paths: opts.Paths, Now: opts.Now}
	challenge, workspace, err := store.FindChallenge(challengeID)
	if err != nil {
		t.Fatalf("FindChallenge() error = %v", err)
	}
	granted, err := store.Grant(workspace, challenge.ID)
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	return granted.Token
}

func assertLifecycleNoToken(t *testing.T, out map[string]any) {
	t.Helper()
	if token, ok := out["token"]; ok && token != "" {
		t.Fatalf("prepare exposed plaintext token: %#v", token)
	}
}

func TestClaimLifecycleApproveAndProvenance(t *testing.T) {
	opts := testOptions(t)
	draft := lifecycleDraft(t, opts, "MCP approval")
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)

	prepared, err := callTool(t, cs, "claim_lifecycle", map[string]any{
		"operation": "prepare",
		"action":    "approve",
		"workspace": "research",
		"claim_id":  draft.ID,
	})
	if err != nil || prepared.IsError {
		t.Fatalf("claim_lifecycle prepare error=%v isError=%v text=%s", err, prepared != nil && prepared.IsError, resultText(prepared))
	}
	preparedOut := lifecycleResultMap(t, prepared)
	challengeID := lifecycleString(t, preparedOut, "challenge_id")
	assertLifecycleNoToken(t, preparedOut)
	if lifecycleString(t, preparedOut, "action_digest") == "" {
		t.Fatal("prepare action_digest is empty")
	}
	if lifecycleString(t, preparedOut, "expires_at") == "" {
		t.Fatal("prepare challenge expiry is empty")
	}
	if _, ok := preparedOut["token_expires_at"]; ok {
		t.Fatalf("prepare returned token expiry before grant: %#v", preparedOut["token_expires_at"])
	}
	if !strings.Contains(resultText(prepared), `"action_summary"`) {
		t.Fatalf("prepare response missing action summary: %s", resultText(prepared))
	}
	challengePath := filepath.Join(opts.Paths.WorkspacesDir, "research", ".zbrain", "challenges", challengeID+".json")
	persisted, err := os.ReadFile(challengePath)
	if err != nil {
		t.Fatalf("ReadFile(challenge) error = %v", err)
	}
	var beforeGrant zruntime.Challenge
	if err := json.Unmarshal(persisted, &beforeGrant); err != nil {
		t.Fatalf("decode challenge before grant: %v", err)
	}
	if beforeGrant.TokenSHA256 != "" || beforeGrant.TokenExpiresAt != "" {
		t.Fatalf("challenge persisted token material before grant: %#v", beforeGrant)
	}

	blocked, err := callTool(t, cs, "claim_lifecycle", map[string]any{
		"operation":    "apply",
		"action":       "approve",
		"workspace":    "research",
		"claim_id":     draft.ID,
		"challenge_id": challengeID,
		"token":        "not-issued",
	})
	if err != nil || blocked == nil || !blocked.IsError || !strings.Contains(resultText(blocked), "has not been owner-granted") {
		t.Fatalf("ungranted apply error=%v result=%#v text=%s", err, blocked, resultText(blocked))
	}
	unchanged, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).Read("research", draft.ID)
	if err != nil {
		t.Fatalf("Read(after ungranted apply) error = %v", err)
	}
	if unchanged.Status != zruntime.ClaimStatusDraft || len(unchanged.Transitions) != 0 {
		t.Fatalf("claim changed after ungranted apply: %#v", unchanged)
	}
	token := grantLifecycleToken(t, opts, challengeID)

	applied, err := callTool(t, cs, "claim_lifecycle", map[string]any{
		"operation":    "apply",
		"action":       "approve",
		"workspace":    "research",
		"claim_id":     draft.ID,
		"challenge_id": challengeID,
		"token":        token,
	})
	if err != nil || applied.IsError {
		t.Fatalf("claim_lifecycle apply error=%v isError=%v text=%s", err, applied != nil && applied.IsError, resultText(applied))
	}
	appliedOut := lifecycleResultMap(t, applied)
	if appliedOut["status"] != string(zruntime.ClaimStatusApproved) || appliedOut["verified_by"] != "owner:mcp" {
		t.Fatalf("apply provenance = %#v, want approved/owner:mcp", appliedOut)
	}
	claim, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).Read("research", draft.ID)
	if err != nil {
		t.Fatalf("Read(applied claim) error = %v", err)
	}
	if claim.Status != zruntime.ClaimStatusApproved || claim.VerifiedBy != "owner:mcp" {
		t.Fatalf("claim provenance = status %s verified_by %q", claim.Status, claim.VerifiedBy)
	}
	if len(claim.Transitions) != 1 || claim.Transitions[0].Authorization == nil {
		t.Fatalf("claim transitions = %#v, want one authorized transition", claim.Transitions)
	}
	authorization := claim.Transitions[0].Authorization
	if authorization.ChallengeID != challengeID || authorization.Method != "mcp.claim_lifecycle" || authorization.MCPClient != "zbrain-test-client/0.0.0" {
		t.Fatalf("authorization = %#v, want challenge/method/client provenance", authorization)
	}

	replay, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "apply", "challenge_id": challengeID, "token": token})
	if err != nil {
		t.Fatalf("replay call error = %v", err)
	}
	if !replay.IsError || !strings.Contains(resultText(replay), "already consumed") {
		t.Fatalf("replay result = isError %v text %s, want consumed failure", replay.IsError, resultText(replay))
	}
}

func TestClaimLifecycleSupersedeAndRevoke(t *testing.T) {
	t.Run("supersede", func(t *testing.T) {
		opts := testOptions(t)
		approved := lifecycleApprovedClaim(t, opts)
		draft := lifecycleSupersedingDraft(t, opts, approved)
		server, err := newServer(opts)
		if err != nil {
			t.Fatalf("newServer() error = %v", err)
		}
		cs := connectClient(t, server)
		prepared, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "prepare", "action": "supersede", "claim_id": draft.ID})
		if err != nil || prepared.IsError {
			t.Fatalf("prepare error=%v isError=%v text=%s", err, prepared != nil && prepared.IsError, resultText(prepared))
		}
		out := lifecycleResultMap(t, prepared)
		challengeID := lifecycleString(t, out, "challenge_id")
		token := grantLifecycleToken(t, opts, challengeID)
		emptySuperseded, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "apply", "challenge_id": challengeID, "token": token, "superseded_ids": []string{}})
		if err != nil || emptySuperseded == nil || !emptySuperseded.IsError || !strings.Contains(resultText(emptySuperseded), "superseded IDs") {
			t.Fatalf("explicit empty superseded IDs error=%v result=%#v text=%s", err, emptySuperseded, resultText(emptySuperseded))
		}
		applied, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "apply", "challenge_id": challengeID, "token": token})
		if err != nil || applied.IsError {
			t.Fatalf("apply error=%v isError=%v text=%s", err, applied != nil && applied.IsError, resultText(applied))
		}
		gotReplacement, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).Read("research", draft.ID)
		if err != nil {
			t.Fatalf("Read(replacement) error = %v", err)
		}
		gotOld, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).Read("research", approved.ID)
		if err != nil {
			t.Fatalf("Read(old) error = %v", err)
		}
		if gotReplacement.Status != zruntime.ClaimStatusApproved || gotOld.Status != zruntime.ClaimStatusSuperseded {
			t.Fatalf("supersede statuses = replacement %s old %s", gotReplacement.Status, gotOld.Status)
		}
	})

	t.Run("revoke", func(t *testing.T) {
		opts := testOptions(t)
		approved := lifecycleApprovedClaim(t, opts)
		server, err := newServer(opts)
		if err != nil {
			t.Fatalf("newServer() error = %v", err)
		}
		cs := connectClient(t, server)
		prepared, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "prepare", "action": "revoke", "claim_id": approved.ID, "revoke_reason": "no longer current"})
		if err != nil || prepared.IsError {
			t.Fatalf("prepare error=%v isError=%v text=%s", err, prepared != nil && prepared.IsError, resultText(prepared))
		}
		out := lifecycleResultMap(t, prepared)
		challengeID := lifecycleString(t, out, "challenge_id")
		token := grantLifecycleToken(t, opts, challengeID)
		applied, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "apply", "challenge_id": challengeID, "token": token})
		if err != nil || applied.IsError {
			t.Fatalf("apply error=%v isError=%v text=%s", err, applied != nil && applied.IsError, resultText(applied))
		}
		got, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).Read("research", approved.ID)
		if err != nil {
			t.Fatalf("Read(revoked) error = %v", err)
		}
		if got.Status != zruntime.ClaimStatusRevoked {
			t.Fatalf("revoke claim status = %s, want revoked", got.Status)
		}
		if len(got.Transitions) == 0 || got.Transitions[len(got.Transitions)-1].By != "owner:mcp" {
			t.Fatalf("revoke transition provenance = %#v, want owner:mcp", got.Transitions)
		}
	})
}

func TestClaimLifecycleFailureBoundaries(t *testing.T) {
	t.Run("wrong token", func(t *testing.T) {
		opts := testOptions(t)
		draft := lifecycleDraft(t, opts, "wrong token")
		server, err := newServer(opts)
		if err != nil {
			t.Fatalf("newServer() error = %v", err)
		}
		cs := connectClient(t, server)
		prepared, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "prepare", "action": "approve", "claim_id": draft.ID})
		if err != nil || prepared.IsError {
			t.Fatalf("prepare error=%v isError=%v text=%s", err, prepared != nil && prepared.IsError, resultText(prepared))
		}
		out := lifecycleResultMap(t, prepared)
		challengeID := lifecycleString(t, out, "challenge_id")
		assertLifecycleNoToken(t, out)
		token := grantLifecycleToken(t, opts, challengeID)
		wrong, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "apply", "challenge_id": challengeID, "token": "wrong"})
		if err != nil || wrong == nil || !wrong.IsError || !strings.Contains(resultText(wrong), "token mismatch") {
			t.Fatalf("wrong token error=%v result=%#v text=%s", err, wrong, resultText(wrong))
		}
		correct, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "apply", "challenge_id": challengeID, "token": token})
		if err != nil || correct.IsError {
			t.Fatalf("correct token after wrong token error=%v isError=%v text=%s", err, correct != nil && correct.IsError, resultText(correct))
		}
	})

	t.Run("stale canonical", func(t *testing.T) {
		opts := testOptions(t)
		draft := lifecycleDraft(t, opts, "stale canonical")
		server, err := newServer(opts)
		if err != nil {
			t.Fatalf("newServer() error = %v", err)
		}
		cs := connectClient(t, server)
		prepared, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "prepare", "action": "approve", "claim_id": draft.ID})
		if err != nil || prepared.IsError {
			t.Fatalf("prepare error=%v isError=%v text=%s", err, prepared != nil && prepared.IsError, resultText(prepared))
		}
		out := lifecycleResultMap(t, prepared)
		challengeID := lifecycleString(t, out, "challenge_id")
		assertLifecycleNoToken(t, out)
		token := grantLifecycleToken(t, opts, challengeID)
		draft.Body = "changed after prepare"
		if err := (zruntime.IndexStore{Paths: opts.Paths}).MarkDirty("research"); err != nil {
			t.Fatalf("MarkDirty() error = %v", err)
		}
		if _, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).WriteDraft("research", draft); err != nil {
			t.Fatalf("WriteDraft(stale mutation) error = %v", err)
		}
		stale, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "apply", "challenge_id": challengeID, "token": token})
		if err != nil || stale == nil || !stale.IsError || !strings.Contains(resultText(stale), "stale") {
			t.Fatalf("stale apply error=%v result=%#v text=%s", err, stale, resultText(stale))
		}
		got, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).Read("research", draft.ID)
		if err != nil {
			t.Fatalf("Read(stale draft) error = %v", err)
		}
		if got.Status != zruntime.ClaimStatusDraft {
			t.Fatalf("stale apply mutated status to %s", got.Status)
		}
	})

	t.Run("wrong workspace", func(t *testing.T) {
		opts := testOptions(t)
		if err := zruntime.CreateWorkspace(opts.Paths, "other", opts.Now()); err != nil {
			t.Fatalf("CreateWorkspace(other) error = %v", err)
		}
		draft := lifecycleDraft(t, opts, "wrong workspace")
		server, err := newServer(opts)
		if err != nil {
			t.Fatalf("newServer() error = %v", err)
		}
		cs := connectClient(t, server)
		prepared, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "prepare", "action": "approve", "workspace": "research", "claim_id": draft.ID})
		if err != nil || prepared.IsError {
			t.Fatalf("prepare error=%v isError=%v text=%s", err, prepared != nil && prepared.IsError, resultText(prepared))
		}
		out := lifecycleResultMap(t, prepared)
		challengeID := lifecycleString(t, out, "challenge_id")
		assertLifecycleNoToken(t, out)
		wrong, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "apply", "workspace": "other", "challenge_id": challengeID, "token": "not-issued"})
		if err != nil || wrong == nil || !wrong.IsError || !strings.Contains(resultText(wrong), "does not own") {
			t.Fatalf("wrong workspace error=%v result=%#v text=%s", err, wrong, resultText(wrong))
		}
	})

	t.Run("expiry", func(t *testing.T) {
		opts := testOptions(t)
		now := opts.Now()
		opts.Now = func() time.Time { return now }
		draft := lifecycleDraft(t, opts, "expired token")
		server, err := newServer(opts)
		if err != nil {
			t.Fatalf("newServer() error = %v", err)
		}
		cs := connectClient(t, server)
		prepared, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "prepare", "action": "approve", "claim_id": draft.ID})
		if err != nil || prepared.IsError {
			t.Fatalf("prepare error=%v isError=%v text=%s", err, prepared != nil && prepared.IsError, resultText(prepared))
		}
		out := lifecycleResultMap(t, prepared)
		challengeID := lifecycleString(t, out, "challenge_id")
		assertLifecycleNoToken(t, out)
		token := grantLifecycleToken(t, opts, challengeID)
		now = now.Add(6 * time.Minute)
		expired, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "apply", "challenge_id": challengeID, "token": token})
		if err != nil || expired == nil || !expired.IsError || !strings.Contains(resultText(expired), "token expired") {
			t.Fatalf("expired apply error=%v result=%#v text=%s", err, expired, resultText(expired))
		}
	})
}

func TestClaimLifecycleConcurrentApply(t *testing.T) {
	opts := testOptions(t)
	draft := lifecycleDraft(t, opts, "concurrent apply")
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)
	prepared, err := callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "prepare", "action": "approve", "claim_id": draft.ID})
	if err != nil || prepared.IsError {
		t.Fatalf("prepare error=%v isError=%v text=%s", err, prepared != nil && prepared.IsError, resultText(prepared))
	}
	out := lifecycleResultMap(t, prepared)
	challengeID := lifecycleString(t, out, "challenge_id")
	assertLifecycleNoToken(t, out)
	token := grantLifecycleToken(t, opts, challengeID)
	args := map[string]any{"operation": "apply", "challenge_id": challengeID, "token": token}
	const attempts = 8
	results := make(chan *mcp.CallToolResult, attempts)
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			res, err := callTool(t, cs, "claim_lifecycle", args)
			results <- res
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	winners := 0
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent apply RPC error = %v", err)
		}
	}
	for res := range results {
		if res != nil && !res.IsError {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent apply winners = %d, want exactly one", winners)
	}
}

func TestMCPClientProvenanceNilSafe(t *testing.T) {
	if got := mcpClientName(nil); got != "unknown" {
		t.Fatalf("mcpClientName(nil) = %q, want unknown", got)
	}
	if got := mcpClientName(&mcp.CallToolRequest{}); got != "unknown" {
		t.Fatalf("mcpClientName(nil session) = %q, want unknown", got)
	}

	opts := testOptions(t)
	draft := lifecycleDraft(t, opts, "nil client provenance")
	prepared, _, err := prepareLifecycle(opts, lifecycleIn{Operation: "prepare", Action: "approve", ClaimID: draft.ID})
	if err != nil || prepared.IsError {
		t.Fatalf("prepareLifecycle() error=%v isError=%v text=%s", err, prepared != nil && prepared.IsError, resultText(prepared))
	}
	out := lifecycleResultMap(t, prepared)
	challengeID := lifecycleString(t, out, "challenge_id")
	assertLifecycleNoToken(t, out)
	token := grantLifecycleToken(t, opts, challengeID)
	applied, _, err := applyLifecycle(opts, nil, lifecycleIn{
		Operation:   "apply",
		ChallengeID: challengeID,
		Token:       token,
	})
	if err != nil || applied.IsError {
		t.Fatalf("applyLifecycle(nil request) error=%v isError=%v text=%s", err, applied != nil && applied.IsError, resultText(applied))
	}
	claim, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).Read("research", draft.ID)
	if err != nil {
		t.Fatalf("Read(nil provenance claim) error = %v", err)
	}
	if len(claim.Transitions) != 1 || claim.Transitions[0].Authorization == nil || claim.Transitions[0].Authorization.MCPClient != "unknown" {
		t.Fatalf("nil client authorization = %#v, want unknown", claim.Transitions)
	}
}

func schemaProperties(t *testing.T, tool *mcp.Tool) map[string]any {
	t.Helper()
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("%s input schema type = %T, want map[string]any", tool.Name, tool.InputSchema)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s input schema properties = %#v, want object", tool.Name, schema["properties"])
	}
	return properties
}

func TestToolInputSchemas(t *testing.T) {
	opts := testOptions(t)
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)
	tools := map[string]*mcp.Tool{}
	for tool, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Tools() error = %v", err)
		}
		tools[tool.Name] = tool
	}
	for _, name := range []string{"memory_ask", "memory_reindex", "claim_lifecycle"} {
		if tools[name] == nil {
			t.Fatalf("tools/list missing %s", name)
		}
	}
	for _, name := range []string{"memory_ask", "memory_reindex"} {
		properties := schemaProperties(t, tools[name])
		property, ok := properties["embedding"].(map[string]any)
		if !ok {
			t.Fatalf("%s.embedding schema = %#v, want object", name, properties["embedding"])
		}
		if property["type"] != "boolean" {
			t.Errorf("%s.embedding type = %#v, want boolean", name, property["type"])
		}
		if description, _ := property["description"].(string); !strings.Contains(description, "defaults to false") {
			t.Errorf("%s.embedding description = %q, want false default", name, description)
		}
	}
	lifecycleProperties := schemaProperties(t, tools["claim_lifecycle"])
	for _, field := range []string{
		"operation", "action", "workspace", "claim_id", "challenge_id", "token",
		"canonical_draft_digest", "superseded_ids", "prior_verification_digest", "revoke_reason",
	} {
		if _, ok := lifecycleProperties[field]; !ok {
			t.Errorf("claim_lifecycle schema missing %s; properties=%v", field, lifecycleProperties)
		}
	}
}

func TestMemoryEmbeddingOptInAndLexicalFallback(t *testing.T) {
	opts := testOptions(t)
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)
	query := "embedding-only-term"

	withoutEmbedding, err := callTool(t, cs, "memory_ask", map[string]any{"query": query})
	if err != nil || withoutEmbedding.IsError {
		t.Fatalf("lexical memory_ask error=%v isError=%v text=%s", err, withoutEmbedding != nil && withoutEmbedding.IsError, resultText(withoutEmbedding))
	}
	var lexical map[string]any
	if err := json.Unmarshal([]byte(resultText(withoutEmbedding)), &lexical); err != nil {
		t.Fatalf("decode lexical memory_ask: %v", err)
	}
	if lexical["status"] != string(zruntime.QueryStatusGap) {
		t.Fatalf("lexical status = %#v, want gap", lexical["status"])
	}
	if _, err := os.Stat((zruntime.EmbeddingStore{Paths: opts.Paths}).DatabasePath("research")); !os.IsNotExist(err) {
		t.Fatalf("embedding sidecar exists before opt-in: stat err=%v", err)
	}

	reindexed, err := callTool(t, cs, "memory_reindex", map[string]any{"embedding": true})
	if err != nil || reindexed.IsError {
		t.Fatalf("embedding memory_reindex error=%v isError=%v text=%s", err, reindexed != nil && reindexed.IsError, resultText(reindexed))
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(resultText(reindexed)), &summary); err != nil {
		t.Fatalf("decode embedding summary: %v", err)
	}
	embeddingSummary, ok := summary["embedding"].(map[string]any)
	if !ok || embeddingSummary["strategy"] != "loopback" || embeddingSummary["indexed"] != float64(1) {
		t.Fatalf("embedding summary = %#v, want loopback with one indexed claim", summary["embedding"])
	}

	statusResult, err := callTool(t, cs, "memory_status", nil)
	if err != nil || statusResult.IsError {
		t.Fatalf("memory_status after embedding error=%v isError=%v text=%s", err, statusResult != nil && statusResult.IsError, resultText(statusResult))
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(resultText(statusResult)), &status); err != nil {
		t.Fatalf("decode memory_status after embedding: %v", err)
	}
	statusEmbedding, ok := status["embedding"].(map[string]any)
	if !ok || statusEmbedding["strategy"] != "loopback" || statusEmbedding["indexed"] != float64(1) || statusEmbedding["eligible"] != float64(1) {
		t.Fatalf("memory_status embedding = %#v, want loopback coverage", status["embedding"])
	}

	withEmbedding, err := callTool(t, cs, "memory_ask", map[string]any{"query": query, "embedding": true})
	if err != nil || withEmbedding.IsError {
		t.Fatalf("embedding memory_ask error=%v isError=%v text=%s", err, withEmbedding != nil && withEmbedding.IsError, resultText(withEmbedding))
	}
	var hybrid map[string]any
	if err := json.Unmarshal([]byte(resultText(withEmbedding)), &hybrid); err != nil {
		t.Fatalf("decode hybrid memory_ask: %v", err)
	}
	claims, ok := hybrid["claims"].([]any)
	if !ok || len(claims) != 1 {
		t.Fatalf("hybrid claims = %#v, want one vector-retrieved claim", hybrid["claims"])
	}

	defaultAfterEmbedding, err := callTool(t, cs, "memory_ask", map[string]any{"query": query})
	if err != nil || defaultAfterEmbedding.IsError {
		t.Fatalf("default-after-embedding memory_ask error=%v isError=%v text=%s", err, defaultAfterEmbedding != nil && defaultAfterEmbedding.IsError, resultText(defaultAfterEmbedding))
	}
	var lexicalAgain map[string]any
	if err := json.Unmarshal([]byte(resultText(defaultAfterEmbedding)), &lexicalAgain); err != nil {
		t.Fatalf("decode lexical-after-embedding memory_ask: %v", err)
	}
	if lexicalAgain["status"] != string(zruntime.QueryStatusGap) {
		t.Fatalf("default embedding status = %#v, want gap", lexicalAgain["status"])
	}
}

func TestBounds(t *testing.T) {
	opts := testOptions(t)
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)

	// Capture stderr buffer from testOptions.
	buf, ok := opts.Stderr.(*bytes.Buffer)
	if !ok {
		t.Fatalf("opts.Stderr type = %T, want *bytes.Buffer", opts.Stderr)
	}

	// 1. Oversized input returns -32602 (invalid params) via JSON marshaled size check.
	huge := strings.Repeat("a", (1<<20)+1024)
	_, err = callTool(t, cs, "memory_ask", map[string]any{"query": huge})
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("oversized memory_ask error type = %T, want *jsonrpc.Error (%v)", err, err)
	}
	if rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("oversized memory_ask code = %d, want %d", rpcErr.Code, jsonrpc.CodeInvalidParams)
	}
	if !strings.Contains(rpcErr.Message, "1MB") {
		t.Fatalf("oversized memory_ask message = %q, want 1MB limit", rpcErr.Message)
	}

	// Oversized claim_draft body also returns -32602.
	hugeBody := strings.Repeat("b", (1<<20)+10)
	_, err = callTool(t, cs, "claim_draft", map[string]any{"tier": "projects", "title": "huge", "basis": "owner", "body": hugeBody})
	if !errors.As(err, &rpcErr) {
		t.Fatalf("oversized claim_draft error type = %T, want *jsonrpc.Error (%v)", err, err)
	}
	if rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("oversized claim_draft code = %d, want %d", rpcErr.Code, jsonrpc.CodeInvalidParams)
	}

	// Oversized evidence_capture origin also returns -32602.
	_, err = callTool(t, cs, "evidence_capture", map[string]any{"file": strings.Repeat("c", (1<<20)+10), "origin": "file://x"})
	if !errors.As(err, &rpcErr) {
		t.Fatalf("oversized evidence_capture error type = %T, want *jsonrpc.Error (%v)", err, err)
	}
	if rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("oversized evidence_capture code = %d, want %d", rpcErr.Code, jsonrpc.CodeInvalidParams)
	}

	// 2. Valid input passes and timeout not triggered for normal.
	buf.Reset()
	start := time.Now()
	res, err := callTool(t, cs, "memory_ask", map[string]any{"query": "Evidence Claim"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("valid memory_ask error = %v", err)
	}
	if res.IsError {
		t.Fatalf("valid memory_ask isError; %s", resultText(res))
	}
	if !strings.Contains(resultText(res), "Evidence Claim") {
		t.Fatalf("valid memory_ask missing approved claim; %s", resultText(res))
	}
	if elapsed > 5*time.Second {
		t.Fatalf("valid memory_ask exceeded 5s timeout: %s", elapsed)
	}

	// 3. Audit log to stderr contains tool/workspace/duration without body.
	logged := buf.String()
	if !strings.Contains(logged, "tool=memory_ask") {
		t.Fatalf("audit log missing tool=memory_ask; got %q", logged)
	}
	if !strings.Contains(logged, "duration=") {
		t.Fatalf("audit log missing duration; got %q", logged)
	}
	// Must not log body/content query string.
	if strings.Contains(logged, "Evidence Claim") {
		t.Fatalf("audit log leaked body/content; got %q", logged)
	}
	if strings.Contains(logged, huge) || strings.Contains(logged, hugeBody) {
		t.Fatalf("audit log leaked oversized body; got len %d", len(logged))
	}

	// workspace_current valid also passes and logs without body.
	buf.Reset()
	res, err = callTool(t, cs, "workspace_current", nil)
	if err != nil {
		t.Fatalf("workspace_current error = %v", err)
	}
	if res.IsError {
		t.Fatalf("workspace_current isError; %s", resultText(res))
	}
	if !strings.Contains(buf.String(), "tool=workspace_current") {
		t.Fatalf("workspace_current audit log missing; got %q", buf.String())
	}

	// memory_status valid passes.
	buf.Reset()
	res, err = callTool(t, cs, "memory_status", nil)
	if err != nil || res.IsError {
		t.Fatalf("memory_status valid error=%v isError=%v text=%s", err, res != nil && res.IsError, resultText(res))
	}
	if !strings.Contains(buf.String(), "tool=memory_status") {
		t.Fatalf("memory_status audit log missing; got %q", buf.String())
	}

	// claim_lifecycle oversized still bounded.
	hugeClaimID := strings.Repeat("d", (1<<20)+10)
	_, err = callTool(t, cs, "claim_lifecycle", map[string]any{"operation": "prepare", "action": "approve", "claim_id": hugeClaimID})
	if !errors.As(err, &rpcErr) {
		t.Fatalf("oversized claim_lifecycle error type = %T, want *jsonrpc.Error (%v)", err, err)
	}
	if rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("oversized claim_lifecycle code = %d, want %d", rpcErr.Code, jsonrpc.CodeInvalidParams)
	}
}
