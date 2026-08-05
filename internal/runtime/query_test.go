package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveScopesUsesCurrentAndExplicitIncludes(t *testing.T) {
	paths := queryTestPaths(t)
	if err := CreateWorkspace(paths, "personal", fixedQueryNow()); err != nil {
		t.Fatalf("CreateWorkspace(personal) error = %v", err)
	}
	scopes, err := ResolveQueryScopes(paths, QueryScopeOptions{Includes: []string{"personal", "personal"}})
	if err != nil {
		t.Fatalf("ResolveQueryScopes() error = %v", err)
	}
	if scopes.Primary != "research" {
		t.Fatalf("Primary = %q", scopes.Primary)
	}
	if len(scopes.Includes) != 1 || scopes.Includes[0] != "personal" {
		t.Fatalf("Includes = %v", scopes.Includes)
	}
}

func TestResolveQueryScopesRejectsUnsafeMissingAndSymlinkScopes(t *testing.T) {
	paths := queryTestPaths(t)
	if err := CreateWorkspace(paths, "personal", fixedQueryNow()); err != nil {
		t.Fatalf("CreateWorkspace(personal) error = %v", err)
	}

	for _, options := range []QueryScopeOptions{
		{Workspace: "../outside"},
		{Workspace: "missing"},
		{Includes: []string{""}},
		{Includes: []string{"../outside"}},
		{Includes: []string{"missing"}},
	} {
		if _, err := ResolveQueryScopes(paths, options); err == nil {
			t.Fatalf("ResolveQueryScopes(%#v) error = nil", options)
		}
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(paths.WorkspacesDir, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	for _, options := range []QueryScopeOptions{
		{Workspace: "linked"},
		{Includes: []string{"linked"}},
	} {
		if _, err := ResolveQueryScopes(paths, options); err == nil {
			t.Fatalf("ResolveQueryScopes(symlink %#v) error = nil", options)
		}
	}

	if _, err := TrustedQuery(paths, TrustedQueryOptions{Includes: []string{"missing"}, Query: "anything", Limit: 10}); err == nil {
		t.Fatalf("TrustedQuery(missing include) error = nil")
	}
	if _, err := os.Stat(paths.IndexesDir); !os.IsNotExist(err) {
		t.Fatalf("invalid query scope created indexes directory: stat error = %v", err)
	}
}

func TestTrustedQueryFailsClosedWhenIndexIsStale(t *testing.T) {
	paths := queryTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedQueryNow}
	claim := queryClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Stale Query Claim", ClaimBasisOwner)
	claim.Body = "stale query body\n"
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	contents, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(claim) error = %v", err)
	}
	contents = []byte(strings.Replace(string(contents), "stale query body", "changed stale query body", 1))
	if err := os.WriteFile(claimPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(changed claim) error = %v", err)
	}
	if _, err := TrustedQuery(paths, TrustedQueryOptions{Query: "stale query", Limit: 10}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("TrustedQuery() error = %v, want explicit stale error", err)
	}
}

func TestTrustedQueryBlocksPendingTransition(t *testing.T) {
	paths := queryTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedQueryNow}
	claim := queryClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Pending Query Claim", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	before, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}
	target := append(append([]byte(nil), before...), []byte("pending\n")...)
	if err := WritePendingTransition(paths, "research", PendingTransition{
		OperationID: "txn_query",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets:     []PendingTransitionTarget{pendingTransitionTarget("wiki/projects/"+claim.ID+".md", before, target)},
	}); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}
	if _, err := TrustedQuery(paths, TrustedQueryOptions{Query: "pending query", Limit: 10}); err == nil || !strings.Contains(err.Error(), "pending transition") {
		t.Fatalf("TrustedQuery() error = %v, want pending transition error", err)
	}
	assertFileBytes(t, claimPath, before)
	if _, err := ReadPendingTransition(paths, "research"); err != nil {
		t.Fatalf("ReadPendingTransition() error = %v, want journal preserved", err)
	}
}

func TestTrustedQueryFailsClosedWhenIndexIsRejected(t *testing.T) {
	paths := queryTestPaths(t)
	legacyPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "legacy.md")
	if err := os.WriteFile(legacyPath, []byte("legacy rejected input\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if _, err := TrustedQuery(paths, TrustedQueryOptions{Query: "anything", Limit: 10}); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("TrustedQuery() error = %v, want rejected error", err)
	}
}

