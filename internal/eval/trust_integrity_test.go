package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	zcli "github.com/therealtinhtute/zbrain/internal/cli"
	zmcp "github.com/therealtinhtute/zbrain/internal/mcp"
	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

// evalEnv is the shared fixture environment for the eval-suite runners: an
// isolated runtime plus a fixed clock. ZBRAIN_HOME is never touched.
type evalEnv struct {
	paths zruntime.Paths
	now   func() time.Time
}

func evalFixedNow() time.Time {
	return time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
}

func newEvalEnv(t *testing.T) evalEnv {
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
	if _, err := zruntime.EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	if _, err := zruntime.ExtractBundledAssets(paths); err != nil {
		t.Fatalf("ExtractBundledAssets() error = %v", err)
	}
	if err := zruntime.CreateWorkspace(paths, "research", evalFixedNow()); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	return evalEnv{paths: paths, now: evalFixedNow}
}

func evalClaim(id string, title string, body string, conflictsWith string) zruntime.Claim {
	claim := zruntime.Claim{
		Type:      zruntime.OKFClaimType,
		ID:        id,
		Tier:      "projects",
		Status:    zruntime.ClaimStatusDraft,
		Title:     title,
		Basis:     zruntime.ClaimBasisOwner,
		CreatedAt: evalFixedNow().UTC().Format(time.RFC3339),
		CreatedBy: "eval",
		Body:      body,
	}
	if conflictsWith != "" {
		claim.ConflictsWith = []string{conflictsWith}
	}
	return claim
}

func writeEvalClaim(t *testing.T, e evalEnv, id string, title string, body string, conflictsWith string) zruntime.Claim {
	t.Helper()
	store := zruntime.ClaimStore{Paths: e.paths, Now: e.now}
	created, err := store.WriteDraft("research", evalClaim(id, title, body, conflictsWith))
	if err != nil {
		t.Fatalf("WriteDraft(%s) error = %v", id, err)
	}
	approved, err := store.Approve("research", created.ID)
	if err != nil {
		t.Fatalf("Approve(%s) error = %v", id, err)
	}
	return approved
}

func writeEvalDraft(t *testing.T, e evalEnv, id string, title string, body string) zruntime.Claim {
	t.Helper()
	created, err := (zruntime.ClaimStore{Paths: e.paths, Now: e.now}).WriteDraft("research", evalClaim(id, title, body, ""))
	if err != nil {
		t.Fatalf("WriteDraft(%s) error = %v", id, err)
	}
	return created
}

func markDirtyForMutation(t *testing.T, paths zruntime.Paths) {
	t.Helper()
	if err := (zruntime.IndexStore{Paths: paths}).MarkDirty("research"); err != nil {
		t.Fatalf("MarkDirty() error = %v", err)
	}
}

func reindexWorkspace(t *testing.T, paths zruntime.Paths) {
	t.Helper()
	summary, err := (zruntime.IndexStore{Paths: paths}).Rebuild("research")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if summary.RebuildState != zruntime.RebuildStatusClean {
		t.Fatalf("RebuildState = %q, want clean (invalid=%v)", summary.RebuildState, summary.InvalidClaims)
	}
}

// revokeEvalClaim runs the owner-pinned challenge ceremony for a revoke and
// applies it, mirroring the local owner path.
func revokeEvalClaim(t *testing.T, e evalEnv, id string, reason string) {
	t.Helper()
	store := zruntime.ClaimStore{Paths: e.paths, Now: e.now}
	claim, err := store.Read("research", id)
	if err != nil {
		t.Fatalf("Read(%s) error = %v", id, err)
	}
	prepared, err := store.PrepareChallenge("research", zruntime.ChallengePrepare{
		Workspace:               "research",
		Operation:               zruntime.ChallengeOperationRevoke,
		ClaimID:                 id,
		PriorVerificationDigest: claim.VerifiedDigest,
		RevokeReason:            reason,
	})
	if err != nil {
		t.Fatalf("PrepareChallenge(revoke) error = %v", err)
	}
	granted, err := (zruntime.ChallengeStore{Paths: e.paths, Now: e.now}).Grant("research", prepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Grant(revoke) error = %v", err)
	}
	if _, err := store.ApplyChallenge("research", prepared.Challenge.ID, granted.Token, zruntime.ClaimMutationOptions{}); err != nil {
		t.Fatalf("ApplyChallenge(revoke) error = %v", err)
	}
}

