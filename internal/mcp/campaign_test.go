package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

func campaignSpecArgs() []map[string]any {
	return []map[string]any{
		{"tier": "projects", "title": "Campaign MCP One", "basis": "owner"},
		{"tier": "projects", "title": "Campaign MCP Two", "basis": "evidence"},
	}
}

func TestCampaignToolSurface(t *testing.T) {
	opts := testOptions(t)
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)
	toolNames := map[string]bool{}
	for tool, err := range cs.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatalf("Tools() error = %v", err)
		}
		toolNames[tool.Name] = true
	}
	for _, name := range []string{"campaign_begin", "campaign_next", "campaign_submit_draft"} {
		if !toolNames[name] {
			t.Errorf("tools/list missing %s; got %v", name, toolNames)
		}
	}
}

func TestCampaignToolsAuthorDraftsOnly(t *testing.T) {
	opts := testOptions(t)
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)

	before := readWorkspaceGeneration(t, opts)
	begin, err := callTool(t, cs, "campaign_begin", map[string]any{"workspace": "research", "specs": campaignSpecArgs()})
	if err != nil || begin.IsError {
		t.Fatalf("campaign_begin error=%v isError=%v text=%s", err, begin != nil && begin.IsError, resultText(begin))
	}
	var begun struct {
		RunID string `json:"run_id"`
		Phase string `json:"phase"`
		Total int    `json:"total_drafts"`
	}
	if err := json.Unmarshal([]byte(resultText(begin)), &begun); err != nil {
		t.Fatalf("decode campaign_begin: %v", err)
	}
	if begun.Phase != "drafting" || begun.Total != 2 || !strings.HasPrefix(begun.RunID, "cmp_") {
		t.Fatalf("campaign_begin result = %#v", begun)
	}

	next, err := callTool(t, cs, "campaign_next", map[string]any{"workspace": "research", "run_id": begun.RunID})
	if err != nil || next.IsError {
		t.Fatalf("campaign_next error=%v isError=%v text=%s", err, next != nil && next.IsError, resultText(next))
	}
	var state struct {
		Phase     string `json:"phase"`
		Pending   int    `json:"pending"`
		NextIndex int    `json:"next_index"`
		NextSpec  *struct {
			Title string `json:"title"`
		} `json:"next_spec"`
	}
	if err := json.Unmarshal([]byte(resultText(next)), &state); err != nil {
		t.Fatalf("decode campaign_next: %v", err)
	}
	if state.Phase != "drafting" || state.Pending != 2 || state.NextIndex != 0 || state.NextSpec == nil || state.NextSpec.Title != "Campaign MCP One" {
		t.Fatalf("campaign_next state = %#v", state)
	}

	first, err := callTool(t, cs, "campaign_submit_draft", map[string]any{"workspace": "research", "run_id": begun.RunID, "index": 0, "body": "first body\n"})
	if err != nil || first.IsError {
		t.Fatalf("campaign_submit_draft error=%v isError=%v text=%s", err, first != nil && first.IsError, resultText(first))
	}
	var submitted struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal([]byte(resultText(first)), &submitted); err != nil {
		t.Fatalf("decode campaign_submit_draft: %v", err)
	}
	if submitted.Status != string(zruntime.ClaimStatusDraft) {
		t.Fatalf("submission status = %q, want draft", submitted.Status)
	}
	claim, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).Read("research", submitted.ID)
	if err != nil {
		t.Fatalf("Read(submitted claim) error = %v", err)
	}
	if claim.Status != zruntime.ClaimStatusDraft || claim.VerifiedDigest != "" || len(claim.Transitions) != 0 {
		t.Fatalf("submitted claim = %#v, want unverified draft without transitions", claim)
	}

	// Submitting the same index twice fails closed as isError.
	replay, err := callTool(t, cs, "campaign_submit_draft", map[string]any{"workspace": "research", "run_id": begun.RunID, "index": 0, "body": "again\n"})
	if err != nil || replay == nil || !replay.IsError || !strings.Contains(resultText(replay), "only pending drafts") {
		t.Fatalf("replay submit error=%v result=%#v text=%s", err, replay, resultText(replay))
	}

	second, err := callTool(t, cs, "campaign_submit_draft", map[string]any{"run_id": begun.RunID, "index": 1, "body": "second body\n"})
	if err != nil || second.IsError {
		t.Fatalf("campaign_submit_draft(1) error=%v isError=%v text=%s", err, second != nil && second.IsError, resultText(second))
	}

	after := readWorkspaceGeneration(t, opts)
	if after.Published != before.Published {
		t.Fatalf("campaign tools published generation %d, want unchanged %d", after.Published, before.Published)
	}
	dirtyPath, err := (zruntime.IndexStore{Paths: opts.Paths}).DirtyPath("research")
	if err != nil {
		t.Fatalf("DirtyPath() error = %v", err)
	}
	if _, err := os.Stat(dirtyPath); err != nil {
		t.Fatalf("campaign tools did not leave the index dirty: %v", err)
	}

	exhausted, err := callTool(t, cs, "campaign_next", map[string]any{"run_id": begun.RunID})
	if err != nil || exhausted.IsError {
		t.Fatalf("campaign_next(exhausted) error=%v isError=%v text=%s", err, exhausted != nil && exhausted.IsError, resultText(exhausted))
	}
	var drained struct {
		Pending   int `json:"pending"`
		NextIndex int `json:"next_index"`
	}
	if err := json.Unmarshal([]byte(resultText(exhausted)), &drained); err != nil {
		t.Fatalf("decode drained campaign_next: %v", err)
	}
	if drained.Pending != 0 || drained.NextIndex != -1 {
		t.Fatalf("drained state = %#v", drained)
	}
}