func TestUnrelatedValidClaimRejected(t *testing.T) {
	paths := queryTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedQueryNow}
	claim := queryClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Valid Unrelated Claim", ClaimBasisOwner)
	claim.Body = "valid unrelated trusted token\n"
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	legacyPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "unrelated-legacy.md")
	if err := os.WriteFile(legacyPath, []byte("unrelated invalid token\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if _, err := TrustedQuery(paths, TrustedQueryOptions{Query: "valid unrelated", Limit: 10}); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("TrustedQuery() error = %v, want rejected error for unrelated valid claim", err)
	}
}

func TestDependencyInvalidation(t *testing.T) {
	for _, mode := range []string{"revoked", "superseded", "missing"} {
		t.Run(mode, func(t *testing.T) {
			paths := queryTestPaths(t)
			store := ClaimStore{Paths: paths, Now: fixedQueryNow}
			base := queryClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Base Support", ClaimBasisOwner)
			middle := queryClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "Middle Support", ClaimBasisDerived)
			middle.SupportingClaimIDs = []string{base.ID}
			dependent := queryClaim("clm_cccccccccccccccccccccccccccccccc", "Deep Dependent", ClaimBasisDerived)
			dependent.SupportingClaimIDs = []string{middle.ID}
			dependent.Body = "deep dependent trusted token\n"
			unrelated := queryClaim("clm_dddddddddddddddddddddddddddddddd", "Unrelated Trusted", ClaimBasisOwner)
			unrelated.Body = "unrelated trusted token\n"
			for _, claim := range []Claim{base, middle, dependent, unrelated} {
				if _, err := store.WriteDraft("research", claim); err != nil {
					t.Fatalf("WriteDraft(%s) error = %v", claim.ID, err)
				}
				if _, err := store.Approve("research", claim.ID); err != nil {
					t.Fatalf("Approve(%s) error = %v", claim.ID, err)
				}
			}
			idx := IndexStore{Paths: paths}
			if summary, err := idx.Rebuild("research"); err != nil {
				t.Fatalf("initial Rebuild() error = %v", err)
			} else if summary.RebuildState != RebuildStatusClean {
				t.Fatalf("initial rebuild summary = %#v, want clean", summary)
			}
			initial, err := TrustedQuery(paths, TrustedQueryOptions{Query: "deep dependent trusted", Limit: 10})
			if err != nil {
				t.Fatalf("initial TrustedQuery() error = %v", err)
			}
			if initial.Status != QueryStatusReady || len(initial.Claims) != 1 || initial.Claims[0].ID != dependent.ID {
				t.Fatalf("initial query = %#v, want dependent claim", initial)
			}

			dependentPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", dependent.ID+".md")
			middlePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", middle.ID+".md")
			dependentBefore := sha256Hex(t, dependentPath)
			middleBefore := sha256Hex(t, middlePath)
			basePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", base.ID+".md")
			if mode == "revoked" {
				if _, err := store.Revoke("research", base.ID, "support withdrawn"); err != nil {
					t.Fatalf("Revoke(base) error = %v", err)
				}
			} else if mode == "superseded" {
				replacement := queryClaim("clm_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "Replacement Support", ClaimBasisOwner)
				if _, err := store.WriteSupersedingDraft("research", base.ID, replacement); err != nil {
					t.Fatalf("WriteSupersedingDraft(base) error = %v", err)
				}
				if _, err := store.Approve("research", replacement.ID); err != nil {
					t.Fatalf("Approve(replacement) error = %v", err)
				}
			} else {
				backupPath := basePath + ".bak"
				if err := os.Rename(basePath, backupPath); err != nil {
					t.Fatalf("Rename(missing support) error = %v", err)
				}
				defer func() {
					if err := os.Rename(backupPath, basePath); err != nil {
						t.Errorf("Restore(missing support) error = %v", err)
					}
				}()
			}

			summary, err := idx.Rebuild("research")
			if err != nil {
				t.Fatalf("invalidating Rebuild() error = %v", err)
			}
			if summary.RebuildState != RebuildStatusRejected || summary.Invalid != 2 || summary.InvalidCount != 2 {
				t.Fatalf("invalidating summary = %#v, want two dependent roots rejected", summary)
			}
			if len(summary.InvalidClaims) != 2 || summary.InvalidClaims[0].Path != "projects/"+middle.ID+".md" || summary.InvalidClaims[1].Path != "projects/"+dependent.ID+".md" {
				t.Fatalf("InvalidClaims = %#v, want deterministic dependent paths", summary.InvalidClaims)
			}
			for _, invalid := range summary.InvalidClaims {
				if !strings.Contains(invalid.Error, base.ID) {
					t.Fatalf("invalid claim %s error = %q, want base path", invalid.Path, invalid.Error)
				}
				if !strings.Contains(invalid.Error, mode) {
					t.Fatalf("invalid claim %s error = %q, want %s reason", invalid.Path, invalid.Error, mode)
				}
			}
			if _, err := TrustedQuery(paths, TrustedQueryOptions{Query: "unrelated trusted", Limit: 10}); err == nil || !strings.Contains(err.Error(), "rejected") {
				t.Fatalf("TrustedQuery(unrelated) error = %v, want rejected error", err)
			}
			if after := sha256Hex(t, middlePath); after != middleBefore {
				t.Fatalf("middle canonical bytes changed: before %s after %s", middleBefore, after)
			}
			if after := sha256Hex(t, dependentPath); after != dependentBefore {
				t.Fatalf("dependent canonical bytes changed: before %s after %s", dependentBefore, after)
			}
		})
	}
}