// writeEvalProof writes a deterministic JSON artifact under docs/proofs/.
func writeEvalProof(t *testing.T, name string, payload any) {
	t.Helper()
	path := filepath.Join(evalRepoRoot(t), "docs", "proofs", name)
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal proof %s: %v", name, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func evalRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found upward from test directory")
		}
		dir = parent
	}
}

// connectEvalClient mirrors internal/mcp's in-memory client harness over the
// exported eval server hook.
func connectEvalClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	t1, t2 := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "zbrain-eval-client", Version: "0.0.0"}, nil)
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callEvalTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()
	return cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
}

func evalResultText(res *mcp.CallToolResult) string {
	var parts []string
	for _, content := range res.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// TestEvalTrustIntegrity is the eval-suite trust-integrity runner
// (docs/eval/trust-integrity/README.md). It builds one adversarial fixture
// workspace per case and drives every case through BOTH query layers —
// `zbrain ask` via internal/cli and `memory_ask` over the in-memory MCP
// client — asserting the fail-closed contract of trusted-memory-spec.md §9:
// draft, revoked, superseded, conflicting, and digest-invalid content never
// surfaces as a trusted result, and dirty/stale/missing/rejected indexes are
// hard errors. Results land in docs/proofs/eval-trust-integrity.json.
func TestEvalTrustIntegrity(t *testing.T) {
	type askResult struct {
		status  string
		claims  []string
		promo   []string
		errText string
	}
	type askLayer func(t *testing.T, e evalEnv, query string) (askResult, string)

	cases := []struct {
		name  string
		setup func(t *testing.T, e evalEnv)
		query string
		// exactly one expectation is set per case.
		wantReady   bool
		wantGap     bool
		wantBlocked bool
		// wantErr is the fail-closed error substring; empty means the layers
		// must answer with a JSON status instead of an error.
		wantErr string
		// verify optionally asserts case-specific response details per layer.
		verify func(t *testing.T, layer string, res askResult, raw string)
	}{
		{
			name: "draft_alongside_approved",
			setup: func(t *testing.T, e evalEnv) {
				writeEvalClaim(t, e, "clm_a0000000000000000000000000000001", "Orbital Fallback Baseline", "orbital fallback protocol baseline\n", "")
				writeEvalDraft(t, e, "clm_a0000000000000000000000000000002", "Orbital Fallback Draft", "orbital fallback protocol unapproved draft\n")
				reindexWorkspace(t, e.paths)
			},
			query:     "orbital fallback protocol",
			wantReady: true,
			verify: func(t *testing.T, layer string, res askResult, raw string) {
				if len(res.claims) != 1 || res.claims[0] != "clm_a0000000000000000000000000000001" {
					t.Fatalf("%s claims = %v, want only the approved claim", layer, res.claims)
				}
				if len(res.promo) != 1 || res.promo[0] != "clm_a0000000000000000000000000000002" {
					t.Fatalf("%s promotion_candidates = %v, want the draft only", layer, res.promo)
				}
			},
		},
		{
			name: "revoked_claim",
			setup: func(t *testing.T, e evalEnv) {
				claim := writeEvalClaim(t, e, "clm_b0000000000000000000000000000001", "Quiescent Ledger", "quiescent decommission ledger note\n", "")
				markDirtyForMutation(t, e.paths)
				revokeEvalClaim(t, e, claim.ID, "superseded by verified guidance")
				reindexWorkspace(t, e.paths)
			},
			query:   "quiescent decommission ledger",
			wantGap: true,
			verify: func(t *testing.T, layer string, res askResult, raw string) {
				if strings.Contains(raw, "Quiescent Ledger") || strings.Contains(raw, "decommission ledger note") {
					t.Fatalf("%s leaked revoked content: %s", layer, raw)
				}
			},
		},
		{
			name: "superseded_claim",
			setup: func(t *testing.T, e evalEnv) {
				old := writeEvalClaim(t, e, "clm_c0000000000000000000000000000001", "Sync Cadence", "sync cadence guidance hourly heartbeat\n", "")
				markDirtyForMutation(t, e.paths)
				replacement := evalClaim("clm_c0000000000000000000000000000002", "Sync Cadence Replacement", "sync cadence guidance hourly heartbeat replacement\n", "")
				store := zruntime.ClaimStore{Paths: e.paths, Now: e.now}
				if _, err := store.WriteSupersedingDraft("research", old.ID, replacement); err != nil {
					t.Fatalf("WriteSupersedingDraft() error = %v", err)
				}
				if _, err := store.Approve("research", replacement.ID); err != nil {
					t.Fatalf("Approve(replacement) error = %v", err)
				}
				reindexWorkspace(t, e.paths)
			},
			query:     "sync cadence guidance heartbeat",
			wantReady: true,
			verify: func(t *testing.T, layer string, res askResult, raw string) {
				if len(res.claims) != 1 || res.claims[0] != "clm_c0000000000000000000000000000002" {
					t.Fatalf("%s claims = %v, want only the approved replacement", layer, res.claims)
				}
			},
		},
		{
			name: "conflicting_approved",
			setup: func(t *testing.T, e evalEnv) {
				writeEvalClaim(t, e, "clm_d0000000000000000000000000000001", "Retention Window", "retention window verdict thirty days\n", "")
				writeEvalClaim(t, e, "clm_d0000000000000000000000000000002", "Retention Window Rival", "retention window verdict ninety days\n", "clm_d0000000000000000000000000000001")
				reindexWorkspace(t, e.paths)
			},
			query:       "retention window verdict",
			wantBlocked: true,
		},
		{
			name: "digest_tampered_approved",
			setup: func(t *testing.T, e evalEnv) {
				claim := writeEvalClaim(t, e, "clm_e0000000000000000000000000000001", "Tamper Probe", "tamper probe trusted body\n", "")
				reindexWorkspace(t, e.paths)
				claimPath := filepath.Join(e.paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
				contents, err := os.ReadFile(claimPath)
				if err != nil {
					t.Fatalf("ReadFile(canonical claim) error = %v", err)
				}
				digestIndex := strings.Index(string(contents), "sha256:")
				if digestIndex < 0 {
					t.Fatalf("canonical claim has no sha256 digest:\n%s", contents)
				}
				tampered := string(contents[:digestIndex]) + "sha256:" + strings.Repeat("0", 64) + string(contents[digestIndex+len("sha256:")+64:])
				if err := os.WriteFile(claimPath, []byte(tampered), 0o600); err != nil {
					t.Fatalf("WriteFile(tampered digest) error = %v", err)
				}
				summary, err := (zruntime.IndexStore{Paths: e.paths}).Rebuild("research")
				if err != nil {
					t.Fatalf("Rebuild(tampered) error = %v", err)
				}
				if summary.RebuildState != zruntime.RebuildStatusRejected {
					t.Fatalf("RebuildState = %q, want rejected", summary.RebuildState)
				}
			},
			query:   "tamper probe trusted body",
			wantErr: "rejected",
		},
		{
			name: "stale_index",
			setup: func(t *testing.T, e evalEnv) {
				claim := writeEvalClaim(t, e, "clm_f0000000000000000000000000000001", "Stale Probe", "stale probe original body\n", "")
				reindexWorkspace(t, e.paths)
				claimPath := filepath.Join(e.paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
				contents, err := os.ReadFile(claimPath)
				if err != nil {
					t.Fatalf("ReadFile(canonical claim) error = %v", err)
				}
				edited := strings.Replace(string(contents), "stale probe original body", "stale probe edited body", 1)
				if err := os.WriteFile(claimPath, []byte(edited), 0o600); err != nil {
					t.Fatalf("WriteFile(edited claim) error = %v", err)
				}
			},
			query:   "stale probe body",
			wantErr: "stale",
		},
		{
			name: "dirty_index",
			setup: func(t *testing.T, e evalEnv) {
				writeEvalClaim(t, e, "clm_10000000000000000000000000000001", "Dirty Probe", "dirty probe body\n", "")
				reindexWorkspace(t, e.paths)
				if err := (zruntime.IndexStore{Paths: e.paths}).MarkDirty("research"); err != nil {
					t.Fatalf("MarkDirty() error = %v", err)
				}
			},
			query:   "dirty probe body",
			wantErr: "dirty",
		},
		{
			name: "missing_index",
			setup: func(t *testing.T, e evalEnv) {
				writeEvalClaim(t, e, "clm_20000000000000000000000000000001", "Missing Probe", "missing probe body\n", "")
				reindexWorkspace(t, e.paths)
				path, err := (zruntime.IndexStore{Paths: e.paths}).DatabasePath("research")
				if err != nil {
					t.Fatalf("DatabasePath() error = %v", err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatalf("Remove(index database) error = %v", err)
				}
			},
			query:   "missing probe body",
			wantErr: "index",
		},
		{
			name: "unindexed_legacy_doc",
			setup: func(t *testing.T, e evalEnv) {
				writeEvalClaim(t, e, "clm_30000000000000000000000000000001", "Indexed Probe", "indexed probe body\n", "")
				reindexWorkspace(t, e.paths)
				// A canonical claim file that exists on disk but was never part
				// of the published trust-input manifest (legacy-unindexed doc)
				// must fail the next query closed.
				writeEvalDraft(t, e, "clm_30000000000000000000000000000002", "Legacy Unindexed", "legacy unindexed document body\n")
			},
			query:   "legacy unindexed document body",
			wantErr: "dirty",
		},
	}

	askCLI := func(t *testing.T, e evalEnv, query string) (askResult, string) {
		t.Helper()
		app := zcli.App{
			Stdout: &bytes.Buffer{},
			Stderr: &bytes.Buffer{},
			Stdin:  strings.NewReader(""),
			Paths:  e.paths,
			Now:    e.now,
		}
		err := app.Run([]string{"ask", "--workspace", "research", query})
		raw := app.Stdout.(*bytes.Buffer).String()
		res := askResult{}
		if err != nil {
			res.errText = err.Error()
			return res, raw
		}
		var parsed struct {
			Status string `json:"status"`
			Claims []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"claims"`
			PromotionCandidates []struct {
				ID string `json:"id"`
			} `json:"promotion_candidates"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
			t.Fatalf("ask stdout is not JSON: %q", raw)
		}
		res.status = parsed.Status
		for _, claim := range parsed.Claims {
			if claim.Status != string(zruntime.ClaimStatusApproved) {
				t.Fatalf("ask returned non-approved claim %q with status %q", claim.ID, claim.Status)
			}
			res.claims = append(res.claims, claim.ID)
		}
		for _, promo := range parsed.PromotionCandidates {
			res.promo = append(res.promo, promo.ID)
		}
		return res, raw
	}

	askMCP := func(t *testing.T, e evalEnv, query string) (askResult, string) {
		t.Helper()
		server, err := zmcp.NewServerForEval(zmcp.Options{Paths: e.paths, Now: e.now, Version: "eval", Stderr: io.Discard})
		if err != nil {
			t.Fatalf("NewServerForEval() error = %v", err)
		}
		cs := connectEvalClient(t, server)
		res, err := callEvalTool(t, cs, "memory_ask", map[string]any{"workspace": "research", "query": query})
		out := askResult{}
		if err != nil {
			out.errText = err.Error()
			return out, out.errText
		}
		raw := evalResultText(res)
		if res.IsError {
			out.errText = raw
			return out, raw
		}
		var parsed struct {
			Status string `json:"status"`
			Claims []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"claims"`
			PromotionCandidates []struct {
				ID string `json:"id"`
			} `json:"promotion_candidates"`
		}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			t.Fatalf("memory_ask result is not JSON: %q", raw)
		}
		out.status = parsed.Status
		for _, claim := range parsed.Claims {
			if claim.Status != string(zruntime.ClaimStatusApproved) {
				t.Fatalf("memory_ask returned non-approved claim %q with status %q", claim.ID, claim.Status)
			}
			out.claims = append(out.claims, claim.ID)
		}
		for _, promo := range parsed.PromotionCandidates {
			out.promo = append(out.promo, promo.ID)
		}
		return out, raw
	}

	layers := []struct {
		name string
		ask  askLayer
	}{
		{name: "cli:zbrain ask", ask: askCLI},
		{name: "mcp:memory_ask", ask: askMCP},
	}

	records := make([]map[string]any, 0, len(cases)*len(layers))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEvalEnv(t)
			tc.setup(t, e)
			for _, layer := range layers {
				res, raw := layer.ask(t, e, tc.query)
				switch {
				case tc.wantErr != "":
					if res.errText == "" {
						t.Fatalf("%s: expected fail-closed error, got status %q output %q", layer.name, res.status, raw)
					}
					if !strings.Contains(res.errText, tc.wantErr) {
						t.Fatalf("%s: error %q does not contain %q", layer.name, res.errText, tc.wantErr)
					}
					if res.status != "" || len(res.claims) > 0 {
						t.Fatalf("%s: fail-closed error produced trusted status/claims: %#v", layer.name, res)
					}
				case tc.wantGap:
					if res.errText != "" {
						t.Fatalf("%s: unexpected error: %v", layer.name, res.errText)
					}
					if res.status != string(zruntime.QueryStatusGap) {
						t.Fatalf("%s: status = %q, want gap", layer.name, res.status)
					}
					if len(res.claims) > 0 {
						t.Fatalf("%s: gap response carried claims %v", layer.name, res.claims)
					}
				case tc.wantBlocked:
					if res.errText != "" {
						t.Fatalf("%s: unexpected error: %v", layer.name, res.errText)
					}
					if res.status != string(zruntime.QueryStatusBlocked) {
						t.Fatalf("%s: status = %q, want blocked", layer.name, res.status)
					}
				case tc.wantReady:
					if res.errText != "" {
						t.Fatalf("%s: unexpected error: %v", layer.name, res.errText)
					}
					if res.status != string(zruntime.QueryStatusReady) {
						t.Fatalf("%s: status = %q, want ready", layer.name, res.status)
					}
				}
				if tc.verify != nil {
					tc.verify(t, layer.name, res, raw)
				}
				observed := res.status
				if res.errText != "" {
					observed = "error:" + tc.wantErr
				}
				record := map[string]any{
					"case":     tc.name,
					"layer":    layer.name,
					"observed": observed,
				}
				if len(res.claims) > 0 {
					record["trusted_claim_ids"] = res.claims
				}
				if len(res.promo) > 0 {
					record["promotion_candidate_ids"] = res.promo
				}
				records = append(records, record)
				if t.Failed() {
					return
				}
			}
		})
	}

	writeEvalProof(t, "eval-trust-integrity.json", map[string]any{
		"schema":     "zbrain.eval.trust-integrity/v1",
		"case_count": len(cases),
		"layers":     []string{"cli:zbrain ask", "mcp:memory_ask"},
		"cases":      records,
	})
}
