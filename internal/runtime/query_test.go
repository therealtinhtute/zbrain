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