func TestEvidenceInvalidationAndRepair(t *testing.T) {
	for _, mode := range []string{"tampered", "missing"} {
		t.Run(mode, func(t *testing.T) {
			paths := queryTestPaths(t)
			evidence := addStoreEvidence(t, paths, "evidence repair bytes")
			store := ClaimStore{Paths: paths, Now: fixedQueryNow}
			claim := queryClaim("clm_ffffffffffffffffffffffffffffffff", "Evidence Repair", ClaimBasisEvidence)
			claim.EvidenceIDs = []string{evidence.ID}
			claim.Body = "evidence repair trusted token\n"
			if _, err := store.WriteDraft("research", claim); err != nil {
				t.Fatalf("WriteDraft() error = %v", err)
			}
			if _, err := store.Approve("research", claim.ID); err != nil {
				t.Fatalf("Approve() error = %v", err)
			}
			idx := IndexStore{Paths: paths}
			if summary, err := idx.Rebuild("research"); err != nil {
				t.Fatalf("initial Rebuild() error = %v", err)
			} else if summary.RebuildState != RebuildStatusClean {
				t.Fatalf("initial rebuild summary = %#v, want clean", summary)
			}
			initial, err := TrustedQuery(paths, TrustedQueryOptions{Query: "evidence repair trusted", Limit: 10})
			if err != nil {
				t.Fatalf("initial TrustedQuery() error = %v", err)
			}
			if initial.Status != QueryStatusReady || len(initial.Claims) != 1 || initial.Claims[0].ID != claim.ID {
				t.Fatalf("initial query = %#v, want evidence claim", initial)
			}

			claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
			claimBefore := sha256Hex(t, claimPath)
			rawPath := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
			originalRaw, err := os.ReadFile(rawPath)
			if err != nil {
				t.Fatalf("ReadFile(raw) error = %v", err)
			}
			backupPath := rawPath + ".bak"
			if mode == "tampered" {
				if err := os.Chmod(rawPath, 0o644); err != nil {
					t.Fatalf("Chmod(raw) error = %v", err)
				}
				if err := os.WriteFile(rawPath, []byte(strings.Repeat("x", len(originalRaw))), 0o644); err != nil {
					t.Fatalf("WriteFile(tampered raw) error = %v", err)
				}
			} else if err := os.Rename(rawPath, backupPath); err != nil {
				t.Fatalf("Rename(missing raw) error = %v", err)
			}

			summary, err := idx.Rebuild("research")
			if err != nil {
				t.Fatalf("invalidating Rebuild() error = %v", err)
			}
			if summary.RebuildState != RebuildStatusRejected || summary.Invalid != 1 || summary.InvalidCount != 1 || len(summary.InvalidClaims) != 1 {
				t.Fatalf("invalidating summary = %#v, want one evidence root rejected", summary)
			}
			if !strings.Contains(summary.InvalidClaims[0].Error, evidence.ID) || !strings.Contains(summary.InvalidClaims[0].Error, "raw") {
				t.Fatalf("invalid claim = %#v, want evidence ID/raw reason", summary.InvalidClaims[0])
			}
			if _, err := TrustedQuery(paths, TrustedQueryOptions{Query: "evidence repair trusted", Limit: 10}); err == nil || !strings.Contains(err.Error(), "rejected") {
				t.Fatalf("TrustedQuery() error = %v, want rejected error", err)
			}

			if mode == "missing" {
				if err := os.Rename(backupPath, rawPath); err != nil {
					t.Fatalf("Restore(raw) error = %v", err)
				}
			} else if err := os.WriteFile(rawPath, originalRaw, 0o644); err != nil {
				t.Fatalf("Restore(tampered raw) error = %v", err)
			}
			repaired, err := idx.Rebuild("research")
			if err != nil {
				t.Fatalf("repair Rebuild() error = %v", err)
			}
			if repaired.RebuildState != RebuildStatusClean || repaired.Approved != 1 || repaired.InvalidCount != 0 {
				t.Fatalf("repair summary = %#v, want clean evidence index", repaired)
			}
			response, err := TrustedQuery(paths, TrustedQueryOptions{Query: "evidence repair trusted", Limit: 10})
			if err != nil {
				t.Fatalf("repaired TrustedQuery() error = %v", err)
			}
			if response.Status != QueryStatusReady || len(response.Claims) != 1 || response.Claims[0].ID != claim.ID {
				t.Fatalf("repaired query = %#v, want evidence claim", response)
			}
			if after := sha256Hex(t, claimPath); after != claimBefore {
				t.Fatalf("claim canonical bytes changed: before %s after %s", claimBefore, after)
			}
		})
	}
}

