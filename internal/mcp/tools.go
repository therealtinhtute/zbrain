package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

// registerTools registers the seven typed gateway tools.
//
// Domain failures return a regular error, which the SDK embeds in a
// CallToolResult with IsError set; structurally invalid parameters are rejected
// by the generated JSON schema with -32602; genuine server failures surface as
// -32603. No tool exposes grant/approval UI or mutation HTTP behavior.
func registerTools(server *mcp.Server, opts Options) error {
	type currentIn struct{}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "workspace_current",
		Description: "Resolve the current workspace boundary.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in currentIn) (*mcp.CallToolResult, any, error) {
		current, err := zruntime.ResolveCurrentWorkspace(opts.Paths)
		if err != nil {
			return nil, nil, err
		}
		out := struct {
			SchemaVersion       int      `json:"schema_version"`
			ProjectRoot         string   `json:"project_root"`
			Workspace           string   `json:"workspace"`
			SecondaryWorkspaces []string `json:"secondary_workspaces"`
		}{1, current.ProjectRoot, current.Workspace, current.SecondaryWorkspaces}
		return jsonResult(out)
	})

	type askIn struct {
		Query     string   `json:"query" jsonschema:"the trusted-memory query"`
		Workspace string   `json:"workspace,omitempty" jsonschema:"workspace to query; defaults to the current workspace"`
		Include   []string `json:"include,omitempty" jsonschema:"read-only secondary workspace to include"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_ask",
		Description: "Query trusted memory and return trusted context JSON without calling an LLM.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in askIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Query) == "" {
			return nil, nil, fmt.Errorf("query is required")
		}
		workspace, err := resolveWorkspace(opts, in.Workspace)
		if err != nil {
			return nil, nil, err
		}
		response, err := zruntime.TrustedQuery(opts.Paths, zruntime.TrustedQueryOptions{
			Workspace: workspace,
			Includes:  in.Include,
			Query:     in.Query,
			Limit:     10,
		})
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(response)
	})

	type statusIn struct {
		Workspace string `json:"workspace,omitempty" jsonschema:"workspace to inspect; defaults to the current workspace"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_status",
		Description: "Report machine-readable trust, claim, and index health for a workspace.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in statusIn) (*mcp.CallToolResult, any, error) {
		workspace, err := resolveWorkspace(opts, in.Workspace)
		if err != nil {
			return nil, nil, err
		}
		summary := zruntime.IndexSummary{
			Workspace: workspace,
			Embedding: zruntime.EmbeddingSummary{Strategy: "lexical", Degraded: "embeddings not configured"},
		}
		if scan, scanErr := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).ScanWorkspaceForTrust(workspace); scanErr == nil {
			summary.Approved = len(scan.Claims)
			summary.Invalid = len(scan.Invalid)
			summary.InvalidCount = len(scan.Invalid)
			summary.InvalidClaims = scan.Invalid
		}
		if err := (zruntime.IndexStore{Paths: opts.Paths}).CheckFresh(workspace); err != nil {
			summary.RebuildState = zruntime.RebuildStatusRejected
			if summary.Invalid == 0 {
				summary.InvalidClaims = []zruntime.InvalidClaim{{Path: "", Error: err.Error()}}
			}
		}
		out := struct {
			SchemaVersion int `json:"schema_version"`
			zruntime.IndexSummary
		}{2, summary}
		return jsonResult(out)
	})

	type reindexIn struct {
		Workspace string `json:"workspace,omitempty" jsonschema:"workspace whose derived index to rebuild; defaults to the current workspace"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_reindex",
		Description: "Rebuild the disposable derived index for a workspace.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in reindexIn) (*mcp.CallToolResult, any, error) {
		workspace, err := resolveWorkspace(opts, in.Workspace)
		if err != nil {
			return nil, nil, err
		}
		summary, err := (zruntime.IndexStore{Paths: opts.Paths}).Rebuild(workspace)
		if err != nil {
			return nil, nil, err
		}
		out := struct {
			SchemaVersion int `json:"schema_version"`
			zruntime.IndexSummary
		}{1, summary}
		return jsonResult(out)
	})

	type evidenceCaptureIn struct {
		Workspace string `json:"workspace,omitempty" jsonschema:"target workspace; defaults to the current workspace"`
		File      string `json:"file" jsonschema:"local source file to snapshot"`
		Origin    string `json:"origin" jsonschema:"origin URI or path recorded in the evidence metadata"`
		MediaType string `json:"media_type,omitempty" jsonschema:"optional media type"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "evidence_capture",
		Description: "Snapshot a local source file into an immutable evidence record.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in evidenceCaptureIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.File) == "" {
			return nil, nil, fmt.Errorf("file is required")
		}
		if strings.TrimSpace(in.Origin) == "" {
			return nil, nil, fmt.Errorf("origin is required")
		}
		workspace, err := resolveWorkspace(opts, in.Workspace)
		if err != nil {
			return nil, nil, err
		}
		evidence, err := (zruntime.EvidenceStore{Paths: opts.Paths, Now: opts.Now}).AddFile(workspace, in.File, in.Origin, in.MediaType)
		if err != nil {
			return nil, nil, err
		}
		out := struct {
			SchemaVersion int `json:"schema_version"`
			Workspace     string `json:"workspace"`
			zruntime.Evidence
		}{1, workspace, evidence}
		return jsonResult(out)
	})

	type claimDraftIn struct {
		Workspace     string   `json:"workspace,omitempty" jsonschema:"target workspace; defaults to the current workspace"`
		Tier          string   `json:"tier" jsonschema:"claim tier"`
		Title         string   `json:"title" jsonschema:"claim title"`
		Basis         string   `json:"basis" jsonschema:"owner, evidence, or derived"`
		Evidence      []string `json:"evidence,omitempty" jsonschema:"evidence IDs to bind"`
		Support       []string `json:"support,omitempty" jsonschema:"supporting claim IDs"`
		ConflictsWith []string `json:"conflicts_with,omitempty" jsonschema:"conflicting claim IDs"`
		Body          string   `json:"body" jsonschema:"claim body"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "claim_draft",
		Description: "Create a draft claim as a promotion candidate (drafts are never trusted answer material).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in claimDraftIn) (*mcp.CallToolResult, any, error) {
		workspace, err := resolveWorkspace(opts, in.Workspace)
		if err != nil {
			return nil, nil, err
		}
		id, err := zruntime.NewClaimID()
		if err != nil {
			return nil, nil, err
		}
		claim := zruntime.Claim{
			Type:               zruntime.OKFClaimType,
			ID:                 id,
			Tier:               in.Tier,
			Status:             zruntime.ClaimStatusDraft,
			Title:              in.Title,
			Basis:              zruntime.ClaimBasis(in.Basis),
			CreatedAt:          opts.Now().UTC().Format(time.RFC3339),
			CreatedBy:          "owner:mcp",
			EvidenceIDs:        in.Evidence,
			SupportingClaimIDs: in.Support,
			ConflictsWith:      in.ConflictsWith,
			Body:               in.Body,
		}
		if err := (zruntime.IndexStore{Paths: opts.Paths}).MarkDirty(workspace); err != nil {
			return nil, nil, err
		}
		created, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).WriteDraft(workspace, claim)
		if err != nil {
			return nil, nil, err
		}
		out := struct {
			SchemaVersion int                  `json:"schema_version"`
			Workspace     string               `json:"workspace"`
			ID            string               `json:"id"`
			Status        zruntime.ClaimStatus `json:"status"`
			Path          string               `json:"path"`
		}{1, workspace, created.ID, created.Status, created.Path}
		return jsonResult(out)
	})

	type lifecycleIn struct {
		Operation string `json:"operation" jsonschema:"prepare or apply"`
		Workspace string `json:"workspace,omitempty" jsonschema:"target workspace; defaults to the current workspace"`
		ClaimID   string `json:"claim_id,omitempty" jsonschema:"target claim ID"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "claim_lifecycle",
		Description: "Owner-pinned lifecycle challenge. Fails closed: prepare and apply are unavailable until the owner-pinned-lifecycle phase lands.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in lifecycleIn) (*mcp.CallToolResult, any, error) {
		return nil, nil, fmt.Errorf("claim_lifecycle %s is unavailable: the owner-pinned-lifecycle phase has not landed yet", in.Operation)
	})

	return nil
}

// resolveWorkspace resolves an explicit workspace name or falls back to the
// current workspace, always validating the boundary through the runtime.
func resolveWorkspace(opts Options, name string) (string, error) {
	if name == "" {
		current, err := zruntime.ResolveCurrentWorkspace(opts.Paths)
		if err != nil {
			return "", err
		}
		name = current.Workspace
	}
	if _, err := zruntime.ValidateWorkspace(opts.Paths, name); err != nil {
		return "", err
	}
	return name, nil
}

// jsonResult renders out as both text content and structured content.
func jsonResult(out any) (*mcp.CallToolResult, any, error) {
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("{\"error\": %q}", err.Error())}},
			IsError: true,
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
	}, out, nil
}