func TestCampaignToolsFailClosed(t *testing.T) {
	opts := testOptions(t)
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)

	invalidSpec, err := callTool(t, cs, "campaign_begin", map[string]any{
		"specs": []map[string]any{{"tier": "not-a-tier", "title": "Bad", "basis": "owner"}},
	})
	if err != nil || invalidSpec == nil || !invalidSpec.IsError || !strings.Contains(resultText(invalidSpec), "campaign spec 0") {
		t.Fatalf("invalid spec error=%v result=%#v text=%s", err, invalidSpec, resultText(invalidSpec))
	}
	emptySpecs, err := callTool(t, cs, "campaign_begin", map[string]any{"specs": []map[string]any{}})
	if err != nil || emptySpecs == nil || !emptySpecs.IsError || !strings.Contains(resultText(emptySpecs), "at least one draft spec") {
		t.Fatalf("empty specs error=%v result=%#v text=%s", err, emptySpecs, resultText(emptySpecs))
	}
	unknownRun, err := callTool(t, cs, "campaign_next", map[string]any{"run_id": "cmp_00000000000000000000000000000000"})
	if err != nil || unknownRun == nil || !unknownRun.IsError || !strings.Contains(resultText(unknownRun), "not found") {
		t.Fatalf("unknown run error=%v result=%#v text=%s", err, unknownRun, resultText(unknownRun))
	}
	malformedRun, err := callTool(t, cs, "campaign_next", map[string]any{"run_id": "not-a-run-id"})
	if err != nil || malformedRun == nil || !malformedRun.IsError {
		t.Fatalf("malformed run id error=%v result=%#v text=%s", err, malformedRun, resultText(malformedRun))
	}
	unknownSubmit, err := callTool(t, cs, "campaign_submit_draft", map[string]any{"run_id": "cmp_00000000000000000000000000000000", "index": 0, "body": "body"})
	if err != nil || unknownSubmit == nil || !unknownSubmit.IsError {
		t.Fatalf("unknown submit error=%v result=%#v text=%s", err, unknownSubmit, resultText(unknownSubmit))
	}

	begin, err := callTool(t, cs, "campaign_begin", map[string]any{"workspace": "research", "specs": campaignSpecArgs()})
	if err != nil || begin.IsError {
		t.Fatalf("campaign_begin error=%v isError=%v text=%s", err, begin != nil && begin.IsError, resultText(begin))
	}
	var begun struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(resultText(begin)), &begun); err != nil {
		t.Fatalf("decode campaign_begin: %v", err)
	}
	badIndex, err := callTool(t, cs, "campaign_submit_draft", map[string]any{"run_id": begun.RunID, "index": 7, "body": "body"})
	if err != nil || badIndex == nil || !badIndex.IsError || !strings.Contains(resultText(badIndex), "out of range") {
		t.Fatalf("bad index error=%v result=%#v text=%s", err, badIndex, resultText(badIndex))
	}
	if _, err := callTool(t, cs, "campaign_submit_draft", map[string]any{"run_id": begun.RunID, "index": 0, "body": "body"}); err != nil {
		t.Fatalf("valid submit error = %v", err)
	}
	twice, err := callTool(t, cs, "campaign_submit_draft", map[string]any{"run_id": begun.RunID, "index": 0, "body": "body"})
	if err != nil || twice == nil || !twice.IsError || !strings.Contains(resultText(twice), "only pending drafts") {
		t.Fatalf("submitted-twice error=%v result=%#v text=%s", err, twice, resultText(twice))
	}
}

