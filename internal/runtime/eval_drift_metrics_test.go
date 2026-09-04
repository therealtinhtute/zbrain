package runtime

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestEvalDriftMetrics is the eval-suite drift-metrics runner. It builds a
// four-item evidence fixture (unchanged, mutated, missing origin,
// non-local/uncheckable origin), runs EvidenceStore.CheckDrift, computes the
// precision/recall of drift findings against the mutation ground truth, and
// proves evidence check stays read-only (snapshot bytes and mtimes
// unchanged). Results land in docs/proofs/eval-drift-metrics.json.
func TestEvalDriftMetrics(t *testing.T) {
	paths := queryTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedQueryNow}
	evidenceStore := EvidenceStore{Paths: paths, Now: fixedQueryNow}

	// Origins live in their own temp directory, mutated/deleted by the test;
	// the runtime workspace copies stay untouched.
	origins := t.TempDir()
	type item struct {
		name     string
		origin   string
		expected EvidenceDriftStatus // ground truth for the drift classifier
	}
	items := []item{
		{name: "unchanged", expected: EvidenceDriftUnchanged},
		{name: "mutated", expected: EvidenceDriftChanged},
		{name: "missing", expected: EvidenceDriftMissing},
		{name: "uncheckable", expected: EvidenceDriftUncheckable},
	}

	addEvidence := func(t *testing.T, it item, content []byte) Evidence {
		t.Helper()
		source := filepath.Join(origins, it.name+".txt")
		if err := os.WriteFile(source, content, 0o644); err != nil {
			t.Fatalf("WriteFile(%s origin) error = %v", it.name, err)
		}
		origin := source
		if it.expected == EvidenceDriftUncheckable {
			origin = "https://example.invalid/" + it.name
		}
		evidence, err := evidenceStore.AddFile("research", source, origin, "text/plain")
		if err != nil {
			t.Fatalf("AddFile(%s) error = %v", it.name, err)
		}
		return evidence
	}

	claimIDs := map[string]string{} // fixture item name -> binding claim ID
	for _, it := range items {
		content := []byte("drift fixture content for " + it.name + "\n")
		evidence := addEvidence(t, it, content)
		claim := queryClaim(newDriftClaimID(t), "Drift Fixture "+it.name, ClaimBasisEvidence)
		claim.EvidenceIDs = []string{evidence.ID}
		claim.Body = "drift fixture body " + it.name + "\n"
		if _, err := store.WriteDraft("research", claim); err != nil {
			t.Fatalf("WriteDraft(%s) error = %v", it.name, err)
		}
		if _, err := store.Approve("research", claim.ID); err != nil {
			t.Fatalf("Approve(%s) error = %v", it.name, err)
		}
		claimIDs[it.name] = claim.ID
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	// Apply the ground-truth mutations to the origins (never to snapshots).
	mutatedOrigin := filepath.Join(origins, "mutated.txt")
	if err := os.WriteFile(mutatedOrigin, []byte("drift fixture content for mutated — TAMPERED\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(mutated origin) error = %v", err)
	}
	if err := os.Remove(filepath.Join(origins, "missing.txt")); err != nil {
		t.Fatalf("Remove(missing origin) error = %v", err)
	}

	// Read-only proof: capture snapshot bytes + mtimes before the check.
	type snapshotState struct {
		path  string
		size  int64
		mtime int64
		mode  os.FileMode
	}
	capture := func(t *testing.T) []snapshotState {
		t.Helper()
		root := filepath.Join(paths.WorkspacesDir, "research", "evidence")
		var states []snapshotState
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			states = append(states, snapshotState{path: path, size: info.Size(), mtime: info.ModTime().UnixNano(), mode: info.Mode()})
			return nil
		})
		if err != nil {
			t.Fatalf("Walk(evidence snapshots) error = %v", err)
		}
		return states
	}
	before := capture(t)

	report, err := evidenceStore.CheckDrift("research")
	if err != nil {
		t.Fatalf("CheckDrift() error = %v", err)
	}
	after := capture(t)
	if len(before) != len(after) {
		t.Fatalf("evidence file count changed during CheckDrift: %d → %d (read-only violation)", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("CheckDrift mutated %s: %#v → %#v (read-only violation)", before[i].path, before[i], after[i])
		}
	}

	// Score the findings against ground truth.
	byItem := map[string]EvidenceDriftFinding{}
	for _, finding := range report.Findings {
		matched := false
		for _, it := range items {
			evidence, err := evidenceStore.Read("research", finding.ID)
			if err != nil {
				t.Fatalf("Read(%s) error = %v", finding.ID, err)
			}
			if it.expected == EvidenceDriftUncheckable {
				if evidence.Origin == "https://example.invalid/"+it.name {
					byItem[it.name] = finding
					matched = true
				}
				continue
			}
			if evidence.Origin == filepath.Join(origins, it.name+".txt") {
				byItem[it.name] = finding
				matched = true
			}
		}
		if !matched {
			t.Fatalf("unexpected finding %q (origin %q)", finding.ID, finding.Origin)
		}
	}
	if len(report.Findings) != len(items) {
		t.Fatalf("findings = %d, want %d", len(report.Findings), len(items))
	}

	truePositives, falsePositives, falseNegatives := 0, 0, 0
	perItem := make([]map[string]any, 0, len(items))
	for _, it := range items {
		finding, ok := byItem[it.name]
		if !ok {
			t.Fatalf("no finding for fixture item %q", it.name)
		}
		if finding.Status != it.expected {
			t.Fatalf("finding %q status = %q, want %q", it.name, finding.Status, it.expected)
		}
		drifted := it.expected == EvidenceDriftChanged || it.expected == EvidenceDriftMissing
		predicted := finding.Status == EvidenceDriftChanged || finding.Status == EvidenceDriftMissing
		switch {
		case drifted && predicted:
			truePositives++
		case !drifted && predicted:
			falsePositives++
		case drifted && !predicted:
			falseNegatives++
		}
		record := map[string]any{
			"item":     it.name,
			"expected": string(it.expected),
			"observed": string(finding.Status),
		}
		if len(finding.AffectedClaimIDs) > 0 {
			if finding.AffectedClaimIDs[0] != claimIDs[it.name] {
				t.Fatalf("finding %q affected claims = %v, want [%s]", it.name, finding.AffectedClaimIDs, claimIDs[it.name])
			}
			record["affected_claim_ids"] = finding.AffectedClaimIDs
		}
		perItem = append(perItem, record)
	}
	sort.Slice(perItem, func(i, j int) bool { return perItem[i]["item"].(string) < perItem[j]["item"].(string) })

	predicted := truePositives + falsePositives
	truth := truePositives + falseNegatives
	precision := 1.0
	if predicted > 0 {
		precision = float64(truePositives) / float64(predicted)
	}
	recall := 1.0
	if truth > 0 {
		recall = float64(truePositives) / float64(truth)
	}
	if precision < 1.0 || recall < 1.0 {
		t.Fatalf("drift metrics precision=%.3f recall=%.3f (tp=%d fp=%d fn=%d), want perfect on this fixture", precision, recall, truePositives, falsePositives, falseNegatives)
	}

	writeProofJSON(t, "eval-drift-metrics.json", map[string]any{
		"schema":       "zbrain.eval.drift-metrics/v1",
		"fixture_size": len(items),
		"metrics": map[string]any{
			"precision":       round4(precision),
			"recall":          round4(recall),
			"true_positives":  truePositives,
			"false_positives": falsePositives,
			"false_negatives": falseNegatives,
			"note":            "uncheckable origins are excluded from the drift prediction set; unchanged is a true negative",
		},
		"findings": perItem,
		"read_only_proof": map[string]any{
			"files_inspected":           len(before),
			"bytes_and_mtime_unchanged": true,
		},
	})
}

func newDriftClaimID(t *testing.T) string {
	t.Helper()
	id, err := NewClaimID()
	if err != nil {
		t.Fatalf("NewClaimID() error = %v", err)
	}
	return id
}
