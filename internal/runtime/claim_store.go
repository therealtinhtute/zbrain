package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ClaimStore struct {
	Paths Paths
	Now   func() time.Time
}

type ClaimScan struct {
	Claims          []Claim
	LegacyUnindexed []string
	Invalid         []InvalidClaim
}

type InvalidClaim struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

type ClaimMigrationSummary struct {
	Workspace            string   `json:"workspace"`
	Migrated             int      `json:"migrated"`
	Skipped              int      `json:"skipped"`
	Invalid              int      `json:"invalid"`
	ReapprovalRequired   int      `json:"reapproval_required"`
	ReapprovalCandidates []string `json:"reapproval_candidates,omitempty"`
}

func (store ClaimStore) WriteDraft(workspace string, claim Claim) (Claim, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return Claim{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := recoverPendingTransitionForMutationUnlocked(store.Paths, workspace); err != nil {
		return Claim{}, err
	}
	return store.writeDraftUnlocked(workspace, claim)
}

func (store ClaimStore) writeDraftUnlocked(workspace string, claim Claim) (Claim, error) {
	requestedPath := claim.Path
	claim.Schema = ""
	claim.Type = OKFClaimType
	claim.Status = ClaimStatusDraft
	claim.Path = ""
	claim.Tier = strings.TrimSpace(claim.Tier)
	claim.VerifiedAt = ""
	claim.VerifiedBy = ""
	claim.VerifiedDigest = ""
	claim.Sources = nil
	if err := ValidateClaim(claim); err != nil {
		return Claim{}, err
	}
	path, err := store.claimPath(workspace, claim)
	if err != nil {
		return Claim{}, err
	}
	matches, err := store.readCanonicalClaimsByID(workspace, claim.ID)
	if err != nil {
		return Claim{}, err
	}
	if len(matches) > 1 {
		return Claim{}, duplicateCanonicalClaimError(claim.ID, matches)
	}
	var existing Claim
	if requestedPath != "" {
		existing, err = store.readClaimPath(workspace, requestedPath)
		if err == nil && len(matches) == 1 && existing.Path != matches[0].Path {
			return Claim{}, fmt.Errorf("claim %s already exists at canonical path %q; requested path %q would duplicate its identity", claim.ID, matches[0].Path, requestedPath)
		}
		if os.IsNotExist(err) && len(matches) == 1 {
			return Claim{}, fmt.Errorf("claim %s already exists at canonical path %q; requested path %q would duplicate its identity", claim.ID, matches[0].Path, requestedPath)
		}
	} else if len(matches) == 1 {
		existing, err = matches[0], nil
	} else {
		err = os.ErrNotExist
	}
	if err == nil {
		if existing.Status != ClaimStatusDraft {
			return Claim{}, fmt.Errorf("claim %s is %s and cannot be overwritten in place", claim.ID, existing.Status)
		}
		claim.Path = existing.Path
		path, err = store.claimFilePath(workspace, claim)
		if err != nil {
			return Claim{}, err
		}
	} else if !os.IsNotExist(err) {
		return Claim{}, err
	}
	if _, err := beginCanonicalMutationUnlocked(store.Paths, workspace); err != nil {
		return Claim{}, err
	}
	runWorkspaceGenerationTestHook(workspaceGenerationHookBeforeCanonicalWrite)
	if err := writeClaimAtomic(path, claim); err != nil {
		return Claim{}, err
	}
	claim.Path = claimRelPath(claim)
	return claim, nil
}

func (store ClaimStore) Approve(workspace string, id string) (Claim, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return Claim{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := recoverPendingTransitionForMutationUnlocked(store.Paths, workspace); err != nil {
		return Claim{}, err
	}
	return store.approveUnlocked(workspace, id)
}

func (store ClaimStore) approveUnlocked(workspace string, id string) (Claim, error) {
	claim, err := store.Read(workspace, id)
	if err != nil {
		return Claim{}, err
	}
	if claim.Status != ClaimStatusDraft {
		return Claim{}, fmt.Errorf("claim %s is %s; only draft claims can be approved", id, claim.Status)
	}
	if err := ValidateClaimApproval(claim); err != nil {
		return Claim{}, err
	}
	evidenceValidator, err := store.validateApprovalReferences(workspace, claim)
	if err != nil {
		return Claim{}, err
	}
	sources, err := store.claimSources(workspace, claim.EvidenceIDs, evidenceValidator)
	if err != nil {
		return Claim{}, err
	}

	seenOldIDs := make(map[string]struct{}, len(claim.Supersedes))
	oldClaims := make([]Claim, 0, len(claim.Supersedes))
	for _, oldID := range claim.Supersedes {
		if _, ok := seenOldIDs[oldID]; ok {
			return Claim{}, fmt.Errorf("claim %s supersedes duplicate claim %s", id, oldID)
		}
		seenOldIDs[oldID] = struct{}{}
		old, err := store.Read(workspace, oldID)
		if err != nil {
			return Claim{}, err
		}
		if old.Status != ClaimStatusApproved {
			return Claim{}, fmt.Errorf("claim %s is %s; only approved claims can be superseded", oldID, old.Status)
		}
		oldClaims = append(oldClaims, old)
	}

	verifiedAt := store.now().UTC().Format(time.RFC3339)
	claim.Schema = ""
	claim.Type = OKFClaimType
	claim.Status = ClaimStatusApproved
	claim.Sources = sources
	claim.VerifiedAt = verifiedAt
	claim.VerifiedBy = "owner"
	claim.VerifiedDigest = ""
	transitionKind := ClaimTransitionApprove
	if len(oldClaims) > 0 {
		transitionKind = ClaimTransitionSupersede
	}
	claim.Transitions = appendClaimTransition(claim, ClaimTransition{
		Kind:            transitionKind,
		At:              verifiedAt,
		By:              claim.VerifiedBy,
		RelatedClaimIDs: append([]string(nil), claim.Supersedes...),
	})
	digest, err := ClaimVerificationDigest(claim)
	if err != nil {
		return Claim{}, err
	}
	claim.VerifiedDigest = digest

	for i := range oldClaims {
		oldClaims[i].Status = ClaimStatusSuperseded
		oldClaims[i].Transitions = appendClaimTransition(oldClaims[i], ClaimTransition{
			Kind:                    ClaimTransitionSupersede,
			At:                      verifiedAt,
			By:                      claim.VerifiedBy,
			RelatedClaimIDs:         []string{claim.ID},
			PriorVerificationDigest: oldClaims[i].VerifiedDigest,
		})
	}
	var pending *PendingTransition
	if len(oldClaims) > 0 {
		prepared, err := store.pendingSupersession(workspace, claim, oldClaims)
		if err != nil {
			return Claim{}, err
		}
		pending = &prepared
	}
	if _, err := beginCanonicalMutationUnlocked(store.Paths, workspace); err != nil {
		return Claim{}, err
	}
	if pending != nil {
		if err := writePendingTransitionUnlocked(store.Paths, workspace, *pending); err != nil {
			return Claim{}, err
		}
		runWorkspaceGenerationTestHook(workspaceGenerationHookBeforeCanonicalWrite)
		if err := recoverPendingTransitionUnlocked(store.Paths, workspace); err != nil {
			return Claim{}, err
		}
		return claim, nil
	}
	runWorkspaceGenerationTestHook(workspaceGenerationHookBeforeCanonicalWrite)
	if err := store.writeExisting(workspace, claim); err != nil {
		return Claim{}, err
	}
	return claim, nil
}

func (store ClaimStore) WriteSupersedingDraft(workspace string, currentID string, replacement Claim) (Claim, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return Claim{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := recoverPendingTransitionForMutationUnlocked(store.Paths, workspace); err != nil {
		return Claim{}, err
	}
	current, err := store.Read(workspace, currentID)
	if err != nil {
		return Claim{}, err
	}
	if current.Status != ClaimStatusApproved {
		return Claim{}, fmt.Errorf("claim %s is %s; only approved claims can be superseded", currentID, current.Status)
	}
	replacement.Supersedes = appendUniqueClaimID(replacement.Supersedes, currentID)
	return store.writeDraftUnlocked(workspace, replacement)
}

func (store ClaimStore) Revoke(workspace string, id string, reason string) (Claim, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return Claim{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := recoverPendingTransitionForMutationUnlocked(store.Paths, workspace); err != nil {
		return Claim{}, err
	}
	claim, err := store.Read(workspace, id)
	if err != nil {
		return Claim{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return Claim{}, fmt.Errorf("revoke reason is required")
	}
	if claim.Status != ClaimStatusApproved {
		return Claim{}, fmt.Errorf("claim %s is %s; only approved claims can be revoked", id, claim.Status)
	}
	claim.Status = ClaimStatusRevoked
	claim.Transitions = appendClaimTransition(claim, ClaimTransition{
		Kind:                    ClaimTransitionRevoke,
		At:                      store.now().UTC().Format(time.RFC3339),
		By:                      "owner",
		Reason:                  strings.TrimSpace(reason),
		RelatedClaimIDs:         []string{id},
		PriorVerificationDigest: claim.VerifiedDigest,
	})
	if _, err := beginCanonicalMutationUnlocked(store.Paths, workspace); err != nil {
		return Claim{}, err
	}
	runWorkspaceGenerationTestHook(workspaceGenerationHookBeforeCanonicalWrite)
	if err := store.writeExisting(workspace, claim); err != nil {
		return Claim{}, err
	}
	return claim, nil
}

func (store ClaimStore) Read(workspace string, id string) (Claim, error) {
	if !claimIDPattern.MatchString(id) {
		return Claim{}, fmt.Errorf("claim id must match clm_<32 lowercase hex chars>")
	}
	matches, err := store.readCanonicalClaimsByID(workspace, id)
	if err != nil {
		return Claim{}, err
	}
	if len(matches) == 0 {
		return Claim{}, os.ErrNotExist
	}
	if len(matches) > 1 {
		return Claim{}, duplicateCanonicalClaimError(id, matches)
	}
	return matches[0], nil
}

func (store ClaimStore) readCanonicalClaimsByID(workspace string, id string) ([]Claim, error) {
	if !claimIDPattern.MatchString(id) {
		return nil, fmt.Errorf("claim id must match clm_<32 lowercase hex chars>")
	}
	workspaceRoot, err := ValidateWorkspace(store.Paths, workspace)
	if err != nil {
		return nil, err
	}
	wikiRoot, err := ResolveWorkspacePath(store.Paths, workspace, "wiki")
	if err != nil {
		return nil, err
	}
	matches, err := store.readFlatClaims(workspace, id)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	nested, err := wikiHasNestedDirectories(wikiRoot)
	if err != nil {
		return nil, err
	}
	if !nested {
		sort.Slice(matches, func(i, j int) bool { return matches[i].Path < matches[j].Path })
		return matches, nil
	}

	matches = make([]Claim, 0)
	err = filepath.WalkDir(wikiRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		workspaceRel, err := filepath.Rel(workspaceRoot, path)
		if err != nil {
			return err
		}
		safePath, err := ResolveWorkspacePath(store.Paths, workspace, filepath.ToSlash(workspaceRel))
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(safePath)
		if err != nil {
			return err
		}
		if !isZbrainClaimDocument(contents) {
			return nil
		}
		rel, err := filepath.Rel(wikiRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		tier := strings.Split(rel, "/")[0]
		claim, err := ParseClaimMarkdown(tier, rel, contents)
		if err != nil {
			return err
		}
		if claim.ID != id {
			return nil
		}
		matches = append(matches, claim)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Path < matches[j].Path })
	return matches, nil
}

func wikiHasNestedDirectories(wikiRoot string) (bool, error) {
	for _, tier := range WikiTiers {
		tierRoot := filepath.Join(wikiRoot, tier)
		info, err := os.Lstat(tierRoot)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return true, nil
		}
		hasNested, known := fileInfoHasSubdirectories(info)
		if !known || hasNested {
			return true, nil
		}
	}
	return false, nil
}

func fileInfoHasSubdirectories(info os.FileInfo) (bool, bool) {
	if info == nil {
		return false, false
	}
	value := reflect.ValueOf(info.Sys())
	if !value.IsValid() {
		return false, false
	}
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return false, false
	}
	nlink, ok := reflectInteger(value.FieldByName("Nlink"))
	if !ok || nlink < 2 {
		return false, false
	}
	return nlink > 2, true
}

func duplicateCanonicalClaimError(id string, claims []Claim) error {
	paths := make([]string, 0, len(claims))
	for _, claim := range claims {
		paths = append(paths, claim.Path)
	}
	return duplicateCanonicalClaimPathsError(id, paths)
}

func duplicateCanonicalClaimPathsError(id string, paths []string) error {
	ordered := append([]string(nil), paths...)
	sort.Strings(ordered)
	return fmt.Errorf("duplicate canonical claim ID %q found at paths: %s", id, strings.Join(ordered, ", "))
}

func (store ClaimStore) readFlatClaims(workspace string, id string) ([]Claim, error) {
	if !claimIDPattern.MatchString(id) {
		return nil, fmt.Errorf("claim id must match clm_<32 lowercase hex chars>")
	}
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return nil, err
	}
	matches := make([]Claim, 0)
	for _, tier := range WikiTiers {
		relative := filepath.ToSlash(filepath.Join("wiki", tier, id+".md"))
		path, err := ResolveWorkspacePath(store.Paths, workspace, relative)
		if err != nil {
			return nil, err
		}
		contents, err := os.ReadFile(path)
		if err == nil {
			claim, err := ParseClaimMarkdown(tier, filepath.ToSlash(filepath.Join(tier, id+".md")), contents)
			if err != nil {
				return nil, err
			}
			matches = append(matches, claim)
			continue
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	if len(matches) == 0 {
		return nil, os.ErrNotExist
	}
	return matches, nil
}

func (store ClaimStore) readFlatClaim(workspace string, id string) (Claim, error) {
	matches, err := store.readFlatClaims(workspace, id)
	if err != nil {
		return Claim{}, err
	}
	if len(matches) > 1 {
		return Claim{}, duplicateCanonicalClaimError(id, matches)
	}
	return matches[0], nil
}

func (store ClaimStore) readClaimPath(workspace string, relative string) (Claim, error) {
	relative = filepath.ToSlash(relative)
	if _, err := safeRelativePath(relative); err != nil {
		return Claim{}, err
	}
	parts := strings.Split(relative, "/")
	if len(parts) < 2 || !isKnownWikiTier(parts[0]) || filepath.Ext(filepath.FromSlash(relative)) != ".md" {
		return Claim{}, fmt.Errorf("claim path %q is not safe", relative)
	}
	path, err := ResolveWorkspacePath(store.Paths, workspace, filepath.ToSlash(filepath.Join("wiki", filepath.FromSlash(relative))))
	if err != nil {
		return Claim{}, err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return Claim{}, err
	}
	return ParseClaimMarkdown(parts[0], relative, contents)
}

func (store ClaimStore) ScanWorkspace(workspace string) (ClaimScan, error) {
	return store.scanWorkspace(workspace, true)
}

// ScanWorkspaceForTrust parses every zbrain claim so rebuild can classify invalid approved roots.
func (store ClaimStore) ScanWorkspaceForTrust(workspace string) (ClaimScan, error) {
	return store.scanWorkspace(workspace, false)
}

func (store ClaimStore) scanWorkspace(workspace string, verifyDigests bool) (ClaimScan, error) {
	workspaceRoot, err := ValidateWorkspace(store.Paths, workspace)
	if err != nil {
		return ClaimScan{}, err
	}
	wikiRoot, err := ResolveWorkspacePath(store.Paths, workspace, "wiki")
	if err != nil {
		return ClaimScan{}, err
	}
	if _, err := os.Stat(wikiRoot); err != nil {
		if os.IsNotExist(err) {
			return ClaimScan{}, fmt.Errorf("workspace %q does not exist", workspace)
		}
		return ClaimScan{}, err
	}
	type parsedClaim struct {
		claim Claim
		path  string
	}
	scan := ClaimScan{}
	parsed := make([]parsedClaim, 0)
	claimsByID := make(map[string][]parsedClaim)
	invalidByPath := make(map[string][]string)
	err = filepath.WalkDir(wikiRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(wikiRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		workspaceRel, err := filepath.Rel(workspaceRoot, path)
		if err != nil {
			return err
		}
		safePath, err := ResolveWorkspacePath(store.Paths, workspace, filepath.ToSlash(workspaceRel))
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(safePath)
		if err != nil {
			return err
		}
		if !isZbrainClaimDocument(contents) {
			scan.LegacyUnindexed = append(scan.LegacyUnindexed, rel)
			return nil
		}
		claim, err := ParseClaimMarkdown(strings.Split(rel, "/")[0], rel, contents)
		if err != nil {
			invalidByPath[rel] = append(invalidByPath[rel], err.Error())
			return nil
		}
		parsedClaim := parsedClaim{claim: claim, path: rel}
		parsed = append(parsed, parsedClaim)
		claimsByID[claim.ID] = append(claimsByID[claim.ID], parsedClaim)
		if verifyDigests {
			if err := VerifyClaimDigest(claim); err != nil {
				invalidByPath[rel] = append(invalidByPath[rel], err.Error())
			}
		}
		return nil
	})
	if err != nil {
		return ClaimScan{}, err
	}

	ids := make([]string, 0, len(claimsByID))
	for id, claims := range claimsByID {
		if len(claims) > 1 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		claims := claimsByID[id]
		paths := make([]string, 0, len(claims))
		for _, parsedClaim := range claims {
			paths = append(paths, parsedClaim.path)
		}
		sort.Strings(paths)
		reason := fmt.Sprintf("duplicate canonical claim ID %q found at paths: %s", id, strings.Join(paths, ", "))
		for _, parsedClaim := range claims {
			invalidByPath[parsedClaim.path] = append(invalidByPath[parsedClaim.path], reason)
		}
	}

	sort.Slice(parsed, func(i, j int) bool { return parsed[i].path < parsed[j].path })
	for _, parsedClaim := range parsed {
		if len(invalidByPath[parsedClaim.path]) != 0 {
			continue
		}
		scan.Claims = append(scan.Claims, parsedClaim.claim)
	}
	invalidPaths := make([]string, 0, len(invalidByPath))
	for path := range invalidByPath {
		invalidPaths = append(invalidPaths, path)
	}
	sort.Strings(invalidPaths)
	for _, path := range invalidPaths {
		reasons := invalidByPath[path]
		sort.Strings(reasons)
		scan.Invalid = append(scan.Invalid, InvalidClaim{Path: path, Error: strings.Join(reasons, "; ")})
	}
	sort.Strings(scan.LegacyUnindexed)
	return scan, nil
}

func (store ClaimStore) MigrateOKF(workspace string) (ClaimMigrationSummary, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return ClaimMigrationSummary{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := recoverPendingTransitionForMutationUnlocked(store.Paths, workspace); err != nil {
		return ClaimMigrationSummary{}, err
	}
	scan, err := store.scanWorkspace(workspace, false)
	if err != nil {
		return ClaimMigrationSummary{}, err
	}
	summary := ClaimMigrationSummary{Workspace: workspace, Invalid: len(scan.Invalid), Skipped: len(scan.LegacyUnindexed)}
	migrated := make([]Claim, 0)
	for _, claim := range scan.Claims {
		if claim.Schema != ClaimSchemaVersion {
			if claim.Status == ClaimStatusApproved {
				if err := VerifyClaimDigest(claim); err != nil {
					summary.Invalid++
				}
			}
			summary.Skipped++
			continue
		}
		claim, requiresReapproval := migrateLegacyClaim(claim)
		if requiresReapproval {
			summary.ReapprovalRequired++
			summary.ReapprovalCandidates = append(summary.ReapprovalCandidates, claim.ID)
		}
		migrated = append(migrated, claim)
	}
	if len(migrated) == 0 {
		return summary, nil
	}
	if _, err := beginCanonicalMutationUnlocked(store.Paths, workspace); err != nil {
		return ClaimMigrationSummary{}, err
	}
	runWorkspaceGenerationTestHook(workspaceGenerationHookBeforeCanonicalWrite)
	for _, claim := range migrated {
		if err := store.writeExisting(workspace, claim); err != nil {
			return ClaimMigrationSummary{}, err
		}
		summary.Migrated++
	}
	return summary, nil
}

func migrateLegacyClaim(claim Claim) (Claim, bool) {
	claim.Schema = ""
	claim.Type = OKFClaimType
	if claim.Status != ClaimStatusApproved {
		return claim, false
	}
	if err := VerifyClaimDigest(claim); err == nil {
		return claim, false
	}
	claim.Status = ClaimStatusDraft
	claim.VerifiedAt = ""
	claim.VerifiedBy = ""
	claim.VerifiedDigest = ""
	return claim, true
}

func (store ClaimStore) claimPath(workspace string, claim Claim) (string, error) {
	relative := filepath.ToSlash(filepath.Join("wiki", claim.Tier, claim.ID+".md"))
	return ResolveWorkspacePath(store.Paths, workspace, relative)
}

func (store ClaimStore) claimFilePath(workspace string, claim Claim) (string, error) {
	if claim.Path != "" {
		if filepath.Ext(filepath.FromSlash(claim.Path)) != ".md" {
			return "", fmt.Errorf("claim path %q is not safe", claim.Path)
		}
		return ResolveWorkspacePath(store.Paths, workspace, filepath.ToSlash("wiki/"+claim.Path))
	}
	return store.claimPath(workspace, claim)
}

func (store ClaimStore) writeExisting(workspace string, claim Claim) error {
	path, err := store.claimFilePath(workspace, claim)
	if err != nil {
		return err
	}
	return writeClaimAtomic(path, claim)
}

func (store ClaimStore) pendingSupersession(workspace string, replacement Claim, oldClaims []Claim) (PendingTransition, error) {
	workspaceRoot, err := ValidateWorkspace(store.Paths, workspace)
	if err != nil {
		return PendingTransition{}, err
	}
	operationID, err := NewPendingTransitionID()
	if err != nil {
		return PendingTransition{}, err
	}
	claims := make([]Claim, 0, len(oldClaims)+1)
	claims = append(claims, replacement)
	claims = append(claims, oldClaims...)
	targets := make([]PendingTransitionTarget, 0, len(claims))
	for _, claim := range claims {
		path, err := store.claimFilePath(workspace, claim)
		if err != nil {
			return PendingTransition{}, err
		}
		contents, err := RenderClaimMarkdown(claim)
		if err != nil {
			return PendingTransition{}, err
		}
		preimage, err := os.ReadFile(path)
		if err != nil {
			return PendingTransition{}, fmt.Errorf("read supersession preimage %q: %w", path, err)
		}
		relative, err := filepath.Rel(workspaceRoot, path)
		if err != nil {
			return PendingTransition{}, err
		}
		targets = append(targets, PendingTransitionTarget{
			Path:           filepath.ToSlash(relative),
			PreimageSHA256: transitionSHA256(preimage),
			TargetSHA256:   transitionSHA256(contents),
			TargetBytes:    contents,
		})
	}
	return PendingTransition{OperationID: operationID, Kind: ClaimTransitionSupersede, Workspace: workspace, Targets: targets}, nil
}

func (store ClaimStore) markDirty(workspace string) error {
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return err
	}
	return (IndexStore{Paths: store.Paths}).MarkDirty(workspace)
}

func writeClaimAtomic(path string, claim Claim) error {
	contents, err := RenderClaimMarkdown(claim)
	if err != nil {
		return err
	}
	if err := ensureDirectoryMode(filepath.Dir(path), runtimeDirectoryMode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(runtimeMetadataMode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(contents); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return ensureFileMode(path, runtimeMetadataMode)
}

func appendUniqueClaimID(ids []string, id string) []string {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func appendClaimTransition(claim Claim, transition ClaimTransition) []ClaimTransition {
	transitions := append([]ClaimTransition(nil), claim.Transitions...)
	transition.RelatedClaimIDs = append([]string(nil), transition.RelatedClaimIDs...)
	return append(transitions, transition)
}

func (store ClaimStore) validateApprovalReferences(workspace string, claim Claim) (*EvidenceValidator, error) {
	evidenceValidator, err := NewEvidenceValidator(EvidenceStore{Paths: store.Paths}, workspace)
	if err != nil {
		return nil, err
	}
	if err := validateClaimEvidence(evidenceValidator, claim); err != nil {
		return nil, fmt.Errorf("verify claim evidence: %w", err)
	}
	if len(claim.SupportingClaimIDs) == 0 {
		return evidenceValidator, nil
	}
	validator, err := NewTrustValidatorFromStore(store, workspace)
	if err != nil {
		return nil, err
	}
	validator.validateSupporting = func(support Claim) error {
		return validateClaimEvidence(evidenceValidator, support)
	}
	if err := validator.ValidateClaim(claim); err != nil {
		return nil, fmt.Errorf("validate supporting claims for %s: %w", claim.ID, err)
	}
	return evidenceValidator, nil
}

func (store ClaimStore) claimSources(workspace string, evidenceIDs []string, validator *EvidenceValidator) ([]ClaimSource, error) {
	if validator == nil {
		return nil, fmt.Errorf("evidence validator is nil")
	}
	sources := make([]ClaimSource, 0, len(evidenceIDs))
	evidenceStore := EvidenceStore{Paths: store.Paths}
	for _, evidenceID := range evidenceIDs {
		evidence, err := evidenceStore.Read(workspace, evidenceID)
		if err != nil {
			return nil, err
		}
		if err := validator.Verify(evidenceID); err != nil {
			return nil, err
		}
		sources = append(sources, ClaimSource{
			ID:       evidence.ID,
			Resource: filepath.ToSlash(filepath.Join("evidence", "sources", evidence.ID, "raw")),
			Title:    evidence.Origin,
			Digest:   validator.snapshotDigests[evidenceID],
		})
	}
	return sources, nil
}

func (store ClaimStore) now() time.Time {
	if store.Now != nil {
		return store.Now()
	}
	return time.Now()
}

func claimRelPath(claim Claim) string {
	if claim.Path != "" {
		return filepath.ToSlash(claim.Path)
	}
	return filepath.ToSlash(filepath.Join(claim.Tier, claim.ID+".md"))
}

func isZbrainClaimDocument(contents []byte) bool {
	frontmatter, _, err := splitMarkdownFrontmatter(contents)
	if err != nil {
		return false
	}
	var probe struct {
		Schema string `yaml:"schema"`
		Type   string `yaml:"type"`
	}
	if err := yaml.Unmarshal(frontmatter, &probe); err != nil {
		return false
	}
	return probe.Schema == ClaimSchemaVersion || probe.Type == OKFClaimType
}