func TestCampaignToolsWorkspaceBindingMatchesClaimDraft(t *testing.T) {
	opts := testOptions(t)
	if err := zruntime.CreateWorkspace(opts.Paths, "other", opts.Now()); err != nil {
		t.Fatalf("CreateWorkspace(other) error = %v", err)
	}
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)

	// Default workspace binding: no workspace param behaves like claim_draft.
	beginDefault, err := callTool(t, cs, "campaign_begin", map[string]any{"specs": campaignSpecArgs()})
	if err != nil || beginDefault.IsError {
		t.Fatalf("campaign_begin(default) error=%v isError=%v text=%s", err, beginDefault != nil && beginDefault.IsError, resultText(beginDefault))
	}
	var begun struct {
		Workspace string `json:"workspace"`
		RunID     string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(resultText(beginDefault)), &begun); err != nil {
		t.Fatalf("decode campaign_begin(default): %v", err)
	}
	if begun.Workspace != "research" {
		t.Fatalf("campaign_begin workspace = %q, want default research", begun.Workspace)
	}

	// Explicit workspace binding.
	beginOther, err := callTool(t, cs, "campaign_begin", map[string]any{"workspace": "other", "specs": campaignSpecArgs()})
	if err != nil || beginOther.IsError {
		t.Fatalf("campaign_begin(other) error=%v isError=%v text=%s", err, beginOther != nil && beginOther.IsError, resultText(beginOther))
	}
	var otherBegun struct {
		Workspace string `json:"workspace"`
		RunID     string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(resultText(beginOther)), &otherBegun); err != nil {
		t.Fatalf("decode campaign_begin(other): %v", err)
	}
	if otherBegun.Workspace != "other" {
		t.Fatalf("campaign_begin(other) workspace = %q", otherBegun.Workspace)
	}
	if _, err := os.Stat(filepath.Join(opts.Paths.WorkspacesDir, "other", "campaigns", otherBegun.RunID+".json")); err != nil {
		t.Fatalf("run file not bound to workspace other: %v", err)
	}

	// A run started in one workspace is invisible from another, matching the
	// claim_draft workspace boundary.
	crossWorkspace, err := callTool(t, cs, "campaign_next", map[string]any{"workspace": "other", "run_id": begun.RunID})
	if err != nil || crossWorkspace == nil || !crossWorkspace.IsError || !strings.Contains(resultText(crossWorkspace), "not found") {
		t.Fatalf("cross-workspace next error=%v result=%#v text=%s", err, crossWorkspace, resultText(crossWorkspace))
	}
	nonexistent, err := callTool(t, cs, "campaign_begin", map[string]any{"workspace": "nonexistent", "specs": campaignSpecArgs()})
	if err != nil || nonexistent == nil || !nonexistent.IsError {
		t.Fatalf("nonexistent workspace error=%v result=%#v text=%s", err, nonexistent, resultText(nonexistent))
	}
}

func readWorkspaceGeneration(t *testing.T, opts Options) zruntime.WorkspaceGeneration {
	t.Helper()
	path := filepath.Join(opts.Paths.WorkspacesDir, "research", ".zbrain", "generation.json")
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return zruntime.WorkspaceGeneration{}
	}
	if err != nil {
		t.Fatalf("ReadFile(generation) error = %v", err)
	}
	var generation zruntime.WorkspaceGeneration
	if err := json.Unmarshal(contents, &generation); err != nil {
		t.Fatalf("decode generation: %v", err)
	}
	return generation
}
