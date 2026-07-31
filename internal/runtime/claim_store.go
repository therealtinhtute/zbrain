package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	Path  string
	Error string
}

func (store ClaimStore) WriteDraft(workspace string, claim Claim) (Claim, error) {
	claim.Status = ClaimStatusDraft
	claim.Path = ""
	claim.Tier = strings.TrimSpace(claim.Tier)
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
	return claim, writeClaimAtomic(path, claim)
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
	claim.Status = ClaimStatusApproved
	if err := store.writeExisting(workspace, claim); err != nil {
		return Claim{}, err
	}
	for _, oldID := range claim.Supersedes {
		old, err := store.Read(workspace, oldID)
		if err != nil {
			return Claim{}, err
		}
		if old.Status == ClaimStatusApproved {
			old.Status = ClaimStatusSuperseded
			if err := store.writeExisting(workspace, old); err != nil {
				return Claim{}, err
			}
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
	claim.Body = strings.TrimRight(claim.Body, "\n") + "\n\nRevoked: " + strings.TrimSpace(reason) + "\n"
	if err := store.writeExisting(workspace, claim); err != nil {
		return Claim{}, err
	}
	return claim, nil
}

func (store ClaimStore) Read(workspace string, id string) (Claim, error) {
	if !claimIDPattern.MatchString(id) {
		return Claim{}, fmt.Errorf("claim id must match clm_<32 lowercase hex chars>")
	}
	root := filepath.Join(store.Paths.WorkspacesDir, workspace, "wiki")
	for _, tier := range WikiTiers {
		path := filepath.Join(root, tier, id+".md")
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
	wikiRoot := filepath.Join(store.Paths.WorkspacesDir, workspace, "wiki")
	if _, err := os.Stat(wikiRoot); err != nil {
		if os.IsNotExist(err) {
			return ClaimScan{}, fmt.Errorf("workspace %q does not exist", workspace)
		}
		return ClaimScan{}, err
	}
	scan := ClaimScan{}
	err := filepath.WalkDir(wikiRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(wikiRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(string(contents), "---\n") || !strings.Contains(string(contents), "schema: "+ClaimSchemaVersion) {
			scan.LegacyUnindexed = append(scan.LegacyUnindexed, rel)
			return nil
		}
		claim, err := ParseClaimMarkdown(strings.Split(rel, "/")[0], rel, contents)
		if err != nil {
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

func (store ClaimStore) claimPath(workspace string, claim Claim) (string, error) {
	if !IsSafeWorkspaceName(workspace) {
		return "", fmt.Errorf("workspace name must use lowercase letters, numbers, or hyphens only")
	}
	return filepath.Join(store.Paths.WorkspacesDir, workspace, "wiki", claim.Tier, claim.ID+".md"), nil
}

func (store ClaimStore) writeExisting(workspace string, claim Claim) error {
	path, err := store.claimPath(workspace, claim)
	if err != nil {
		return err
	}
	return writeClaimAtomic(path, claim)
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
