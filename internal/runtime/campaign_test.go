package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func campaignTestPaths(t *testing.T) (Paths, CampaignStore, time.Time) {
	t.Helper()
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: filepath.Join(tmp, "project"), HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if _, err := EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	if err := CreateWorkspace(paths, "research", now); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	return paths, CampaignStore{Paths: paths, Now: func() time.Time { return now }}, now
}

func campaignSpecs() []CampaignSpec {
	return []CampaignSpec{
		{Tier: "projects", Title: "Campaign One", Basis: "owner"},
		{Tier: "projects", Title: "Campaign Two", Basis: "evidence", EvidenceIDs: []string{"evd_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
	}
}

func campaignRunFilePath(paths Paths, runID string) string {
	return filepath.Join(paths.WorkspacesDir, "research", "campaigns", runID+".json")
}

func requireDraftExists(t *testing.T, paths Paths, submission CampaignSubmission) {
	t.Helper()
	claim, err := (ClaimStore{Paths: paths}).Read("research", submission.ClaimID)
	if err != nil {
		t.Fatalf("Read(submitted claim %s) error = %v", submission.ClaimID, err)
	}
	if claim.Status != ClaimStatusDraft {
		t.Fatalf("submitted claim status = %s, want draft", claim.Status)
	}
	if claim.VerifiedAt != "" || claim.VerifiedBy != "" || claim.VerifiedDigest != "" {
		t.Fatalf("submitted claim carries verification material: %#v", claim)
	}
	if len(claim.Transitions) != 0 {
		t.Fatalf("submitted claim carries transitions: %#v", claim.Transitions)
	}
	if claim.Path != submission.ClaimPath {
		t.Fatalf("submission path %q does not match claim path %q", submission.ClaimPath, claim.Path)
	}
}

func readRunFromDisk(t *testing.T, paths Paths, runID string) CampaignRun {
	t.Helper()
	contents, err := os.ReadFile(campaignRunFilePath(paths, runID))
	if err != nil {
		t.Fatalf("ReadFile(run file) error = %v", err)
	}
	var run CampaignRun
	if err := json.Unmarshal(contents, &run); err != nil {
		t.Fatalf("decode run file: %v", err)
	}
	return run
}

func TestCampaignBeginPersistsResumableRunFile(t *testing.T) {
	paths, store, now := campaignTestPaths(t)
	run, err := store.BeginCampaign("research", campaignSpecs())
	if err != nil {
		t.Fatalf("BeginCampaign() error = %v", err)
	}
	if !campaignRunIDPattern.MatchString(run.RunID) {
		t.Fatalf("run id %q must match cmp_<32 hex>", run.RunID)
	}
	if run.Phase != CampaignPhaseDrafting {
		t.Fatalf("phase = %q, want drafting", run.Phase)
	}
	if len(run.Drafts) != 2 {
		t.Fatalf("drafts = %d, want 2", len(run.Drafts))
	}
	for index, draft := range run.Drafts {
		if draft.Status != CampaignDraftStatusPending {
			t.Fatalf("draft %d status = %q, want pending", index, draft.Status)
		}
		if draft.ClaimID != "" || draft.SubmittedAt != "" {
			t.Fatalf("draft %d carries submission metadata: %#v", index, draft)
		}
	}
	if run.CreatedAt != now.UTC().Format(time.RFC3339) || run.UpdatedAt != run.CreatedAt {
		t.Fatalf("timestamps = %q/%q, want fixed now", run.CreatedAt, run.UpdatedAt)
	}

	info, err := os.Stat(campaignRunFilePath(paths, run.RunID))
	if err != nil {
		t.Fatalf("Stat(run file) error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("run file mode = %o, want 600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Join(paths.WorkspacesDir, "research", "campaigns"))
	if err != nil {
		t.Fatalf("Stat(campaigns dir) error = %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("campaigns dir mode = %o, want 700", dirInfo.Mode().Perm())
	}
	persisted := readRunFromDisk(t, paths, run.RunID)
	if persisted.RunID != run.RunID || persisted.Schema != CampaignSchemaVersion {
		t.Fatalf("persisted run = %#v", persisted)
	}
}

func TestCampaignBeginValidatesSpecsWithClaimDraftSemantics(t *testing.T) {
	paths, store, _ := campaignTestPaths(t)
	invalid := []CampaignSpec{
		{Tier: "not-a-tier", Title: "Bad Tier", Basis: "owner"},
		{Tier: "projects", Title: "Bad Basis", Basis: "guessed"},
		{Tier: "projects", Title: "Bad Evidence", Basis: "evidence", EvidenceIDs: []string{"evd_ZZZ"}},
		{Tier: "projects", Title: "Bad Support", Basis: "derived", SupportingClaimIDs: []string{"clm_ZZZ"}},
		{Tier: "projects", Title: "", Basis: "owner"},
	}
	for index, spec := range invalid {
		if _, err := store.BeginCampaign("research", []CampaignSpec{spec}); err == nil {
			t.Fatalf("BeginCampaign(invalid spec %d) error = nil", index)
		}
	}
	if _, err := store.BeginCampaign("research", nil); err == nil {
		t.Fatal("BeginCampaign(empty specs) error = nil")
	}
	if _, err := store.BeginCampaign("missing", campaignSpecs()); err == nil {
		t.Fatal("BeginCampaign(missing workspace) error = nil")
	}
	entries, err := os.ReadDir(filepath.Join(paths.WorkspacesDir, "research", "campaigns"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadDir(campaigns) error = %v", err)
	}
	if err == nil && len(entries) != 0 {
		t.Fatalf("failed BeginCampaign wrote %d run files", len(entries))
	}
}

func TestCampaignNextIsDeterministicAndPure(t *testing.T) {
	paths, store, _ := campaignTestPaths(t)
	run, err := store.BeginCampaign("research", campaignSpecs())
	if err != nil {
		t.Fatalf("BeginCampaign() error = %v", err)
	}
	before, err := os.ReadFile(campaignRunFilePath(paths, run.RunID))
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}
	for round := 0; round < 2; round++ {
		index, spec, err := store.NextCampaignDraft("research", run.RunID)
		if err != nil {
			t.Fatalf("NextCampaignDraft(round %d) error = %v", round, err)
		}
		if index != 0 || spec.Title != "Campaign One" {
			t.Fatalf("NextCampaignDraft(round %d) = (%d, %#v)", round, index, spec)
		}
	}
	after, err := os.ReadFile(campaignRunFilePath(paths, run.RunID))
	if err != nil {
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("NextCampaignDraft mutated the run file")
	}

	submission, err := store.SubmitCampaignDraft("research", run.RunID, 0, "first body\n")
	if err != nil {
		t.Fatalf("SubmitCampaignDraft() error = %v", err)
	}
	requireDraftExists(t, paths, submission)
	index, spec, err := store.NextCampaignDraft("research", run.RunID)
	if err != nil {
		t.Fatalf("NextCampaignDraft(after submit) error = %v", err)
	}
	if index != 1 || spec.Title != "Campaign Two" {
		t.Fatalf("NextCampaignDraft(after submit) = (%d, %#v), want (1, Campaign Two)", index, spec)
	}
}

func TestCampaignSubmitCreatesDraftThroughClaimPath(t *testing.T) {
	paths, store, _ := campaignTestPaths(t)
	run, err := store.BeginCampaign("research", campaignSpecs())
	if err != nil {
		t.Fatalf("BeginCampaign() error = %v", err)
	}
	if _, err := ensureWorkspaceGenerationUnlocked(paths, "research"); err != nil {
		t.Fatalf("ensureWorkspaceGenerationUnlocked(before) error = %v", err)
	}
	generationBefore, err := readWorkspaceGeneration(paths, "research")
	if err != nil {
		t.Fatalf("readWorkspaceGeneration(before) error = %v", err)
	}
	submission, err := store.SubmitCampaignDraft("research", run.RunID, 0, "campaign body\n")
	if err != nil {
		t.Fatalf("SubmitCampaignDraft() error = %v", err)
	}
	if submission.ClaimStatus != ClaimStatusDraft {
		t.Fatalf("submission status = %s, want draft", submission.ClaimStatus)
	}
	requireDraftExists(t, paths, submission)
	generationAfter, err := readWorkspaceGeneration(paths, "research")
	if err != nil {
		t.Fatalf("readWorkspaceGeneration(after) error = %v", err)
	}
	if generationAfter.Published != generationBefore.Published {
		t.Fatalf("campaign submission published generation %d, want unchanged %d", generationAfter.Published, generationBefore.Published)
	}
	if generationAfter.Current <= generationBefore.Current {
		t.Fatalf("campaign submission did not advance the canonical generation: %d -> %d", generationBefore.Current, generationAfter.Current)
	}
	if _, err := os.Stat((IndexStore{Paths: paths}).dirtyPath("research")); err != nil {
		t.Fatalf("campaign submission did not mark the derived index dirty: %v", err)
	}

	runOnDisk := readRunFromDisk(t, paths, run.RunID)
	if runOnDisk.Drafts[0].Status != CampaignDraftStatusSubmitted {
		t.Fatalf("run draft 0 status = %q, want submitted", runOnDisk.Drafts[0].Status)
	}
	if runOnDisk.Drafts[0].ClaimID != submission.ClaimID {
		t.Fatalf("run draft 0 claim id = %q, want %q", runOnDisk.Drafts[0].ClaimID, submission.ClaimID)
	}
	if _, err := time.Parse(time.RFC3339, runOnDisk.Drafts[0].SubmittedAt); err != nil {
		t.Fatalf("run draft 0 submitted_at = %q: %v", runOnDisk.Drafts[0].SubmittedAt, err)
	}
	if runOnDisk.Drafts[1].Status != CampaignDraftStatusPending {
		t.Fatalf("run draft 1 status = %q, want pending", runOnDisk.Drafts[1].Status)
	}
}

func TestCampaignSubmitFailsClosed(t *testing.T) {
	paths, store, _ := campaignTestPaths(t)
	run, err := store.BeginCampaign("research", campaignSpecs())
	if err != nil {
		t.Fatalf("BeginCampaign() error = %v", err)
	}
	if _, err := store.SubmitCampaignDraft("research", run.RunID, 0, "body\n"); err != nil {
		t.Fatalf("SubmitCampaignDraft() error = %v", err)
	}
	before, err := os.ReadFile(campaignRunFilePath(paths, run.RunID))
	if err != nil {
		t.Fatalf("ReadFile(run file) error = %v", err)
	}

	if _, err := store.SubmitCampaignDraft("research", run.RunID, 0, "again\n"); err == nil {
		t.Fatal("SubmitCampaignDraft(already submitted) error = nil")
	}
	if _, err := store.SubmitCampaignDraft("research", run.RunID, -1, "body\n"); err == nil {
		t.Fatal("SubmitCampaignDraft(negative index) error = nil")
	}
	if _, err := store.SubmitCampaignDraft("research", run.RunID, 2, "body\n"); err == nil {
		t.Fatal("SubmitCampaignDraft(out-of-range index) error = nil")
	}
	if _, err := store.SubmitCampaignDraft("research", "cmp_00000000000000000000000000000000", 0, "body\n"); err == nil {
		t.Fatal("SubmitCampaignDraft(unknown run) error = nil")
	}
	if _, err := store.SubmitCampaignDraft("research", "not-a-run-id", 0, "body\n"); err == nil {
		t.Fatal("SubmitCampaignDraft(malformed run id) error = nil")
	}
	after, err := os.ReadFile(campaignRunFilePath(paths, run.RunID))
	if err != nil {
		t.Fatalf("ReadFile(run file after failures) error = %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("failed submissions mutated the run file")
	}
	claims, err := (ClaimStore{Paths: paths}).ScanWorkspace("research")
	if err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}
	draftCount := 0
	for _, claim := range claims.Claims {
		if claim.Status == ClaimStatusDraft {
			draftCount++
		}
	}
	if draftCount != 1 {
		t.Fatalf("failed submissions created %d drafts, want 1", draftCount)
	}
}

func TestCampaignResumeCounts(t *testing.T) {
	_, store, _ := campaignTestPaths(t)
	run, err := store.BeginCampaign("research", campaignSpecs())
	if err != nil {
		t.Fatalf("BeginCampaign() error = %v", err)
	}
	state, err := store.ResumeCampaign("research", run.RunID)
	if err != nil {
		t.Fatalf("ResumeCampaign() error = %v", err)
	}
	if state.Pending != 2 || state.Submitted != 0 || state.NextIndex != 0 {
		t.Fatalf("initial state = %#v", state)
	}
	if _, err := store.SubmitCampaignDraft("research", run.RunID, 1, "second body\n"); err != nil {
		t.Fatalf("SubmitCampaignDraft(1) error = %v", err)
	}
	state, err = store.ResumeCampaign("research", run.RunID)
	if err != nil {
		t.Fatalf("ResumeCampaign(after submit) error = %v", err)
	}
	if state.Pending != 1 || state.Submitted != 1 || state.NextIndex != 0 {
		t.Fatalf("state after submit = %#v", state)
	}
	if _, err := store.ResumeCampaign("research", "cmp_00000000000000000000000000000000"); err == nil {
		t.Fatal("ResumeCampaign(unknown run) error = nil")
	}
}

func TestCampaignFinishRequiresNoPending(t *testing.T) {
	_, store, _ := campaignTestPaths(t)
	run, err := store.BeginCampaign("research", campaignSpecs())
	if err != nil {
		t.Fatalf("BeginCampaign() error = %v", err)
	}
	if _, err := store.FinishCampaign("research", run.RunID); err == nil {
		t.Fatal("FinishCampaign(with pending) error = nil")
	}
	for _, index := range []int{0, 1} {
		if _, err := store.SubmitCampaignDraft("research", run.RunID, index, "body\n"); err != nil {
			t.Fatalf("SubmitCampaignDraft(%d) error = %v", index, err)
		}
	}
	finished, err := store.FinishCampaign("research", run.RunID)
	if err != nil {
		t.Fatalf("FinishCampaign() error = %v", err)
	}
	if finished.Phase != CampaignPhaseFinished {
		t.Fatalf("phase = %q, want finished", finished.Phase)
	}
	if _, err := store.SubmitCampaignDraft("research", run.RunID, 0, "late body\n"); err == nil {
		t.Fatal("SubmitCampaignDraft(finished run) error = nil")
	}
	state, err := store.ResumeCampaign("research", run.RunID)
	if err != nil {
		t.Fatalf("ResumeCampaign(finished) error = %v", err)
	}
	if state.Pending != 0 || state.Submitted != 2 || state.NextIndex != -1 {
		t.Fatalf("finished state = %#v", state)
	}
}

func TestCampaignMalformedRunFileFailsClosed(t *testing.T) {
	paths, store, _ := campaignTestPaths(t)
	run, err := store.BeginCampaign("research", campaignSpecs())
	if err != nil {
		t.Fatalf("BeginCampaign() error = %v", err)
	}
	path := campaignRunFilePath(paths, run.RunID)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(original) error = %v", err)
	}

	corruptions := map[string][]byte{
		"bad json":           []byte("{not json"),
		"wrong schema":       mustJSON(t, CampaignRun{Schema: "zbrain.campaign/v0", RunID: run.RunID, Phase: CampaignPhaseDrafting, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt, Drafts: run.Drafts}),
		"bad phase":          mustJSON(t, CampaignRun{Schema: CampaignSchemaVersion, RunID: run.RunID, Phase: "paused", CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt, Drafts: run.Drafts}),
		"bad draft status":   mustJSON(t, CampaignRun{Schema: CampaignSchemaVersion, RunID: run.RunID, Phase: CampaignPhaseDrafting, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt, Drafts: []CampaignDraft{{Spec: run.Drafts[0].Spec, Status: "approved"}}}),
		"pending with claim": mustJSON(t, CampaignRun{Schema: CampaignSchemaVersion, RunID: run.RunID, Phase: CampaignPhaseDrafting, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt, Drafts: []CampaignDraft{{Spec: run.Drafts[0].Spec, Status: CampaignDraftStatusPending, ClaimID: "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}),
		"empty drafts":       mustJSON(t, CampaignRun{Schema: CampaignSchemaVersion, RunID: run.RunID, Phase: CampaignPhaseDrafting, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt, Drafts: nil}),
	}
	for name, contents := range corruptions {
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
		if _, err := store.ResumeCampaign("research", run.RunID); err == nil {
			t.Fatalf("ResumeCampaign(%s) error = nil", name)
		}
		if _, _, err := store.NextCampaignDraft("research", run.RunID); err == nil {
			t.Fatalf("NextCampaignDraft(%s) error = nil", name)
		}
		if _, err := store.SubmitCampaignDraft("research", run.RunID, 0, "body\n"); err == nil {
			t.Fatalf("SubmitCampaignDraft(%s) error = nil", name)
		}
		if _, err := store.FinishCampaign("research", run.RunID); err == nil {
			t.Fatalf("FinishCampaign(%s) error = nil", name)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", name, err)
		}
		if string(after) != string(contents) {
			t.Fatalf("%s: run file was mutated or reset", name)
		}
	}

	// A submitted draft without a claim id is malformed, and duplicate claim
	// ids across drafts are malformed.
	submittedNoClaim := CampaignRun{
		Schema: CampaignSchemaVersion, RunID: run.RunID, Phase: CampaignPhaseDrafting,
		CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
		Drafts: []CampaignDraft{{Spec: run.Drafts[0].Spec, Status: CampaignDraftStatusSubmitted, SubmittedAt: run.CreatedAt}},
	}
	if err := os.WriteFile(path, mustJSON(t, submittedNoClaim), 0o600); err != nil {
		t.Fatalf("WriteFile(submitted without claim) error = %v", err)
	}
	if _, err := store.ResumeCampaign("research", run.RunID); err == nil {
		t.Fatal("ResumeCampaign(submitted without claim) error = nil")
	}
	duplicateClaims := CampaignRun{
		Schema: CampaignSchemaVersion, RunID: run.RunID, Phase: CampaignPhaseDrafting,
		CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
		Drafts: []CampaignDraft{
			{Spec: run.Drafts[0].Spec, Status: CampaignDraftStatusSubmitted, ClaimID: "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SubmittedAt: run.CreatedAt},
			{Spec: run.Drafts[1].Spec, Status: CampaignDraftStatusSubmitted, ClaimID: "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SubmittedAt: run.CreatedAt},
		},
	}
	if err := os.WriteFile(path, mustJSON(t, duplicateClaims), 0o600); err != nil {
		t.Fatalf("WriteFile(duplicate claims) error = %v", err)
	}
	if _, err := store.ResumeCampaign("research", run.RunID); err == nil {
		t.Fatal("ResumeCampaign(duplicate claims) error = nil")
	}
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile(restore) error = %v", err)
	}
	if _, err := store.ResumeCampaign("research", run.RunID); err != nil {
		t.Fatalf("ResumeCampaign(restored) error = %v", err)
	}
}

func TestCampaignCannotApprove(t *testing.T) {
	paths, store, now := campaignTestPaths(t)
	claimStore := ClaimStore{Paths: paths, Now: func() time.Time { return now }}
	approvedID, err := NewClaimID()
	if err != nil {
		t.Fatalf("NewClaimID() error = %v", err)
	}
	if err := (IndexStore{Paths: paths}).MarkDirty("research"); err != nil {
		t.Fatalf("MarkDirty() error = %v", err)
	}
	approved, err := claimStore.WriteDraft("research", Claim{
		Type: OKFClaimType, ID: approvedID, Tier: "projects", Status: ClaimStatusDraft,
		Title: "Approved Baseline", Basis: ClaimBasisOwner,
		CreatedAt: now.UTC().Format(time.RFC3339), CreatedBy: "owner", Body: "baseline body\n",
	})
	if err != nil {
		t.Fatalf("WriteDraft(baseline) error = %v", err)
	}
	approved, err = claimStore.Approve("research", approved.ID)
	if err != nil {
		t.Fatalf("Approve(baseline) error = %v", err)
	}
	if approved.Status != ClaimStatusApproved {
		t.Fatalf("baseline status = %s, want approved", approved.Status)
	}
	if _, err := ensureWorkspaceGenerationUnlocked(paths, "research"); err != nil {
		t.Fatalf("ensureWorkspaceGenerationUnlocked(before) error = %v", err)
	}
	generationBefore, err := readWorkspaceGeneration(paths, "research")
	if err != nil {
		t.Fatalf("readWorkspaceGeneration(before) error = %v", err)
	}

	specs := []CampaignSpec{
		{Tier: "projects", Title: "Adversarial Owner", Basis: "owner"},
		{Tier: "projects", Title: "Adversarial Conflicts", Basis: "owner", ConflictsWith: []string{approved.ID}},
		{Tier: "projects", Title: "Adversarial Support", Basis: "derived", SupportingClaimIDs: []string{approved.ID}},
	}
	run, err := store.BeginCampaign("research", specs)
	if err != nil {
		t.Fatalf("BeginCampaign() error = %v", err)
	}
	if _, _, err := store.NextCampaignDraft("research", run.RunID); err != nil {
		t.Fatalf("NextCampaignDraft() error = %v", err)
	}
	for index := range specs {
		if _, err := store.SubmitCampaignDraft("research", run.RunID, index, "adversarial body\n"); err != nil {
			t.Fatalf("SubmitCampaignDraft(%d) error = %v", index, err)
		}
	}
	if _, err := store.ResumeCampaign("research", run.RunID); err != nil {
		t.Fatalf("ResumeCampaign() error = %v", err)
	}
	if _, err := store.FinishCampaign("research", run.RunID); err != nil {
		t.Fatalf("FinishCampaign() error = %v", err)
	}
	// Every further campaign call on the finished run must fail closed.
	if _, _, err := store.NextCampaignDraft("research", run.RunID); err == nil {
		t.Fatal("NextCampaignDraft(finished) error = nil")
	}
	if _, err := store.SubmitCampaignDraft("research", run.RunID, 0, "again\n"); err == nil {
		t.Fatal("SubmitCampaignDraft(finished) error = nil")
	}

	scan, err := claimStore.ScanWorkspace("research")
	if err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}
	approvedCount := 0
	for _, claim := range scan.Claims {
		if claim.Status == ClaimStatusApproved {
			approvedCount++
			if claim.ID != approved.ID {
				t.Fatalf("campaign produced approved claim %s", claim.ID)
			}
			if claim.VerifiedBy != approved.VerifiedBy || claim.VerifiedDigest != approved.VerifiedDigest {
				t.Fatalf("approved claim verification material changed: %#v", claim)
			}
			if err := VerifyClaimDigest(claim); err != nil {
				t.Fatalf("approved claim digest broke: %v", err)
			}
			if len(claim.Transitions) != 1 || claim.Transitions[0].Kind != ClaimTransitionApprove {
				t.Fatalf("approved claim transitions changed: %#v", claim.Transitions)
			}
		} else if claim.Status != ClaimStatusDraft {
			t.Fatalf("campaign claim %s has status %s, want draft", claim.ID, claim.Status)
		}
	}
	if approvedCount != 1 {
		t.Fatalf("approved claim count = %d, want 1", approvedCount)
	}
	if len(scan.Claims) != 4 {
		t.Fatalf("workspace claim count = %d, want 4 (1 approved + 3 campaign drafts)", len(scan.Claims))
	}
	generationAfter, err := readWorkspaceGeneration(paths, "research")
	if err != nil {
		t.Fatalf("readWorkspaceGeneration(after) error = %v", err)
	}
	if generationAfter.Published != generationBefore.Published {
		t.Fatalf("campaign published generation %d, want unchanged %d", generationAfter.Published, generationBefore.Published)
	}

	// The campaigns directory is runtime metadata outside the wiki trust
	// boundary and must never enter the claim scan.
	for _, claim := range scan.Claims {
		if strings.HasPrefix(filepath.ToSlash(claim.Path), "campaigns/") {
			t.Fatalf("campaign run file entered the claim scan at %q", claim.Path)
		}
	}

	source, err := os.ReadFile("campaign.go")
	if err != nil {
		t.Fatalf("ReadFile(campaign.go) error = %v", err)
	}
	for _, forbidden := range []string{
		"net/http",
		"Approve",
		"Revoke",
		"WriteSupersedingDraft",
		"ClaimTransition",
		"ChallengeStore",
		"PrepareChallenge",
		"ApplyChallenge",
		"Rebuild(",
		"VerifiedDigest",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("campaign.go must not reference %q", forbidden)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return encoded
}
