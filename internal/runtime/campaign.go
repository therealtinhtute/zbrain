package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// CampaignSchemaVersion identifies the persisted authoring-campaign run
// shape. Campaign run files are runtime metadata, never trust inputs: nothing
// in ask or the derived index reads them, and a malformed run file fails
// closed instead of discarding resumable work.
const CampaignSchemaVersion = "zbrain.campaign/v1"

// CampaignPhase is the lifecycle phase of an authoring campaign run.
type CampaignPhase string

const (
	CampaignPhaseDrafting CampaignPhase = "drafting"
	CampaignPhaseFinished CampaignPhase = "finished"
)

// CampaignDraftStatus is the per-draft state inside a campaign run.
type CampaignDraftStatus string

const (
	CampaignDraftStatusPending           CampaignDraftStatus = "pending"
	CampaignDraftStatusSubmitted         CampaignDraftStatus = "submitted"
	CampaignDraftStatusSupersededByOwner CampaignDraftStatus = "superseded-by-owner"
)

// CampaignSpec is one claim draft to author. It carries exactly the fields
// the existing claim-draft path accepts; the body is supplied at submit time.
type CampaignSpec struct {
	Tier               string   `json:"tier"`
	Title              string   `json:"title"`
	Basis              string   `json:"basis"`
	EvidenceIDs        []string `json:"evidence,omitempty"`
	SupportingClaimIDs []string `json:"support,omitempty"`
	ConflictsWith      []string `json:"conflicts_with,omitempty"`
}

// CampaignDraft is one ordered entry in a campaign run.
type CampaignDraft struct {
	Spec        CampaignSpec        `json:"spec"`
	Status      CampaignDraftStatus `json:"status"`
	ClaimID     string              `json:"claim_id,omitempty"`
	SubmittedAt string              `json:"submitted_at,omitempty"`
}

