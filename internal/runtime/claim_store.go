package runtime

import (
	"fmt"
	"os"
	"path/filepath"
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
	Workspace string `json:"workspace"`
	Migrated  int    `json:"migrated"`
	Skipped   int    `json:"skipped"`
	Invalid   int    `json:"invalid"`
}

func (store ClaimStore) WriteDraft(workspace string, claim Claim) (Claim, error) {
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
	if existing, err := store.Read(workspace, claim.ID); err == nil {
		if existing.Status != ClaimStatusDraft {
			return Claim{}, fmt.Errorf("claim %s is %s and cannot be overwritten in place", claim.ID, existing.Status)
		}
	} else if !os.IsNotExist(err) {
		return Claim{}, err
	}
	if err := store.markDirty(workspace); err != nil {
		return Claim{}, err
	}
	if err := writeClaimAtomic(path, claim); err != nil {
		return Claim{}, err
	}
	claim.Path = claimRelPath(claim)
	return claim, nil
}

func (store ClaimStore) Approve(workspace string, id string) (Claim, error) {
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
	if err := store.validateApprovalReferences(workspace, claim); err != nil {
		return Claim{}, err
	}
	sources, err := store.claimSources(workspace, claim.EvidenceIDs)
	if err != nil {
		return Claim{}, err
	}
	oldClaims := make([]Claim, 0, len(claim.Supersedes))
	for _, oldID := range claim.Supersedes {
		old, err := store.Read(workspace, oldID)
		if err != nil {
			return Claim{}, err
		}
		if old.Status == ClaimStatusApproved {
			old.Status = ClaimStatusSuperseded
			old.VerifiedAt = ""
			old.VerifiedBy = ""
			old.VerifiedDigest = ""
			oldClaims = append(oldClaims, old)
		}
	}
	claim.Schema = ""
	claim.Type = OKFClaimType
	claim.Status = ClaimStatusApproved
	claim.Sources = sources
	claim.VerifiedAt = store.now().UTC().Format(time.RFC3339)
	claim.VerifiedBy = "owner"
	claim.VerifiedDigest = ""
	digest, err := ClaimVerificationDigest(claim)
	if err != nil {
		return Claim{}, err
	}
	claim.VerifiedDigest = digest
	if err := store.markDirty(workspace); err != nil {
		return Claim{}, err
	}
	if err := store.writeExisting(workspace, claim); err != nil {
		return Claim{}, err
	}
	for _, old := range oldClaims {
		if err := store.writeExisting(workspace, old); err != nil {
			return Claim{}, err
		}
	}
	return claim, nil
}

func (store ClaimStore) WriteSupersedingDraft(workspace string, currentID string, replacement Claim) (Claim, error) {
	current, err := store.Read(workspace, currentID)
	if err != nil {
		return Claim{}, err
	}
	if current.Status != ClaimStatusApproved {
		return Claim{}, fmt.Errorf("claim %s is %s; only approved claims can be superseded", currentID, current.Status)
	}
	replacement.Supersedes = appendUniqueClaimID(replacement.Supersedes, currentID)
	return store.WriteDraft(workspace, replacement)
}