func TestTrustedQueryFailsClosedWhenIndexIsDirty(t *testing.T) {
	paths := queryTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if err := idx.MarkDirty("research"); err != nil {
		t.Fatalf("MarkDirty() error = %v", err)
	}
	if _, err := TrustedQuery(paths, TrustedQueryOptions{Query: "anything", Limit: 10}); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("TrustedQuery() error = %v, want dirty error", err)
	}
}

func TestTrustedQueryFailsClosedWhenIndexIsMissing(t *testing.T) {
	paths := queryTestPaths(t)
	if _, err := TrustedQuery(paths, TrustedQueryOptions{Query: "anything", Limit: 10}); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("TrustedQuery() error = %v, want missing error", err)
	}
}

func TestTrustedQueryReturnsApprovedClaimsAndPromotionCandidatesSeparately(t *testing.T) {
	paths := queryTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedQueryNow}
	approved := queryClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Approved Retrieval", ClaimBasisOwner)
	approved.Body = "local durable memory answer\n"
	if _, err := store.WriteDraft("research", approved); err != nil {
		t.Fatalf("WriteDraft(approved) error = %v", err)
	}
	if _, err := store.Approve("research", approved.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	draft := queryClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "Draft Retrieval", ClaimBasisOwner)
	draft.Body = "local durable memory draft candidate\n"
	if _, err := store.WriteDraft("research", draft); err != nil {
		t.Fatalf("WriteDraft(draft) error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	response, err := TrustedQuery(paths, TrustedQueryOptions{Query: "local durable", Limit: 10})
	if err != nil {
		t.Fatalf("TrustedQuery() error = %v", err)
	}
	if response.Status != QueryStatusReady {
		t.Fatalf("Status = %q", response.Status)
	}
	if len(response.Claims) != 1 || response.Claims[0].ID != approved.ID || response.Claims[0].Type != OKFClaimType {
		t.Fatalf("Claims = %#v", response.Claims)
	}
	if len(response.PromotionCandidates) != 1 || response.PromotionCandidates[0].ID != draft.ID {
		t.Fatalf("PromotionCandidates = %#v", response.PromotionCandidates)
	}
	if response.Claims[0].Status != ClaimStatusApproved || response.PromotionCandidates[0].Status != ClaimStatusDraft {
		t.Fatalf("draft leaked into trusted claims: %#v", response)
	}
}

func TestTrustedQueryReportsGapWhenNoApprovedClaimsMatch(t *testing.T) {
	paths := queryTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	response, err := TrustedQuery(paths, TrustedQueryOptions{Query: "missing context", Limit: 10})
	if err != nil {
		t.Fatalf("TrustedQuery() error = %v", err)
	}
	if response.Status != QueryStatusGap {
		t.Fatalf("Status = %q", response.Status)
	}
	if len(response.Gaps) != 1 {
		t.Fatalf("Gaps = %#v", response.Gaps)
	}
}

func TestTrustedQueryBlocksExplicitConflict(t *testing.T) {
	paths := queryTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedQueryNow}
	first := queryClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Conflict A", ClaimBasisOwner)
	first.Body = "conflict token shared memory\n"
	second := queryClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "Conflict B", ClaimBasisOwner)
	second.Body = "conflict token shared memory\n"
	second.ConflictsWith = []string{first.ID}
	if _, err := store.WriteDraft("research", first); err != nil {
		t.Fatalf("WriteDraft(first) error = %v", err)
	}
	if _, err := store.Approve("research", first.ID); err != nil {
		t.Fatalf("Approve(first) error = %v", err)
	}
	if _, err := store.WriteDraft("research", second); err != nil {
		t.Fatalf("WriteDraft(second) error = %v", err)
	}
	if _, err := store.Approve("research", second.ID); err != nil {
		t.Fatalf("Approve(second) error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	response, err := TrustedQuery(paths, TrustedQueryOptions{Query: "conflict shared", Limit: 10})
	if err != nil {
		t.Fatalf("TrustedQuery() error = %v", err)
	}
	if response.Status != QueryStatusBlocked {
		t.Fatalf("Status = %q", response.Status)
	}
	if len(response.Conflicts) != 1 {
		t.Fatalf("Conflicts = %#v", response.Conflicts)
	}
}

func TestTrustedQueryDoesNotUseOmittedWorkspace(t *testing.T) {
	paths := queryTestPaths(t)
	if err := CreateWorkspace(paths, "personal", fixedQueryNow()); err != nil {
		t.Fatalf("CreateWorkspace(personal) error = %v", err)
	}
	store := ClaimStore{Paths: paths, Now: fixedQueryNow}
	personalClaim := queryClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Personal Only", ClaimBasisOwner)
	personalClaim.Body = "private omitted token\n"
	if _, err := store.WriteDraft("personal", personalClaim); err != nil {
		t.Fatalf("WriteDraft(personal) error = %v", err)
	}
	if _, err := store.Approve("personal", personalClaim.ID); err != nil {
		t.Fatalf("Approve(personal) error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild(research) error = %v", err)
	}
	if _, err := idx.Rebuild("personal"); err != nil {
		t.Fatalf("Rebuild(personal) error = %v", err)
	}
	withoutInclude, err := TrustedQuery(paths, TrustedQueryOptions{Query: "private omitted", Limit: 10})
	if err != nil {
		t.Fatalf("TrustedQuery(no include) error = %v", err)
	}
	if withoutInclude.Status != QueryStatusGap {
		t.Fatalf("Status without include = %q", withoutInclude.Status)
	}
	withInclude, err := TrustedQuery(paths, TrustedQueryOptions{Query: "private omitted", Includes: []string{"personal"}, Limit: 10})
	if err != nil {
		t.Fatalf("TrustedQuery(include) error = %v", err)
	}
	if withInclude.Status != QueryStatusReady || len(withInclude.Claims) != 1 || withInclude.Claims[0].Workspace != "personal" {
		t.Fatalf("withInclude = %#v", withInclude)
	}
}

func queryTestPaths(t *testing.T) Paths {
	t.Helper()
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: filepath.Join(tmp, "project"), HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if _, err := EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", fixedQueryNow()); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	return paths
}

func queryClaim(id string, title string, basis ClaimBasis) Claim {
	return Claim{
		Type:      OKFClaimType,
		ID:        id,
		Tier:      "projects",
		Status:    ClaimStatusDraft,
		Title:     title,
		Basis:     basis,
		CreatedAt: fixedQueryNow().Format(time.RFC3339),
		CreatedBy: "owner",
		Tags:      []string{"memory"},
		Body:      "query body\n",
	}
}

func fixedQueryNow() time.Time {
	return time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
}
