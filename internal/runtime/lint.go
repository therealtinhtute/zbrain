package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// StructuralFindings returns read-only workspace hygiene findings. It takes a
// shared lock internally (the flock helper is unexported). Callers must not
// rewrite canonical files from these strings.
func StructuralFindings(paths Paths, workspace string) ([]string, error) {
	return structuralFindingsAt(paths, workspace, time.Now().UTC())
}

func structuralFindingsAt(paths Paths, workspace string, now time.Time) ([]string, error) {
	lock, err := acquireWorkspaceLock(paths, workspace, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = lock.Close() }()

	scan, err := (ClaimStore{Paths: paths}).ScanWorkspaceForTrust(workspace)
	if err != nil {
		return nil, err
	}
	claimIDs := make(map[string]struct{}, len(scan.Claims))
	citedEvidence := make(map[string]struct{})
	for _, claim := range scan.Claims {
		claimIDs[claim.ID] = struct{}{}
		for _, id := range claim.EvidenceIDs {
			citedEvidence[id] = struct{}{}
		}
		for _, source := range claim.Sources {
			if source.ID != "" {
				citedEvidence[source.ID] = struct{}{}
			}
		}
	}

	findings := make([]string, 0)
	for _, claim := range scan.Claims {
		for _, id := range claim.SupportingClaimIDs {
			if _, ok := claimIDs[id]; !ok {
				findings = append(findings, fmt.Sprintf("claim %s supporting_claim_ids references missing %s", claim.ID, id))
			}
		}
		for _, id := range claim.ConflictsWith {
			if _, ok := claimIDs[id]; !ok {
				findings = append(findings, fmt.Sprintf("claim %s conflicts_with references missing %s", claim.ID, id))
			}
		}
		for _, id := range claim.EvidenceIDs {
			if _, err := (EvidenceStore{Paths: paths}).Read(workspace, id); err != nil {
				findings = append(findings, fmt.Sprintf("claim %s evidence_ids references missing %s", claim.ID, id))
			}
		}
		if claim.Status == ClaimStatusApproved && claim.StaleAfter != "" {
			staleAfter, err := time.Parse(time.RFC3339, claim.StaleAfter)
			if err == nil && staleAfter.Before(now) {
				findings = append(findings, fmt.Sprintf("claim %s stale_after %s is in the past", claim.ID, claim.StaleAfter))
			}
		}
	}

	evidenceIDs, shaByID, err := listEvidenceSnapshots(paths, workspace)
	if err != nil {
		return nil, err
	}
	for _, id := range evidenceIDs {
		if _, ok := citedEvidence[id]; !ok {
			findings = append(findings, fmt.Sprintf("evidence %s is not cited by any claim", id))
		}
	}
	shaIDs := make(map[string][]string)
	for id, sha := range shaByID {
		if sha == "" {
			continue
		}
		shaIDs[sha] = append(shaIDs[sha], id)
	}
	hashes := make([]string, 0, len(shaIDs))
	for sha, ids := range shaIDs {
		if len(ids) > 1 {
			hashes = append(hashes, sha)
		}
	}
	sort.Strings(hashes)
	for _, sha := range hashes {
		ids := shaIDs[sha]
		sort.Strings(ids)
		findings = append(findings, fmt.Sprintf("duplicate sha256 %s on evidence %s", sha, strings.Join(ids, ", ")))
	}

	sort.Strings(findings)
	return findings, nil
}

func listEvidenceSnapshots(paths Paths, workspace string) ([]string, map[string]string, error) {
	sourcesDir, err := ResolveWorkspacePath(paths, workspace, filepath.ToSlash(filepath.Join("evidence", "sources")))
	if err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(sourcesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, map[string]string{}, nil
		}
		return nil, nil, err
	}
	ids := make([]string, 0, len(entries))
	shaByID := make(map[string]string, len(entries))
	store := EvidenceStore{Paths: paths}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !evidenceIDPattern.MatchString(name) {
			continue
		}
		ids = append(ids, name)
		evidence, err := store.Read(workspace, name)
		if err != nil {
			continue
		}
		shaByID[name] = evidence.SHA256
	}
	sort.Strings(ids)
	return ids, shaByID, nil
}
