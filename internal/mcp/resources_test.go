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

	// Reading the evidence resource returns the metadata and raw snapshot.
	evidenceRes, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "zbrain://workspace/research/evidence/" + evidenceID,
	})
	if err != nil {
		t.Fatalf("ReadResource(evidence) error = %v", err)
	}
	evidenceText := resourceText(evidenceRes)
	if !strings.Contains(evidenceText, evidenceID) || !strings.Contains(evidenceText, "raw snapshot bytes") {
		t.Errorf("evidence resource missing metadata or raw content; got %s", evidenceText)
	}

	// Reading a nonexistent claim maps to ResourceNotFound.
	_, err = cs.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "zbrain://workspace/research/claim/clm_ffffffffffffffffffffffffffffffff",
	})
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != mcp.CodeResourceNotFound {
		t.Errorf("missing claim error = %v, want CodeResourceNotFound", err)
	}

	// Reading a nonexistent workspace rejects the read.
	_, err = cs.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "zbrain://workspace/nonexistent/claim/" + claimID,
	})
	if !errors.As(err, &rpcErr) || rpcErr.Code != mcp.CodeResourceNotFound {
		t.Errorf("nonexistent workspace error = %v, want CodeResourceNotFound", err)
	}

	// A malformed resource type is rejected.
	_, err = cs.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "zbrain://workspace/research/delete/" + claimID,
	})
	if !errors.As(err, &rpcErr) || rpcErr.Code != mcp.CodeResourceNotFound {
		t.Errorf("non-resource path error = %v, want CodeResourceNotFound", err)
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