// CampaignRun is the persisted, resumable campaign run file.
type CampaignRun struct {
	Schema    string          `json:"schema"`
	RunID     string          `json:"run_id"`
	Phase     CampaignPhase   `json:"phase"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	Drafts    []CampaignDraft `json:"drafts"`
}

// CampaignState is the resumable state of a campaign run.
type CampaignState struct {
	Run               CampaignRun `json:"run"`
	Pending           int         `json:"pending"`
	Submitted         int         `json:"submitted"`
	SupersededByOwner int         `json:"superseded_by_owner"`
	NextIndex         int         `json:"next_index"`
}

// CampaignSubmission reports the claim created by one submitted draft.
type CampaignSubmission struct {
	RunID       string      `json:"run_id"`
	Index       int         `json:"index"`
	ClaimID     string      `json:"claim_id"`
	ClaimPath   string      `json:"claim_path"`
	ClaimStatus ClaimStatus `json:"claim_status"`
	State       CampaignState
}

// CampaignStore drives resumable bulk authoring of claim drafts. It never
// approves, supersedes, revokes, or reindexes: submission reuses the existing
// claim-draft write path, so every campaign claim is born a draft.
type CampaignStore struct {
	Paths Paths
	Now   func() time.Time
}

var campaignRunIDPattern = regexp.MustCompile(`^cmp_[0-9a-f]{32}$`)

// NewCampaignRunID returns a new campaign run ID.
func NewCampaignRunID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "cmp_" + hex.EncodeToString(buf), nil
}

// BeginCampaign validates the specs with the claim-draft validators, persists
// a new drafting run file, and returns it. No claims are created.
func (store CampaignStore) BeginCampaign(workspace string, specs []CampaignSpec) (CampaignRun, error) {
	if len(specs) == 0 {
		return CampaignRun{}, fmt.Errorf("campaign requires at least one draft spec")
	}
	for index, spec := range specs {
		if err := ValidateCampaignSpec(spec); err != nil {
			return CampaignRun{}, fmt.Errorf("campaign spec %d: %w", index, err)
		}
	}
	runID, err := NewCampaignRunID()
	if err != nil {
		return CampaignRun{}, err
	}
	now := store.now().UTC().Format(time.RFC3339)
	drafts := make([]CampaignDraft, 0, len(specs))
	for _, spec := range specs {
		drafts = append(drafts, CampaignDraft{Spec: cloneCampaignSpec(spec), Status: CampaignDraftStatusPending})
	}
	run := CampaignRun{
		Schema:    CampaignSchemaVersion,
		RunID:     runID,
		Phase:     CampaignPhaseDrafting,
		CreatedAt: now,
		UpdatedAt: now,
		Drafts:    drafts,
	}
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return CampaignRun{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := store.writeCampaignRunUnlocked(workspace, run); err != nil {
		return CampaignRun{}, err
	}
	return run, nil
}

// NextCampaignDraft returns the index and spec of the next pending draft in
// deterministic order without mutating anything.
func (store CampaignStore) NextCampaignDraft(workspace string, runID string) (int, CampaignSpec, error) {
	state, err := store.ResumeCampaign(workspace, runID)
	if err != nil {
		return -1, CampaignSpec{}, err
	}
	if state.NextIndex < 0 {
		return -1, CampaignSpec{}, fmt.Errorf("campaign run %s has no pending drafts", runID)
	}
	return state.NextIndex, state.Run.Drafts[state.NextIndex].Spec, nil
}

// SubmitCampaignDraft marks the pending draft at index submitted under the
// workspace lock and creates the claim through the existing claim-draft write
// path. An already-submitted or unknown index fails closed, and the run file
// is never advanced when claim creation fails.
func (store CampaignStore) SubmitCampaignDraft(workspace string, runID string, index int, body string) (CampaignSubmission, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return CampaignSubmission{}, err
	}
	defer func() { _ = lock.Close() }()
	run, err := store.readCampaignRunUnlocked(workspace, runID)
	if err != nil {
		return CampaignSubmission{}, err
	}
	if run.Phase != CampaignPhaseDrafting {
		return CampaignSubmission{}, fmt.Errorf("campaign run %s is %s; only drafting runs accept submissions", runID, run.Phase)
	}
	if index < 0 || index >= len(run.Drafts) {
		return CampaignSubmission{}, fmt.Errorf("campaign draft index %d is out of range for run %s", index, runID)
	}
	draft := &run.Drafts[index]
	if draft.Status != CampaignDraftStatusPending {
		return CampaignSubmission{}, fmt.Errorf("campaign draft %d of run %s is %s; only pending drafts can be submitted", index, runID, draft.Status)
	}
	claimID, err := NewClaimID()
	if err != nil {
		return CampaignSubmission{}, err
	}
	claim := Claim{
		Type:               OKFClaimType,
		ID:                 claimID,
		Tier:               draft.Spec.Tier,
		Status:             ClaimStatusDraft,
		Title:              draft.Spec.Title,
		Basis:              ClaimBasis(draft.Spec.Basis),
		CreatedAt:          store.now().UTC().Format(time.RFC3339),
		CreatedBy:          "owner:mcp",
		EvidenceIDs:        append([]string(nil), draft.Spec.EvidenceIDs...),
		SupportingClaimIDs: append([]string(nil), draft.Spec.SupportingClaimIDs...),
		ConflictsWith:      append([]string(nil), draft.Spec.ConflictsWith...),
		Body:               body,
	}
	claimStore := ClaimStore{Paths: store.Paths, Now: store.Now}
	if err := recoverPendingTransitionForMutationUnlocked(store.Paths, workspace); err != nil {
		return CampaignSubmission{}, err
	}
	created, err := claimStore.writeDraftUnlocked(workspace, claim)
	if err != nil {
		return CampaignSubmission{}, err
	}
	draft.Status = CampaignDraftStatusSubmitted
	draft.ClaimID = created.ID
	draft.SubmittedAt = store.now().UTC().Format(time.RFC3339)
	run.UpdatedAt = draft.SubmittedAt
	if err := store.writeCampaignRunUnlocked(workspace, run); err != nil {
		return CampaignSubmission{}, err
	}
	state := campaignStateFromRun(run)
	return CampaignSubmission{
		RunID:       runID,
		Index:       index,
		ClaimID:     created.ID,
		ClaimPath:   claimRelPath(created),
		ClaimStatus: created.Status,
		State:       state,
	}, nil
}

// ResumeCampaign reads and validates a run file and returns its resumable
// state without mutating anything.
func (store CampaignStore) ResumeCampaign(workspace string, runID string) (CampaignState, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, false)
	if err != nil {
		return CampaignState{}, err
	}
	defer func() { _ = lock.Close() }()
	run, err := store.readCampaignRunUnlocked(workspace, runID)
	if err != nil {
		return CampaignState{}, err
	}
	return campaignStateFromRun(run), nil
}

// FinishCampaign marks a run finished once no pending drafts remain.
func (store CampaignStore) FinishCampaign(workspace string, runID string) (CampaignRun, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return CampaignRun{}, err
	}
	defer func() { _ = lock.Close() }()
	run, err := store.readCampaignRunUnlocked(workspace, runID)
	if err != nil {
		return CampaignRun{}, err
	}
	for index, draft := range run.Drafts {
		if draft.Status == CampaignDraftStatusPending {
			return CampaignRun{}, fmt.Errorf("campaign run %s still has pending draft %d; submit every draft before finishing", runID, index)
		}
	}
	run.Phase = CampaignPhaseFinished
	run.UpdatedAt = store.now().UTC().Format(time.RFC3339)
	if err := store.writeCampaignRunUnlocked(workspace, run); err != nil {
		return CampaignRun{}, err
	}
	return run, nil
}

// ValidateCampaignSpec applies the claim-draft validators to a spec without
// creating any claim.
func ValidateCampaignSpec(spec CampaignSpec) error {
	probeID, err := NewClaimID()
	if err != nil {
		return err
	}
	probe := Claim{
		Type:               OKFClaimType,
		ID:                 probeID,
		Tier:               spec.Tier,
		Status:             ClaimStatusDraft,
		Title:              spec.Title,
		Basis:              ClaimBasis(spec.Basis),
		CreatedAt:          "2026-01-01T00:00:00Z",
		CreatedBy:          "campaign",
		EvidenceIDs:        append([]string(nil), spec.EvidenceIDs...),
		SupportingClaimIDs: append([]string(nil), spec.SupportingClaimIDs...),
		ConflictsWith:      append([]string(nil), spec.ConflictsWith...),
		Body:               "campaign spec",
	}
	return ValidateClaim(probe)
}

func campaignStateFromRun(run CampaignRun) CampaignState {
	state := CampaignState{Run: run, NextIndex: -1}
	for index, draft := range run.Drafts {
		switch draft.Status {
		case CampaignDraftStatusPending:
			state.Pending++
			if state.NextIndex < 0 {
				state.NextIndex = index
			}
		case CampaignDraftStatusSubmitted:
			state.Submitted++
		case CampaignDraftStatusSupersededByOwner:
			state.SupersededByOwner++
		}
	}
	return state
}

func cloneCampaignSpec(spec CampaignSpec) CampaignSpec {
	return CampaignSpec{
		Tier:               spec.Tier,
		Title:              spec.Title,
		Basis:              spec.Basis,
		EvidenceIDs:        append([]string(nil), spec.EvidenceIDs...),
		SupportingClaimIDs: append([]string(nil), spec.SupportingClaimIDs...),
		ConflictsWith:      append([]string(nil), spec.ConflictsWith...),
	}
}

func (store CampaignStore) campaignRunPath(workspace string, runID string) (string, error) {
	if !campaignRunIDPattern.MatchString(runID) {
		return "", fmt.Errorf("campaign run id must match cmp_<32 lowercase hex chars>")
	}
	root, err := ValidateWorkspace(store.Paths, workspace)
	if err != nil {
		return "", err
	}
	directory := filepath.Join(root, "campaigns")
	if err := validateCampaignDirectory(root, directory); err != nil {
		return "", err
	}
	path := filepath.Join(directory, runID+".json")
	if err := validateWorkspaceControlFile(path); err != nil {
		return "", err
	}
	return path, nil
}

func validateCampaignDirectory(root string, directory string) error {
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("campaigns directory must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("campaigns directory is not a directory")
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return err
	}
	if !pathWithin(root, resolved) {
		return fmt.Errorf("campaigns directory resolves outside workspace")
	}
	return nil
}

// readCampaignRunUnlocked loads a run file and fails closed on any malformed
// shape. A malformed run file is a hard error that preserves the resumable
// work on disk: recovery is an explicit operator decision to remove the file.
func (store CampaignStore) readCampaignRunUnlocked(workspace string, runID string) (CampaignRun, error) {
	path, err := store.campaignRunPath(workspace, runID)
	if err != nil {
		return CampaignRun{}, err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CampaignRun{}, fmt.Errorf("campaign run %s not found in workspace %q", runID, workspace)
		}
		return CampaignRun{}, err
	}
	malformed := func(cause error) (CampaignRun, error) {
		return CampaignRun{}, fmt.Errorf("campaign run file %s is malformed (%v); refusing to discard resumable work; remove it explicitly to abandon the run", path, cause)
	}
	var run CampaignRun
	if err := json.Unmarshal(contents, &run); err != nil {
		return malformed(err)
	}
	if err := validateCampaignRunFile(run, runID); err != nil {
		return malformed(err)
	}
	return run, nil
}

func validateCampaignRunFile(run CampaignRun, expectedRunID string) error {
	if run.Schema != CampaignSchemaVersion {
		return fmt.Errorf("schema %q must be %q", run.Schema, CampaignSchemaVersion)
	}
	if !campaignRunIDPattern.MatchString(run.RunID) || run.RunID != expectedRunID {
		return fmt.Errorf("run_id %q must match the requested run id %q", run.RunID, expectedRunID)
	}
	switch run.Phase {
	case CampaignPhaseDrafting, CampaignPhaseFinished:
	default:
		return fmt.Errorf("phase %q is not supported", run.Phase)
	}
	if _, err := time.Parse(time.RFC3339, run.CreatedAt); err != nil {
		return fmt.Errorf("created_at must be RFC3339: %w", err)
	}
	if _, err := time.Parse(time.RFC3339, run.UpdatedAt); err != nil {
		return fmt.Errorf("updated_at must be RFC3339: %w", err)
	}
	if len(run.Drafts) == 0 {
		return fmt.Errorf("campaign run must contain at least one draft")
	}
	seenClaims := make(map[string]struct{}, len(run.Drafts))
	pendingSeen := false
	for index, draft := range run.Drafts {
		switch draft.Status {
		case CampaignDraftStatusPending:
			pendingSeen = true
			if draft.ClaimID != "" || draft.SubmittedAt != "" {
				return fmt.Errorf("pending draft %d must not carry claim or submission metadata", index)
			}
		case CampaignDraftStatusSubmitted, CampaignDraftStatusSupersededByOwner:
			if !claimIDPattern.MatchString(draft.ClaimID) {
				return fmt.Errorf("draft %d with status %q requires a claim id matching clm_<32 lowercase hex chars>", index, draft.Status)
			}
			if _, err := time.Parse(time.RFC3339, draft.SubmittedAt); err != nil {
				return fmt.Errorf("draft %d submitted_at must be RFC3339: %w", index, err)
			}
			if _, ok := seenClaims[draft.ClaimID]; ok {
				return fmt.Errorf("claim id %s is recorded by more than one draft", draft.ClaimID)
			}
			seenClaims[draft.ClaimID] = struct{}{}
		default:
			return fmt.Errorf("draft %d status %q is not supported", index, draft.Status)
		}
		if err := ValidateCampaignSpec(draft.Spec); err != nil {
			return fmt.Errorf("draft %d spec: %w", index, err)
		}
	}
	if run.Phase == CampaignPhaseFinished && pendingSeen {
		return fmt.Errorf("finished campaign run must not contain pending drafts")
	}
	return nil
}

func (store CampaignStore) writeCampaignRunUnlocked(workspace string, run CampaignRun) error {
	if err := validateCampaignRunFile(run, run.RunID); err != nil {
		return fmt.Errorf("campaign run %s is invalid: %w", run.RunID, err)
	}
	path, err := store.campaignRunPath(workspace, run.RunID)
	if err != nil {
		return err
	}
	if err := ensureDirectoryMode(filepath.Dir(path), runtimeDirectoryMode); err != nil {
		return fmt.Errorf("create campaigns directory: %w", err)
	}
	encoded, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal campaign run %s: %w", run.RunID, err)
	}
	encoded = append(encoded, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+run.RunID+".*.tmp")
	if err != nil {
		return fmt.Errorf("create campaign run temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(runtimeMetadataMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set campaign run temporary permissions: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write campaign run %s: %w", run.RunID, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync campaign run %s: %w", run.RunID, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close campaign run %s: %w", run.RunID, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish campaign run %s: %w", run.RunID, err)
	}
	return ensureFileMode(path, runtimeMetadataMode)
}

func (store CampaignStore) now() time.Time {
	if store.Now != nil {
		return store.Now()
	}
	return time.Now()
}