func (store ClaimStore) Revoke(workspace string, id string, reason string) (Claim, error) {
	claim, err := store.Read(workspace, id)
	if err != nil {
		return Claim{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return Claim{}, fmt.Errorf("revoke reason is required")
	}
	if claim.Status == ClaimStatusRevoked {
		return Claim{}, fmt.Errorf("claim %s is already revoked", id)
	}
	claim.Status = ClaimStatusRevoked
	claim.VerifiedAt = ""
	claim.VerifiedBy = ""
	claim.VerifiedDigest = ""
	claim.Body = strings.TrimRight(claim.Body, "\n") + "\n\nRevoked: " + strings.TrimSpace(reason) + "\n"
	if err := store.markDirty(workspace); err != nil {
		return Claim{}, err
	}
	if err := store.writeExisting(workspace, claim); err != nil {
		return Claim{}, err
	}
	return claim, nil
}

func (store ClaimStore) Read(workspace string, id string) (Claim, error) {
	if !claimIDPattern.MatchString(id) {
		return Claim{}, fmt.Errorf("claim id must match clm_<32 lowercase hex chars>")
	}
	for _, tier := range WikiTiers {
		relative := filepath.ToSlash(filepath.Join("wiki", tier, id+".md"))
		path, err := ResolveWorkspacePath(store.Paths, workspace, relative)
		if err != nil {
			return Claim{}, err
		}
		contents, err := os.ReadFile(path)
		if err == nil {
			return ParseClaimMarkdown(tier, filepath.ToSlash(filepath.Join(tier, id+".md")), contents)
		}
		if !os.IsNotExist(err) {
			return Claim{}, err
		}
	}
	return Claim{}, os.ErrNotExist
}

func (store ClaimStore) ScanWorkspace(workspace string) (ClaimScan, error) {
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
	scan := ClaimScan{}
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
			scan.Invalid = append(scan.Invalid, InvalidClaim{Path: rel, Error: err.Error()})
			return nil
		}
		if err := VerifyClaimDigest(claim); err != nil {
			scan.Invalid = append(scan.Invalid, InvalidClaim{Path: rel, Error: err.Error()})
			return nil
		}
		scan.Claims = append(scan.Claims, claim)
		return nil
	})
	if err != nil {
		return ClaimScan{}, err
	}
	sort.Strings(scan.LegacyUnindexed)
	sort.Slice(scan.Invalid, func(i, j int) bool { return scan.Invalid[i].Path < scan.Invalid[j].Path })
	sort.Slice(scan.Claims, func(i, j int) bool { return scan.Claims[i].Path < scan.Claims[j].Path })
	return scan, nil
}

func (store ClaimStore) MigrateOKF(workspace string) (ClaimMigrationSummary, error) {
	scan, err := store.ScanWorkspace(workspace)
	if err != nil {
		return ClaimMigrationSummary{}, err
	}
	summary := ClaimMigrationSummary{Workspace: workspace, Invalid: len(scan.Invalid), Skipped: len(scan.LegacyUnindexed)}
	migrated := make([]Claim, 0)
	for _, claim := range scan.Claims {
		if claim.Schema != ClaimSchemaVersion {
			summary.Skipped++
			continue
		}
		claim.Schema = ""
		claim.Type = OKFClaimType
		migrated = append(migrated, claim)
	}
	if len(migrated) == 0 {
		return summary, nil
	}
	if err := store.markDirty(workspace); err != nil {
		return ClaimMigrationSummary{}, err
	}
	for _, claim := range migrated {
		if err := store.writeExisting(workspace, claim); err != nil {
			return ClaimMigrationSummary{}, err
		}
		summary.Migrated++
	}
	return summary, nil
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
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
	return nil
}

func appendUniqueClaimID(ids []string, id string) []string {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func (store ClaimStore) validateApprovalReferences(workspace string, claim Claim) error {
	evidenceStore := EvidenceStore{Paths: store.Paths}
	for _, evidenceID := range claim.EvidenceIDs {
		if err := evidenceStore.Verify(workspace, evidenceID); err != nil {
			return fmt.Errorf("verify evidence %s: %w", evidenceID, err)
		}
	}
	for _, supportID := range claim.SupportingClaimIDs {
		support, err := store.Read(workspace, supportID)
		if err != nil {
			return fmt.Errorf("read supporting claim %s: %w", supportID, err)
		}
		if support.Status != ClaimStatusApproved {
			return fmt.Errorf("supporting claim %s is %s; derived claims require approved support", supportID, support.Status)
		}
	}
	return nil
}

func (store ClaimStore) claimSources(workspace string, evidenceIDs []string) ([]ClaimSource, error) {
	sources := make([]ClaimSource, 0, len(evidenceIDs))
	evidenceStore := EvidenceStore{Paths: store.Paths}
	for _, evidenceID := range evidenceIDs {
		evidence, err := evidenceStore.Read(workspace, evidenceID)
		if err != nil {
			return nil, err
		}
		sources = append(sources, ClaimSource{
			ID:       evidence.ID,
			Resource: filepath.ToSlash(filepath.Join("evidence", "sources", evidence.ID, "raw")),
			Title:    evidence.Origin,
			Digest:   "sha256:" + evidence.SHA256,
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
