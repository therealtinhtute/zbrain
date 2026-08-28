package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

const mcpMaxInputBytes = 1 << 20 // 1 MiB

var mcpLogMu sync.Mutex

func validateBounds(in any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "invalid params"}
	}
	if len(data) > mcpMaxInputBytes {
		return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "input exceeds 1MB limit"}
	}
	return nil
}

func runMCPTool[T any](ctx context.Context, opts Options, tool string, in T, workspace string, fn func(context.Context) (*mcp.CallToolResult, any, error)) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	ws := workspace
	if ws == "" {
		ws = "current"
	}
	defer func() {
		if opts.Stderr != nil {
			mcpLogMu.Lock()
			_, _ = fmt.Fprintf(opts.Stderr, "mcp tool=%s workspace=%s duration=%s\n", tool, ws, time.Since(start))
			mcpLogMu.Unlock()
		}
	}()
	if err := validateBounds(in); err != nil {
		return nil, nil, err
	}
	// Use context timeout but call handler synchronously to avoid
	// goroutine leaks and shared-buffer races under -race with concurrent
	// tool calls (e.g., TestClaimLifecycleConcurrentApply).
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// If parent context already cancelled, fail fast without invoking handler.
	select {
	case <-tctx.Done():
		if tctx.Err() == context.DeadlineExceeded {
			return nil, nil, &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "tool timeout"}
		}
		return nil, nil, tctx.Err()
	default:
	}
	res, out, err := fn(tctx)
	// If handler exceeded deadline, surface timeout.
	if tctx.Err() == context.DeadlineExceeded {
		return nil, nil, &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "tool timeout"}
	}
	return res, out, err
}

type lifecycleIn struct {
	Operation               string    `json:"operation" jsonschema:"prepare or apply"`
	Action                  string    `json:"action,omitempty" jsonschema:"approve, supersede, or revoke for prepare"`
	Workspace               string    `json:"workspace,omitempty" jsonschema:"target workspace; defaults to the current workspace for prepare; apply resolves the challenge owner"`
	ClaimID                 string    `json:"claim_id,omitempty" jsonschema:"target claim ID for prepare or optional apply assertion"`
	ChallengeID             string    `json:"challenge_id,omitempty" jsonschema:"challenge ID for apply"`
	Token                   string    `json:"token,omitempty" jsonschema:"one-time challenge token for apply"`
	CanonicalDraftDigest    string    `json:"canonical_draft_digest,omitempty" jsonschema:"canonical draft digest bound to the action"`
	SupersededIDs           *[]string `json:"superseded_ids,omitempty" jsonschema:"canonical superseded claim IDs bound to the action"`
	PriorVerificationDigest string    `json:"prior_verification_digest,omitempty" jsonschema:"prior verification digest bound to the action"`
	RevokeReason            string    `json:"revoke_reason,omitempty" jsonschema:"reason bound to a revoke action"`
}

type lifecycleActionSummary struct {
	Action                  string   `json:"action"`
	Workspace               string   `json:"workspace"`
	ClaimID                 string   `json:"claim_id"`
	CanonicalDraftDigest    string   `json:"canonical_draft_digest"`
	SupersededIDs           []string `json:"superseded_ids"`
	PriorVerificationDigest string   `json:"prior_verification_digest"`
	RevokeReason            string   `json:"revoke_reason"`
}

type lifecycleResult struct {
	SchemaVersion  int                    `json:"schema_version"`
	Operation      string                 `json:"operation"`
	Action         string                 `json:"action"`
	Workspace      string                 `json:"workspace"`
	ClaimID        string                 `json:"claim_id"`
	ChallengeID    string                 `json:"challenge_id"`
	ActionSummary  lifecycleActionSummary `json:"action_summary"`
	ActionDigest   string                 `json:"action_digest"`
	ExpiresAt      string                 `json:"expires_at"`
	TokenExpiresAt string                 `json:"token_expires_at,omitempty"`
	Token          string                 `json:"token,omitempty"`
	Status         zruntime.ClaimStatus   `json:"status,omitempty"`
	VerifiedBy     string                 `json:"verified_by,omitempty"`
	Claim          *zruntime.Claim        `json:"claim,omitempty"`
}

