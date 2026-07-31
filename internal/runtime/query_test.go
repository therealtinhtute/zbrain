package runtime

import (
	"path/filepath"
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
