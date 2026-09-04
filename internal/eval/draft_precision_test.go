package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

// TestEvalDraftPrecisionGolden is the eval-suite draft-precision golden run
// (docs/eval/draft-precision.md). A fixture campaign produces evidence-bound
// drafts with a deterministic clock, mock judge verdicts are imported as
// data (zbrain core runs zero model calls), the import format is validated
// fail-closed, the metric is recomputed, and the golden artifact is written
// to docs/proofs/eval-draft-precision-golden.json.
func TestEvalDraftPrecisionGolden(t *testing.T) {
	e := newEvalEnv(t)

	// 1. Fixture: one immutable evidence snapshot the drafts must trace to.
	source := filepath.Join(t.TempDir(), "campaign-evidence.txt")
	const evidenceText = "The ingestion pipeline flushes its buffer every 30 seconds under normal load.\n"
	if err := os.WriteFile(source, []byte(evidenceText), 0o644); err != nil {
		t.Fatalf("WriteFile(evidence source) error = %v", err)
	}
	evidence, err := (zruntime.EvidenceStore{Paths: e.paths, Now: e.now}).AddFile("research", source, "file://campaign-evidence", "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}

	// 2. Campaign: three evidence-bound draft specs, deterministic clock.
	specs := []zruntime.CampaignSpec{
		{Tier: "projects", Title: "Ingestion Flush Cadence", Basis: "evidence", EvidenceIDs: []string{evidence.ID}},
		{Tier: "projects", Title: "Flush Cadence Detail", Basis: "evidence", EvidenceIDs: []string{evidence.ID}},
		{Tier: "projects", Title: "Flush Cadence Speculation", Basis: "evidence", EvidenceIDs: []string{evidence.ID}},
	}
	store := zruntime.CampaignStore{Paths: e.paths, Now: e.now}
	run, err := store.BeginCampaign("research", specs)
	if err != nil {
		t.Fatalf("BeginCampaign() error = %v", err)
	}
	bodies := []string{
		"The ingestion pipeline flushes its buffer every 30 seconds under normal load.\n",
		"The pipeline flushes every 30 seconds; the flush window is bounded by normal load.\n",
		"The pipeline flushes every 5 minutes and reorders events during the window.\n",
	}
	claimIDs := make([]string, 0, len(bodies))
	for index, body := range bodies {
		submission, err := store.SubmitCampaignDraft("research", run.RunID, index, body)
		if err != nil {
			t.Fatalf("SubmitCampaignDraft(%d) error = %v", index, err)
		}
		claimIDs = append(claimIDs, submission.ClaimID)
	}
	state, err := store.ResumeCampaign("research", run.RunID)
	if err != nil {
		t.Fatalf("ResumeCampaign() error = %v", err)
	}
	if state.Submitted != len(bodies) || state.Pending != 0 {
		t.Fatalf("campaign state = %#v, want all drafts submitted", state)
	}
	for _, id := range claimIDs {
		claim, err := (zruntime.ClaimStore{Paths: e.paths, Now: e.now}).Read("research", id)
		if err != nil {
			t.Fatalf("Read(%s) error = %v", id, err)
		}
		if claim.Status != zruntime.ClaimStatusDraft {
			t.Fatalf("campaign claim %s status = %q, want draft (campaigns never approve)", id, claim.Status)
		}
	}

	// 3. Mock judge verdicts imported as data (the judge runs outside; no
	// model call happens in this process).
	type verdict struct {
		DraftIndex         int     `json:"draft_index"`
		Title              string  `json:"title"`
		BodySHA256         string  `json:"body_sha256"`
		BoundEvidenceCount int     `json:"bound_evidence_count"`
		Verdict            string  `json:"verdict"`
		TraceScore         float64 `json:"trace_score"`
		Rationale          string  `json:"rationale"`
	}
	verdicts := []verdict{
		{DraftIndex: 0, Title: specs[0].Title, Verdict: "supported", TraceScore: 1.0, Rationale: "body restates the evidence snapshot verbatim"},
		{DraftIndex: 1, Title: specs[1].Title, Verdict: "supported", TraceScore: 0.9, Rationale: "body paraphrases the evidence flush cadence"},
		{DraftIndex: 2, Title: specs[2].Title, Verdict: "unverified", TraceScore: 0.4, Rationale: "five-minute cadence and reordering claim is not in the evidence"},
	}
	const threshold = 0.7
	for index := range verdicts {
		sum := sha256.Sum256([]byte(bodies[verdicts[index].DraftIndex]))
		verdicts[index].BodySHA256 = hex.EncodeToString(sum[:])
		verdicts[index].BoundEvidenceCount = len(specs[verdicts[index].DraftIndex].EvidenceIDs)
	}

	// 4. Validate the import fail-closed, then recompute the metric.
	type importDoc struct {
		Schema string `json:"schema"`
		Judge  struct {
			Threshold float64 `json:"threshold"`
		} `json:"judge"`
		Verdicts []verdict `json:"verdicts"`
		Metrics  struct {
			Total          int     `json:"total"`
			Supported      int     `json:"supported"`
			DraftPrecision float64 `json:"draft_precision"`
		} `json:"metrics"`
	}
	validate := func(doc importDoc) error {
		if doc.Schema != "zbrain.eval.draft-precision/v1" {
			return fmt.Errorf("schema %q must be zbrain.eval.draft-precision/v1", doc.Schema)
		}
		if len(doc.Verdicts) == 0 {
			return fmt.Errorf("verdicts must not be empty")
		}
		supported := 0
		for index, v := range doc.Verdicts {
			if v.DraftIndex != index {
				return fmt.Errorf("verdict %d has draft_index %d; verdicts must be contiguous from 0", index, v.DraftIndex)
			}
			switch v.Verdict {
			case "supported", "unverified", "contradicted":
			default:
				return fmt.Errorf("verdict %d has unsupported verdict %q", index, v.Verdict)
			}
			if v.TraceScore < 0 || v.TraceScore > 1 {
				return fmt.Errorf("verdict %d trace_score %v outside [0,1]", index, v.TraceScore)
			}
			if v.Verdict == "supported" && v.TraceScore < doc.Judge.Threshold {
				return fmt.Errorf("verdict %d supported with trace_score %v below threshold %v", index, v.TraceScore, doc.Judge.Threshold)
			}
			if v.Verdict == "supported" {
				supported++
			}
			if v.BoundEvidenceCount == 0 && v.Verdict == "supported" {
				return fmt.Errorf("verdict %d supported with no bound evidence", index)
			}
		}
		if doc.Metrics.Total != len(doc.Verdicts) || doc.Metrics.Supported != supported {
			return fmt.Errorf("metrics totals = %d/%d, want %d/%d", doc.Metrics.Supported, doc.Metrics.Total, supported, len(doc.Verdicts))
		}
		want := 0.0
		if len(doc.Verdicts) > 0 {
			want = float64(supported) / float64(len(doc.Verdicts))
		}
		if math.Abs(doc.Metrics.DraftPrecision-want) > 1e-9 {
			return fmt.Errorf("draft_precision %v, want %v", doc.Metrics.DraftPrecision, want)
		}
		return nil
	}

	recompute := func() importDoc {
		doc := importDoc{Schema: "zbrain.eval.draft-precision/v1"}
		doc.Judge.Threshold = threshold
		supported := 0
		for _, v := range verdicts {
			doc.Verdicts = append(doc.Verdicts, v)
			if v.Verdict == "supported" {
				supported++
			}
		}
		doc.Metrics.Total = len(verdicts)
		doc.Metrics.Supported = supported
		doc.Metrics.DraftPrecision = float64(supported) / float64(len(verdicts))
		if err := validate(doc); err != nil {
			t.Fatalf("recomputed golden import is invalid: %v", err)
		}
		return doc
	}

	golden := recompute()

	// 5. Content identity: every verdict must match the live fixture draft
	// it claims to judge — unknown or mismatched verdicts fail closed.
	for _, v := range golden.Verdicts {
		if v.DraftIndex < 0 || v.DraftIndex >= len(specs) {
			t.Fatalf("verdict draft_index %d out of range", v.DraftIndex)
		}
		if v.Title != specs[v.DraftIndex].Title {
			t.Fatalf("verdict %d title %q does not match fixture spec title %q", v.DraftIndex, v.Title, specs[v.DraftIndex].Title)
		}
		sum := sha256.Sum256([]byte(bodies[v.DraftIndex]))
		if v.BodySHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("verdict %d body_sha256 does not match the fixture draft body", v.DraftIndex)
		}
		if v.BoundEvidenceCount != len(specs[v.DraftIndex].EvidenceIDs) {
			t.Fatalf("verdict %d bound_evidence_count = %d, want %d", v.DraftIndex, v.BoundEvidenceCount, len(specs[v.DraftIndex].EvidenceIDs))
		}
	}

	// 6. Fail-closed validation on tampered imports (in memory only).
	tampered := golden
	tampered.Verdicts = append(tampered.Verdicts, verdict{DraftIndex: 3, Title: "Unknown Draft", Verdict: "supported", TraceScore: 1.0})
	tampered.Metrics.Total = len(tampered.Verdicts)
	tampered.Metrics.Supported++
	tampered.Metrics.DraftPrecision = float64(tampered.Metrics.Supported) / float64(tampered.Metrics.Total)
	if err := validate(tampered); err == nil {
		t.Fatal("unknown draft_index accepted; import validation must fail closed")
	}
	badVerdict := golden
	badVerdict.Verdicts[2].Verdict = "supported"
	if err := validate(badVerdict); err == nil {
		t.Fatal("supported-below-threshold accepted; import validation must fail closed")
	}
	badMetrics := golden
	badMetrics.Metrics.DraftPrecision = 1.0
	if err := validate(badMetrics); err == nil {
		t.Fatal("mismatched metrics accepted; import validation must fail closed")
	}

	// 7. Write the golden artifact (deterministic: no IDs, timestamps, or paths).
	payloadVerdicts := make([]map[string]any, 0, len(golden.Verdicts))
	for _, v := range golden.Verdicts {
		payloadVerdicts = append(payloadVerdicts, map[string]any{
			"draft_index":          v.DraftIndex,
			"title":                v.Title,
			"body_sha256":          v.BodySHA256,
			"bound_evidence_count": v.BoundEvidenceCount,
			"verdict":              v.Verdict,
			"trace_score":          v.TraceScore,
			"rationale":            v.Rationale,
		})
	}
	writeEvalProof(t, "eval-draft-precision-golden.json", map[string]any{
		"schema":   "zbrain.eval.draft-precision/v1",
		"run":      map[string]any{"workspace": "research", "run_label": "golden", "created_at": evalFixedNow().UTC().Format("2006-01-02T15:04:05Z")},
		"judge":    map[string]any{"name": "mock-judge-fixture", "model": "none (imported verdicts)", "rubric_version": 1, "threshold": threshold},
		"verdicts": payloadVerdicts,
		"metrics":  golden.Metrics,
	})

	// The golden metric itself: 2 of 3 drafts materially trace to evidence.
	if golden.Metrics.Supported != 2 || golden.Metrics.Total != 3 {
		t.Fatalf("golden metrics = %#v, want supported=2 total=3", golden.Metrics)
	}
}