// registerTools registers the seven typed gateway tools.
//
// Domain failures return a regular error, which the SDK embeds in a
// CallToolResult with IsError set; structurally invalid parameters fail
// closed as tool-level isError results via generated JSON schema
// validation; oversized inputs and unknown tools map to -32602; genuine
// server failures surface as -32603. No tool exposes grant/approval UI or
// mutation HTTP behavior.
func registerTools(server *mcp.Server, opts Options) error {
	type currentIn struct{}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "workspace_current",
		Description: "Resolve the current workspace boundary.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in currentIn) (*mcp.CallToolResult, any, error) {
		return runMCPTool(ctx, opts, "workspace_current", in, "", func(tctx context.Context) (*mcp.CallToolResult, any, error) {
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
	})

	type askIn struct {
		Query     string   `json:"query" jsonschema:"the trusted-memory query"`
		Workspace string   `json:"workspace,omitempty" jsonschema:"workspace to query; defaults to the current workspace"`
		Include   []string `json:"include,omitempty" jsonschema:"read-only secondary workspace to include"`
		Embedding bool     `json:"embedding,omitempty" jsonschema:"enable local hybrid embedding retrieval; defaults to false"`
		After     string   `json:"after,omitempty" jsonschema:"filter claims verified/created at or after RFC3339 timestamp"`
		Before    string   `json:"before,omitempty" jsonschema:"filter claims verified/created at or before RFC3339 timestamp"`
		AsOf      string   `json:"as_of,omitempty" jsonschema:"reconstruct active memory state as of RFC3339 timestamp"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_ask",
		Description: "Query trusted memory and return trusted context JSON without calling an LLM.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in askIn) (*mcp.CallToolResult, any, error) {
		return runMCPTool(ctx, opts, "memory_ask", in, in.Workspace, func(tctx context.Context) (*mcp.CallToolResult, any, error) {
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
				Embedding: in.Embedding,
				After:     in.After,
				Before:    in.Before,
				AsOf:      in.AsOf,
			})
			if err != nil {
				return nil, nil, err
			}
			return jsonResult(response)
		})
	})

	type statusIn struct {
		Workspace string `json:"workspace,omitempty" jsonschema:"workspace to inspect; defaults to the current workspace"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_status",
		Description: "Report machine-readable trust, claim, and index health for a workspace.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in statusIn) (*mcp.CallToolResult, any, error) {
		return runMCPTool(ctx, opts, "memory_status", in, in.Workspace, func(tctx context.Context) (*mcp.CallToolResult, any, error) {
			workspace, err := resolveWorkspace(opts, in.Workspace)
			if err != nil {
				return nil, nil, err
			}
			summary := zruntime.IndexSummary{Workspace: workspace}
			if scan, scanErr := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).ScanWorkspaceForTrust(workspace); scanErr == nil {
				summary.Approved = len(scan.Claims)
				summary.Invalid = len(scan.Invalid)
				summary.InvalidCount = len(scan.Invalid)
				summary.InvalidClaims = scan.Invalid
			}
			summary.Embedding = (zruntime.EmbeddingStore{Paths: opts.Paths}).Summary(workspace, summary.Approved)
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
	})

	type reindexIn struct {
		Workspace string `json:"workspace,omitempty" jsonschema:"workspace whose derived index to rebuild; defaults to the current workspace"`
		Embedding bool   `json:"embedding,omitempty" jsonschema:"enable local loopback embedding; defaults to false"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_reindex",
		Description: "Rebuild the disposable derived index for a workspace.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in reindexIn) (*mcp.CallToolResult, any, error) {
		return runMCPTool(ctx, opts, "memory_reindex", in, in.Workspace, func(tctx context.Context) (*mcp.CallToolResult, any, error) {
			workspace, err := resolveWorkspace(opts, in.Workspace)
			if err != nil {
				return nil, nil, err
			}
			summary, err := (zruntime.IndexStore{Paths: opts.Paths}).RebuildWithOptions(workspace, zruntime.RebuildOptions{Embedding: in.Embedding})
			if err != nil {
				return nil, nil, err
			}
			out := struct {
				SchemaVersion int `json:"schema_version"`
				zruntime.IndexSummary
			}{1, summary}
			return jsonResult(out)
		})
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
		return runMCPTool(ctx, opts, "evidence_capture", in, in.Workspace, func(tctx context.Context) (*mcp.CallToolResult, any, error) {
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
				SchemaVersion int    `json:"schema_version"`
				Workspace     string `json:"workspace"`
				zruntime.Evidence
			}{1, workspace, evidence}
			return jsonResult(out)
		})
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
		return runMCPTool(ctx, opts, "claim_draft", in, in.Workspace, func(tctx context.Context) (*mcp.CallToolResult, any, error) {
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
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "claim_lifecycle",
		Description: "Prepare an owner-pinned lifecycle challenge or apply one valid one-time token; no approval UI or HTTP mutation endpoint is exposed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in lifecycleIn) (*mcp.CallToolResult, any, error) {
		return runMCPTool(ctx, opts, "claim_lifecycle", in, in.Workspace, func(tctx context.Context) (*mcp.CallToolResult, any, error) {
			switch in.Operation {
			case "prepare":
				return prepareLifecycle(opts, in)
			case "apply":
				return applyLifecycle(opts, req, in)
			default:
				return nil, nil, invalidLifecycleParams("operation must be prepare or apply")
			}
		})
	})

	return nil
}

func prepareLifecycle(opts Options, in lifecycleIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Action) == "" {
		return nil, nil, invalidLifecycleParams("action is required for prepare")
	}
	action, err := lifecycleAction(in.Action)
	if err != nil {
		return nil, nil, err
	}
	workspace, err := resolveWorkspace(opts, in.Workspace)
	if err != nil {
		return nil, nil, err
	}
	claimID := strings.TrimSpace(in.ClaimID)
	if claimID == "" {
		return nil, nil, invalidLifecycleParams("claim_id is required for prepare")
	}
	store := zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}
	claim, err := store.Read(workspace, claimID)
	if err != nil {
		return nil, nil, err
	}
	canonicalDigest, err := store.CanonicalDraftDigest(workspace, claimID)
	if err != nil {
		return nil, nil, err
	}
	if in.CanonicalDraftDigest != "" && in.CanonicalDraftDigest != canonicalDigest {
		return nil, nil, fmt.Errorf("canonical draft digest does not match the current claim")
	}

	supersededIDs := append([]string(nil), claim.Supersedes...)
	sort.Strings(supersededIDs)
	if in.SupersededIDs != nil && !sameLifecycleIDs(*in.SupersededIDs, supersededIDs) {
		return nil, nil, fmt.Errorf("superseded IDs do not match the current claim")
	}

	priorVerificationDigest := ""
	switch action {
	case zruntime.ChallengeOperationSupersede:
		priorVerificationDigest, err = supersededPriorVerificationDigest(store, workspace, supersededIDs)
		if err != nil {
			return nil, nil, err
		}
	case zruntime.ChallengeOperationRevoke:
		if claim.Status == zruntime.ClaimStatusApproved {
			if err := zruntime.VerifyClaimDigest(claim); err != nil {
				return nil, nil, fmt.Errorf("verify claim %s before revoke: %w", claimID, err)
			}
			priorVerificationDigest = claim.VerifiedDigest
		}
	}
	if in.PriorVerificationDigest != "" && in.PriorVerificationDigest != priorVerificationDigest {
		return nil, nil, fmt.Errorf("prior verification digest does not match the current claim")
	}

	revokeReason := in.RevokeReason
	if action == zruntime.ChallengeOperationRevoke {
		if strings.TrimSpace(revokeReason) == "" {
			return nil, nil, invalidLifecycleParams("revoke_reason is required for revoke")
		}
	} else if revokeReason != "" {
		return nil, nil, fmt.Errorf("revoke_reason is only valid for revoke")
	}

	prepared, err := store.PrepareChallenge(workspace, zruntime.ChallengePrepare{
		Workspace:               workspace,
		Operation:               action,
		ClaimID:                 claimID,
		CanonicalDraftDigest:    canonicalDigest,
		SupersededIDs:           supersededIDs,
		PriorVerificationDigest: priorVerificationDigest,
		RevokeReason:            revokeReason,
	})
	if err != nil {
		return nil, nil, err
	}
	challenge := prepared.Challenge
	result := lifecycleResult{
		SchemaVersion: 1,
		Operation:     "prepare",
		Action:        string(challenge.Operation),
		Workspace:     workspace,
		ClaimID:       challenge.ClaimID,
		ChallengeID:   challenge.ID,
		ActionSummary: lifecycleActionSummaryFromChallenge(challenge),
		ActionDigest:  challenge.ActionDigest,
		ExpiresAt:     challenge.ExpiresAt,
	}
	return jsonResult(result)
}

func applyLifecycle(opts Options, req *mcp.CallToolRequest, in lifecycleIn) (*mcp.CallToolResult, any, error) {
	challengeID := strings.TrimSpace(in.ChallengeID)
	if challengeID == "" {
		return nil, nil, invalidLifecycleParams("challenge_id is required for apply")
	}
	if in.Token == "" {
		return nil, nil, invalidLifecycleParams("token is required for apply")
	}

	challengeStore := zruntime.ChallengeStore{Paths: opts.Paths, Now: opts.Now}
	challenge, workspace, err := challengeStore.FindChallenge(challengeID)
	if err != nil {
		return nil, nil, err
	}
	if in.Workspace != "" && in.Workspace != workspace {
		return nil, nil, fmt.Errorf("workspace %q does not own challenge %s", in.Workspace, challengeID)
	}
	if in.ClaimID != "" && strings.TrimSpace(in.ClaimID) != challenge.ClaimID {
		return nil, nil, fmt.Errorf("claim_id does not match challenge %s", challengeID)
	}
	if in.Action != "" {
		action, err := lifecycleAction(in.Action)
		if err != nil {
			return nil, nil, err
		}
		if action != challenge.Operation {
			return nil, nil, fmt.Errorf("action %q does not match challenge %s action %q", in.Action, challengeID, challenge.Operation)
		}
	}
	if in.CanonicalDraftDigest != "" && in.CanonicalDraftDigest != challenge.CanonicalDraftDigest {
		return nil, nil, fmt.Errorf("canonical draft digest does not match challenge %s", challengeID)
	}
	if in.SupersededIDs != nil && !sameLifecycleIDs(*in.SupersededIDs, challenge.SupersededIDs) {
		return nil, nil, fmt.Errorf("superseded IDs do not match challenge %s", challengeID)
	}
	if in.PriorVerificationDigest != "" && in.PriorVerificationDigest != challenge.PriorVerificationDigest {
		return nil, nil, fmt.Errorf("prior verification digest does not match challenge %s", challengeID)
	}
	if in.RevokeReason != "" && in.RevokeReason != challenge.RevokeReason {
		return nil, nil, fmt.Errorf("revoke reason does not match challenge %s", challengeID)
	}

	claim, err := (zruntime.ClaimStore{Paths: opts.Paths, Now: opts.Now}).ApplyChallenge(
		workspace,
		challengeID,
		in.Token,
		zruntime.ClaimMutationOptions{
			VerifiedBy: "owner:mcp",
			Authorization: &zruntime.ClaimTransitionAuthorization{
				ChallengeID: challenge.ID,
				Method:      "mcp.claim_lifecycle",
				MCPClient:   mcpClientName(req),
			},
		},
	)
	if err != nil {
		return nil, nil, err
	}
	result := lifecycleResult{
		SchemaVersion:  1,
		Operation:      "apply",
		Action:         string(challenge.Operation),
		Workspace:      workspace,
		ClaimID:        claim.ID,
		ChallengeID:    challenge.ID,
		ActionSummary:  lifecycleActionSummaryFromChallenge(challenge),
		ActionDigest:   challenge.ActionDigest,
		ExpiresAt:      challenge.ExpiresAt,
		TokenExpiresAt: challenge.TokenExpiresAt,
		Status:         claim.Status,
		VerifiedBy:     claim.VerifiedBy,
		Claim:          &claim,
	}
	return jsonResult(result)
}

func lifecycleAction(action string) (zruntime.ChallengeOperation, error) {
	switch strings.TrimSpace(action) {
	case string(zruntime.ChallengeOperationApprove):
		return zruntime.ChallengeOperationApprove, nil
	case string(zruntime.ChallengeOperationSupersede):
		return zruntime.ChallengeOperationSupersede, nil
	case string(zruntime.ChallengeOperationRevoke):
		return zruntime.ChallengeOperationRevoke, nil
	default:
		return "", invalidLifecycleParams("action must be approve, supersede, or revoke")
	}
}

func supersededPriorVerificationDigest(store zruntime.ClaimStore, workspace string, ids []string) (string, error) {
	if len(ids) == 0 {
		return "", nil
	}
	claim, err := store.Read(workspace, ids[0])
	if err != nil {
		return "", err
	}
	if claim.Status != zruntime.ClaimStatusApproved {
		return "", fmt.Errorf("claim %s is %s; only approved claims can be superseded", claim.ID, claim.Status)
	}
	if err := zruntime.VerifyClaimDigest(claim); err != nil {
		return "", fmt.Errorf("verify superseded claim %s: %w", claim.ID, err)
	}
	return claim.VerifiedDigest, nil
}

func sameLifecycleIDs(left []string, right []string) bool {
	leftCopy := append([]string(nil), left...)
	rightCopy := append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	if len(leftCopy) != len(rightCopy) {
		return false
	}
	for i := range leftCopy {
		if leftCopy[i] != rightCopy[i] {
			return false
		}
	}
	return true
}

func lifecycleActionSummaryFromChallenge(challenge zruntime.Challenge) lifecycleActionSummary {
	supersededIDs := append([]string(nil), challenge.SupersededIDs...)
	if supersededIDs == nil {
		supersededIDs = []string{}
	}
	return lifecycleActionSummary{
		Action:                  string(challenge.Operation),
		Workspace:               challenge.Workspace,
		ClaimID:                 challenge.ClaimID,
		CanonicalDraftDigest:    challenge.CanonicalDraftDigest,
		SupersededIDs:           supersededIDs,
		PriorVerificationDigest: challenge.PriorVerificationDigest,
		RevokeReason:            challenge.RevokeReason,
	}
}

func mcpClientName(req *mcp.CallToolRequest) string {
	if req == nil || req.Session == nil {
		return "unknown"
	}
	params := req.Session.InitializeParams()
	if params == nil || params.ClientInfo == nil {
		return "unknown"
	}
	name := strings.TrimSpace(params.ClientInfo.Name)
	version := strings.TrimSpace(params.ClientInfo.Version)
	if name == "" || version == "" {
		return "unknown"
	}
	return name + "/" + version
}

func invalidLifecycleParams(message string) error {
	return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: message}
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
