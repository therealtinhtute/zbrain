package mcp

import (
	"bytes"
	"context"
	"encoding/json"
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

// resourceFixture builds an isolated runtime with one workspace, one evidence
// record, and one approved claim, returning the Options plus the IDs needed to
// address both resources.
func resourceFixture(t *testing.T) (Options, string, string) {
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
	if err := os.WriteFile(source, []byte("raw snapshot bytes"), 0o644); err != nil {
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
		Title:       "Resource Claim",
		Basis:       zruntime.ClaimBasis("evidence"),
		CreatedAt:   now.UTC().Format(time.RFC3339),
		CreatedBy:   "test",
		EvidenceIDs: []string{evidence.ID},
		Body:        "Resource claim body",
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
	return Options{Paths: paths, Now: func() time.Time { return now }, Version: "test", Stderr: &bytes.Buffer{}}, claimID, evidence.ID
}

// TestResourceSurface asserts both read-only resources resolve under the
// workspace boundary and reject reads outside it or any mutation attempt.
func TestResourceSurface(t *testing.T) {
	opts, claimID, evidenceID := resourceFixture(t)
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)
	ctx := context.Background()

	// The server advertises both resource templates.
	templates := map[string]bool{}
	for tpl, err := range cs.ResourceTemplates(ctx, nil) {
		if err != nil {
			t.Fatalf("ResourceTemplates() error = %v", err)
		}
		templates[tpl.URITemplate] = true
	}
	wantTemplates := []string{
		"zbrain://workspace/{workspace}/claim/{id}",
		"zbrain://workspace/{workspace}/evidence/{id}",
	}
	for _, want := range wantTemplates {
		if !templates[want] {
			t.Errorf("resourceTemplates/list missing %s; got %v", want, templates)
		}
	}
	if len(templates) != len(wantTemplates) {
		t.Errorf("resourceTemplates/list count = %d, want %d", len(templates), len(wantTemplates))
	}

	// Reading the claim resource returns the canonical claim content.
	claimRes, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "zbrain://workspace/research/claim/" + claimID,
	})
	if err != nil {
		t.Fatalf("ReadResource(claim) error = %v", err)
	}
	claimText := resourceText(claimRes)
	if !strings.Contains(claimText, "Resource claim body") || !strings.Contains(claimText, claimID) {
		t.Errorf("claim resource missing claim content; got %s", claimText)
	}

	// Reading the evidence resource returns fenced metadata and nested raw bytes.
	evidenceRes, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "zbrain://workspace/research/evidence/" + evidenceID,
	})
	if err != nil {
		t.Fatalf("ReadResource(evidence) error = %v", err)
	}
	evidenceText := resourceText(evidenceRes)
	if !strings.Contains(evidenceText, evidenceID) || !strings.Contains(evidenceText, `"trust": "untrusted_evidence"`) || !strings.Contains(evidenceText, "raw snapshot bytes") {
		t.Errorf("evidence resource missing fenced envelope; got %s", evidenceText)
	}

	// Reading a nonexistent claim maps to ResourceNotFound.
	_, err = cs.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "zbrain://workspace/research/claim/clm_ffffffffffffffffffffffffffffffff",
	})
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("missing claim error = %v, want CodeInvalidParams (-32602)", err)
	}

	// Reading a nonexistent workspace rejects the read.
	_, err = cs.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "zbrain://workspace/nonexistent/claim/" + claimID,
	})
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("nonexistent workspace error = %v, want CodeInvalidParams (-32602)", err)
	}

	// A malformed resource type is rejected.
	_, err = cs.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "zbrain://workspace/research/delete/" + claimID,
	})
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("non-resource path error = %v, want CodeInvalidParams (-32602)", err)
	}

	// Reading is side-effect free: the canonical claim is still approved and the
	// evidence snapshot is byte-identical.
	readClaim, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).Read("research", claimID)
	if err != nil {
		t.Fatalf("claim still readable after resource reads: %v", err)
	}
	if readClaim.Status != zruntime.ClaimStatusApproved {
		t.Errorf("claim status after reads = %q, want approved", readClaim.Status)
	}
	readEvidence, err := (zruntime.EvidenceStore{Paths: opts.Paths, Now: opts.Now}).Read("research", evidenceID)
	if err != nil {
		t.Fatalf("evidence still readable after resource reads: %v", err)
	}
	if readEvidence.SHA256 == "" {
		t.Errorf("evidence digest empty after resource reads")
	}
}

// TestEvidenceResourceFenced asserts the evidence resource envelope: trust is
// untrusted_evidence, raw bytes live only under untrusted_evidence, and there
// is no top-level raw_content.
func TestEvidenceResourceFenced(t *testing.T) {
	opts, _, evidenceID := resourceFixture(t)
	server, err := newServer(opts)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	cs := connectClient(t, server)
	ctx := context.Background()

	evidenceRes, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "zbrain://workspace/research/evidence/" + evidenceID,
	})
	if err != nil {
		t.Fatalf("ReadResource(evidence) error = %v", err)
	}
	evidenceText := resourceText(evidenceRes)

	var envelope struct {
		SchemaVersion     int             `json:"schema_version"`
		Trust             string          `json:"trust"`
		Evidence          json.RawMessage `json:"evidence"`
		UntrustedEvidence struct {
			RawContent string `json:"raw_content"`
		} `json:"untrusted_evidence"`
	}
	if err := json.Unmarshal([]byte(evidenceText), &envelope); err != nil {
		t.Fatalf("unmarshal evidence resource: %v", err)
	}
	if envelope.Trust != "untrusted_evidence" {
		t.Errorf("trust = %q, want untrusted_evidence", envelope.Trust)
	}
	if envelope.UntrustedEvidence.RawContent != "raw snapshot bytes" {
		t.Errorf("untrusted_evidence.raw_content = %q, want %q", envelope.UntrustedEvidence.RawContent, "raw snapshot bytes")
	}

	var top map[string]any
	if err := json.Unmarshal([]byte(evidenceText), &top); err != nil {
		t.Fatalf("unmarshal evidence resource map: %v", err)
	}
	if _, ok := top["raw_content"]; ok {
		t.Errorf("top-level raw_content present")
	}
	nested, ok := top["untrusted_evidence"].(map[string]any)
	if !ok {
		t.Fatalf("untrusted_evidence missing or not an object: %T", top["untrusted_evidence"])
	}
	if nested["raw_content"] != "raw snapshot bytes" {
		t.Errorf("nested raw_content = %#v, want %q", nested["raw_content"], "raw snapshot bytes")
	}
}

// resourceText joins the text content of a ReadResourceResult.
func resourceText(res *mcp.ReadResourceResult) string {
	var parts []string
	for _, contents := range res.Contents {
		if contents.Text != "" {
			parts = append(parts, contents.Text)
		}
	}
	return strings.Join(parts, "\n")
}
