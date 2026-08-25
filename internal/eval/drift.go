//go:build ignore

package main

// drift.go — eval drift harness per docs/plans/active/zbrain-optimization-plan.md §12.3 C1
//
// Compare two eval JSON files (--before baseline.json --after new.json),
// compute ΔP/R and McNemar-like paired difference.
// For each query, compare hits before vs after, compute McNemar chi2 if possible, or simple delta.
// Output markdown table and JSON. Flag drift if ΔP/R >5%.
//
// Usage:
//   go run ./internal/eval/drift.go --before docs/proofs/eval-baseline.json --after /tmp/new.json
//   go run ./internal/eval/drift.go --before docs/proofs/eval-baseline.json --after /tmp/new.json --json /tmp/drift.json
//   go run ./internal/eval/drift.go --before a.json --after b.json --threshold 0.05
//
// Note: this file has `//go:build ignore` so `go vet ./internal/eval/...` stays green
// despite two package main files in the same directory. `go run ./internal/eval/drift.go`
// runs this file explicitly and ignores the build tag.

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Reuse evalResult shape from internal/eval/eval.go (duplicated for standalone build).

type evalResult struct {
	CorpusSize   int       `json:"corpus_size"`
	Workspace    string    `json:"workspace"`
	Limit        int       `json:"limit"`
	TotalQueries int       `json:"total_queries"`
	PrecisionAtK float64   `json:"precision_at_k"`
	RecallAtK    float64   `json:"recall_at_k"`
	F1AtK        float64   `json:"f1_at_k"`
	MRR          float64   `json:"mrr"`
	NDCGAtK      float64   `json:"ndcg_at_k"`
	MAPAtK       float64   `json:"map_at_k"`
	GapRate      float64   `json:"gap_rate"`
	BlockedRate  float64   `json:"blocked_rate"`
	Faithfulness float64   `json:"faithfulness"`
	Queries      []perQuery `json:"queries,omitempty"`
}

type perQuery struct {
	ID        string  `json:"id"`
	Text      string  `json:"text"`
	Precision float64 `json:"precision_at_k"`
	Recall    float64 `json:"recall_at_k"`
	MRR       float64 `json:"mrr"`
	NDCG      float64 `json:"ndcg_at_k"`
	AP        float64 `json:"ap_at_k"`
	Relevant  int     `json:"relevant_total"`
	Retrieved int     `json:"retrieved"`
	Hits      int     `json:"hits"`
	Gap       bool    `json:"gap"`
	Blocked   bool    `json:"blocked"`
	RankFirst int     `json:"rank_first"`
}

type driftDelta struct {
	Precision    float64 `json:"precision_delta"`
	Recall       float64 `json:"recall_delta"`
	F1           float64 `json:"f1_delta"`
	MRR          float64 `json:"mrr_delta"`
	NDCG         float64 `json:"ndcg_delta"`
	MAP          float64 `json:"map_delta"`
	GapRate      float64 `json:"gap_delta"`
	BlockedRate  float64 `json:"blocked_delta"`
	Faithfulness float64 `json:"faithfulness_delta"`
}

type mcnemarResult struct {
	A           int     `json:"a_both_hit"`
	B           int     `json:"b_before_only"`
	C           int     `json:"c_after_only"`
	D           int     `json:"d_both_miss"`
	Total       int     `json:"total_paired"`
	Discordant  int     `json:"discordant"`
	Chi2        float64 `json:"chi2"`
	Chi2CC      float64 `json:"chi2_cc"`
	PValue      float64 `json:"p_value"`
	PValueCC    float64 `json:"p_value_cc"`
	Significant bool    `json:"significant_p_lt_0_05"`
	Feasible    bool    `json:"feasible"`
	Note        string  `json:"note,omitempty"`
}

type perQueryDelta struct {
	ID              string  `json:"id"`
	Text            string  `json:"text"`
	BeforePrecision float64 `json:"before_precision"`
	AfterPrecision  float64 `json:"after_precision"`
	DeltaPrecision  float64 `json:"delta_precision"`
	BeforeRecall    float64 `json:"before_recall"`
	AfterRecall     float64 `json:"after_recall"`
	DeltaRecall     float64 `json:"delta_recall"`
	BeforeMRR       float64 `json:"before_mrr"`
	AfterMRR        float64 `json:"after_mrr"`
	DeltaMRR        float64 `json:"delta_mrr"`
	BeforeNDCG      float64 `json:"before_ndcg"`
	AfterNDCG       float64 `json:"after_ndcg"`
	DeltaNDCG       float64 `json:"delta_ndcg"`
	BeforeAP        float64 `json:"before_ap"`
	AfterAP         float64 `json:"after_ap"`
	DeltaAP         float64 `json:"delta_ap"`
	BeforeHits      int     `json:"before_hits"`
	AfterHits       int     `json:"after_hits"`
	DeltaHits       int     `json:"delta_hits"`
	BeforeGap       bool    `json:"before_gap"`
	AfterGap        bool    `json:"after_gap"`
	BeforeBlocked   bool    `json:"before_blocked"`
	AfterBlocked    bool    `json:"after_blocked"`
	BeforeRankFirst int     `json:"before_rank_first"`
	AfterRankFirst  int     `json:"after_rank_first"`
}

type driftOutput struct {
	Before         string          `json:"before"`
	After          string          `json:"after"`
	Threshold      float64         `json:"threshold"`
	BeforeSummary  evalResult      `json:"before_summary"`
	AfterSummary   evalResult      `json:"after_summary"`
	Delta          driftDelta      `json:"delta"`
	Drift          bool            `json:"drift"`
	DriftReason    string          `json:"drift_reason,omitempty"`
	McNemar        mcnemarResult   `json:"mcnemar"`
	PerQuery       []perQueryDelta `json:"per_query"`
	UnpairedBefore []string        `json:"unpaired_before,omitempty"`
	UnpairedAfter  []string        `json:"unpaired_after,omitempty"`
}

func main() {
	var beforePath, afterPath, jsonPath string
	var threshold float64
	var help bool

	flag.StringVar(&beforePath, "before", "", "path to baseline eval JSON (required)")
	flag.StringVar(&afterPath, "after", "", "path to new eval JSON (required)")
	flag.StringVar(&jsonPath, "json", "", "write JSON drift report to file (optional, also printed to stdout)")
	flag.Float64Var(&threshold, "threshold", 0.05, "drift threshold for |ΔP| or |ΔR| (default 0.05 = 5%)")
	flag.BoolVar(&help, "help", false, "show help")

	// Also support -h
	var h bool
	flag.BoolVar(&h, "h", false, "show help (shorthand)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "drift — compare two eval JSONs and flag retrieval drift\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  go run ./internal/eval/drift.go --before docs/proofs/eval-baseline.json --after /tmp/new.json [--json /tmp/drift.json] [--threshold 0.05]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nOutputs markdown table to stdout and JSON to --json file or stdout.\n")
		fmt.Fprintf(os.Stderr, "Flag drift if |ΔP| > threshold or |ΔR| > threshold (default 5%%).\n")
		fmt.Fprintf(os.Stderr, "McNemar chi2 is computed on paired per-query hit/miss (hits>0 = hit).\n")
		fmt.Fprintf(os.Stderr, "If discordant pairs b+c < 1, chi2 is not feasible and simple delta is used.\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  go run ./internal/eval/drift.go --before docs/proofs/eval-baseline.json --after /tmp/new.json\n")
		fmt.Fprintf(os.Stderr, "  go run ./internal/eval/drift.go --before a.json --after b.json --json /tmp/drift.json\n")
	}
	flag.Parse()

	if help || h {
		flag.Usage()
		os.Exit(0)
	}
	if beforePath == "" || afterPath == "" {
		fmt.Fprintf(os.Stderr, "error: --before and --after are required\n\n")
		flag.Usage()
		os.Exit(2)
	}
	if threshold < 0 || threshold > 1 {
		fmt.Fprintf(os.Stderr, "error: --threshold must be in [0,1]\n")
		os.Exit(2)
	}

	beforeRes, err := loadEval(beforePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load --before %q: %v\n", beforePath, err)
		os.Exit(1)
	}
	afterRes, err := loadEval(afterPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load --after %q: %v\n", afterPath, err)
		os.Exit(1)
	}

	delta := driftDelta{
		Precision:    afterRes.PrecisionAtK - beforeRes.PrecisionAtK,
		Recall:       afterRes.RecallAtK - beforeRes.RecallAtK,
		F1:           afterRes.F1AtK - beforeRes.F1AtK,
		MRR:          afterRes.MRR - beforeRes.MRR,
		NDCG:         afterRes.NDCGAtK - beforeRes.NDCGAtK,
		MAP:          afterRes.MAPAtK - beforeRes.MAPAtK,
		GapRate:      afterRes.GapRate - beforeRes.GapRate,
		BlockedRate:  afterRes.BlockedRate - beforeRes.BlockedRate,
		Faithfulness: afterRes.Faithfulness - beforeRes.Faithfulness,
	}

	drift := false
	var driftReasons []string
	if math.Abs(delta.Precision) > threshold {
		drift = true
		driftReasons = append(driftReasons, fmt.Sprintf("ΔP=%.3f > %.1f%%", delta.Precision, threshold*100))
	}
	if math.Abs(delta.Recall) > threshold {
		drift = true
		driftReasons = append(driftReasons, fmt.Sprintf("ΔR=%.3f > %.1f%%", delta.Recall, threshold*100))
	}
	driftReason := strings.Join(driftReasons, "; ")

	// Build per-query maps keyed by ID.
	beforeMap := make(map[string]perQuery, len(beforeRes.Queries))
	for _, q := range beforeRes.Queries {
		beforeMap[q.ID] = q
	}
	afterMap := make(map[string]perQuery, len(afterRes.Queries))
	for _, q := range afterRes.Queries {
		afterMap[q.ID] = q
	}

	// Intersection sorted.
	allIDs := make(map[string]bool)
	for id := range beforeMap {
		allIDs[id] = true
	}
	for id := range afterMap {
		allIDs[id] = true
	}
	// Only paired IDs are those present in both.
	var pairedIDs []string
	var unpairedBefore, unpairedAfter []string
	for id := range allIDs {
		_, inB := beforeMap[id]
		_, inA := afterMap[id]
		if inB && inA {
			pairedIDs = append(pairedIDs, id)
		} else if inB {
			unpairedBefore = append(unpairedBefore, id)
		} else {
			unpairedAfter = append(unpairedAfter, id)
		}
	}
	sort.Strings(pairedIDs)
	sort.Strings(unpairedBefore)
	sort.Strings(unpairedAfter)

	perDeltas := make([]perQueryDelta, 0, len(pairedIDs))
	// McNemar counts: binary hit = hits>0 (or !Gap && hits>0). Use hits>0 as success.
	// This matches "compare hits before vs after".
	var a, b, c, d int
	for _, id := range pairedIDs {
		bq := beforeMap[id]
		aq := afterMap[id]
		pd := perQueryDelta{
			ID:              id,
			Text:            bq.Text,
			BeforePrecision: bq.Precision,
			AfterPrecision:  aq.Precision,
			DeltaPrecision:  aq.Precision - bq.Precision,
			BeforeRecall:    bq.Recall,
			AfterRecall:     aq.Recall,
			DeltaRecall:     aq.Recall - bq.Recall,
			BeforeMRR:       bq.MRR,
			AfterMRR:        aq.MRR,
			DeltaMRR:        aq.MRR - bq.MRR,
			BeforeNDCG:      bq.NDCG,
			AfterNDCG:       aq.NDCG,
			DeltaNDCG:       aq.NDCG - bq.NDCG,
			BeforeAP:        bq.AP,
			AfterAP:         aq.AP,
			DeltaAP:         aq.AP - bq.AP,
			BeforeHits:      bq.Hits,
			AfterHits:       aq.Hits,
			DeltaHits:       aq.Hits - bq.Hits,
			BeforeGap:       bq.Gap,
			AfterGap:        aq.Gap,
			BeforeBlocked:   bq.Blocked,
			AfterBlocked:    aq.Blocked,
			BeforeRankFirst: bq.RankFirst,
			AfterRankFirst:  aq.RankFirst,
		}
		if pd.Text == "" {
			pd.Text = aq.Text
		}
		perDeltas = append(perDeltas, pd)

		beforeHit := bq.Hits > 0
		afterHit := aq.Hits > 0
		// Alternative: consider gap? hits>0 already implies !gap, but keep hits>0 as primary.
		switch {
		case beforeHit && afterHit:
			a++
		case beforeHit && !afterHit:
			b++
		case !beforeHit && afterHit:
			c++
		case !beforeHit && !afterHit:
			d++
		}
	}

	mc := mcnemarResult{
		A:          a,
		B:          b,
		C:          c,
		D:          d,
		Total:      len(pairedIDs),
		Discordant: b + c,
	}
	if b+c > 0 {
		mc.Feasible = true
		// Standard McNemar without continuity correction.
		mc.Chi2 = math.Pow(float64(b-c), 2) / float64(b+c)
		mc.PValue = chi2PValue1df(mc.Chi2)
		// With continuity correction (|b-c|-1)^2/(b+c)
		diff := math.Abs(float64(b - c))
		if diff > 0 {
			mc.Chi2CC = math.Pow(diff-1, 2) / float64(b+c)
		} else {
			mc.Chi2CC = 0
		}
		mc.PValueCC = chi2PValue1df(mc.Chi2CC)
		mc.Significant = mc.PValue < 0.05 || mc.PValueCC < 0.05
		if mc.Significant {
			mc.Note = "significant paired difference (p<0.05)"
		} else {
			mc.Note = "no significant paired difference (p>=0.05)"
		}
	} else {
		mc.Feasible = false
		if len(pairedIDs) == 0 {
			mc.Note = "no paired queries; per-query delta only"
		} else {
			mc.Note = "no discordant pairs (b+c=0); chi2 not computable, use simple delta"
		}
		mc.Chi2 = 0
		mc.Chi2CC = 0
		mc.PValue = 1
		mc.PValueCC = 1
	}

	out := driftOutput{
		Before:         beforePath,
		After:          afterPath,
		Threshold:      threshold,
		BeforeSummary:  beforeRes,
		AfterSummary:   afterRes,
		Delta:          delta,
		Drift:          drift,
		DriftReason:    driftReason,
		McNemar:        mc,
		PerQuery:       perDeltas,
		UnpairedBefore: unpairedBefore,
		UnpairedAfter:  unpairedAfter,
	}

	// Write markdown to stdout.
	md := renderMarkdown(out)
	fmt.Print(md)

	// Write JSON.
	jsonData, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal json: %v\n", err)
		os.Exit(1)
	}
	if jsonPath != "" {
		if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %q: %v\n", filepath.Dir(jsonPath), err)
			os.Exit(1)
		}
		if err := os.WriteFile(jsonPath, jsonData, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write %q: %v\n", jsonPath, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "json written to %s\n", jsonPath)
		// Also echo JSON to stdout after markdown separator for convenience when checking.
		// But keep markdown as primary stdout; JSON file is for machine parsing.
	} else {
		// No file requested: print JSON after markdown separator.
		fmt.Println("\n--- JSON ---")
		fmt.Println(string(jsonData))
	}

	// Drift flag to stderr for visibility.
	if drift {
		fmt.Fprintf(os.Stderr, "drift: DETECTED | ΔP=%.3f ΔR=%.3f threshold=%.1f%% reason: %s | McNemar chi2=%.3f p=%.3f feasible=%v\n",
			delta.Precision, delta.Recall, threshold*100, driftReason, mc.Chi2, mc.PValue, mc.Feasible)
	} else {
		fmt.Fprintf(os.Stderr, "drift: none | ΔP=%.3f ΔR=%.3f threshold=%.1f%% | McNemar chi2=%.3f p=%.3f feasible=%v\n",
			delta.Precision, delta.Recall, threshold*100, mc.Chi2, mc.PValue, mc.Feasible)
	}
}

func loadEval(path string) (evalResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return evalResult{}, err
	}
	var r evalResult
	if err := json.Unmarshal(data, &r); err != nil {
		return evalResult{}, fmt.Errorf("json unmarshal: %w", err)
	}
	return r, nil
}

// chi2PValue1df returns p-value for chi-squared with 1 degree of freedom.
// For df=1, CDF = erf(sqrt(chi2/2)), so p = 1 - CDF = erfc(sqrt(chi2/2)).
func chi2PValue1df(chi2 float64) float64 {
	if chi2 <= 0 {
		return 1.0
	}
	return math.Erfc(math.Sqrt(chi2 / 2))
}

func renderMarkdown(o driftOutput) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Drift Report")
	fmt.Fprintln(&b, "")
	fmt.Fprintf(&b, "- **Before:** `%s` (corpus=%d, P@%d=%.3f, R@%d=%.3f, F1=%.3f, MRR=%.3f, NDCG=%.3f, MAP=%.3f, gap=%.1f%%)\n",
		o.Before, o.BeforeSummary.CorpusSize, o.BeforeSummary.Limit, o.BeforeSummary.PrecisionAtK, o.BeforeSummary.Limit, o.BeforeSummary.RecallAtK, o.BeforeSummary.F1AtK, o.BeforeSummary.MRR, o.BeforeSummary.NDCGAtK, o.BeforeSummary.MAPAtK, o.BeforeSummary.GapRate*100)
	fmt.Fprintf(&b, "- **After:** `%s` (corpus=%d, P@%d=%.3f, R@%d=%.3f, F1=%.3f, MRR=%.3f, NDCG=%.3f, MAP=%.3f, gap=%.1f%%)\n",
		o.After, o.AfterSummary.CorpusSize, o.AfterSummary.Limit, o.AfterSummary.PrecisionAtK, o.AfterSummary.Limit, o.AfterSummary.RecallAtK, o.AfterSummary.F1AtK, o.AfterSummary.MRR, o.AfterSummary.NDCGAtK, o.AfterSummary.MAPAtK, o.AfterSummary.GapRate*100)
	fmt.Fprintf(&b, "- **Threshold:** |ΔP| or |ΔR| > %.1f%%\n", o.Threshold*100)
	if o.Drift {
		fmt.Fprintf(&b, "- **Drift:** ⚠️ **DETECTED** — %s\n", o.DriftReason)
	} else {
		fmt.Fprintln(&b, "- **Drift:** ✅ no drift")
	}
	fmt.Fprintln(&b, "")

	fmt.Fprintln(&b, "## Delta (after − before)")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "| Metric | Before | After | Δ | Δ% (relative) | Status |")
	fmt.Fprintln(&b, "|---|---:|---:|---:|---:|---|")
	metrics := []struct {
		name   string
		before float64
		after  float64
		delta  float64
	}{
		{"P@K", o.BeforeSummary.PrecisionAtK, o.AfterSummary.PrecisionAtK, o.Delta.Precision},
		{"R@K", o.BeforeSummary.RecallAtK, o.AfterSummary.RecallAtK, o.Delta.Recall},
		{"F1@K", o.BeforeSummary.F1AtK, o.AfterSummary.F1AtK, o.Delta.F1},
		{"MRR", o.BeforeSummary.MRR, o.AfterSummary.MRR, o.Delta.MRR},
		{"NDCG@K", o.BeforeSummary.NDCGAtK, o.AfterSummary.NDCGAtK, o.Delta.NDCG},
		{"MAP@K", o.BeforeSummary.MAPAtK, o.AfterSummary.MAPAtK, o.Delta.MAP},
		{"Gap", o.BeforeSummary.GapRate, o.AfterSummary.GapRate, o.Delta.GapRate},
		{"Blocked", o.BeforeSummary.BlockedRate, o.AfterSummary.BlockedRate, o.Delta.BlockedRate},
		{"Faith", o.BeforeSummary.Faithfulness, o.AfterSummary.Faithfulness, o.Delta.Faithfulness},
	}
	for _, m := range metrics {
		rel := 0.0
		if m.before != 0 {
			rel = m.delta / m.before * 100
		}
		status := ""
		if m.name == "P@K" || m.name == "R@K" {
			if math.Abs(m.delta) > o.Threshold {
				status = "⚠️ drift"
			} else {
				status = "ok"
			}
		}
		relStr := fmt.Sprintf("%.1f%%", rel)
		if m.before == 0 {
			relStr = "—"
		}
		fmt.Fprintf(&b, "| %s | %.4f | %.4f | %+7.4f | %7s | %s |\n", m.name, m.before, m.after, m.delta, relStr, status)
	}
	fmt.Fprintln(&b, "")

	fmt.Fprintln(&b, "## McNemar (paired per-query hits)")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "Binary hit = `hits>0` (relevant doc retrieved). Discordant = queries where one side hits and the other misses.")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "| Stat | Value |")
	fmt.Fprintln(&b, "|---|---:|")
	fmt.Fprintf(&b, "| paired queries | %d |\n", o.McNemar.Total)
	fmt.Fprintf(&b, "| a (both hit) | %d |\n", o.McNemar.A)
	fmt.Fprintf(&b, "| b (before hit, after miss) | %d |\n", o.McNemar.B)
	fmt.Fprintf(&b, "| c (before miss, after hit) | %d |\n", o.McNemar.C)
	fmt.Fprintf(&b, "| d (both miss) | %d |\n", o.McNemar.D)
	fmt.Fprintf(&b, "| discordant b+c | %d |\n", o.McNemar.Discordant)
	if o.McNemar.Feasible {
		fmt.Fprintf(&b, "| chi² = (b−c)²/(b+c) | %.4f |\n", o.McNemar.Chi2)
		fmt.Fprintf(&b, "| chi² (continuity corrected) | %.4f |\n", o.McNemar.Chi2CC)
		fmt.Fprintf(&b, "| p-value | %.4f |\n", o.McNemar.PValue)
		fmt.Fprintf(&b, "| p-value (CC) | %.4f |\n", o.McNemar.PValueCC)
		sig := "no"
		if o.McNemar.Significant {
			sig = "yes (p<0.05)"
		}
		fmt.Fprintf(&b, "| significant | %s |\n", sig)
	} else {
		fmt.Fprintf(&b, "| chi² | — (not feasible) |\n")
		fmt.Fprintf(&b, "| p-value | — |\n")
	}
	fmt.Fprintf(&b, "| note | %s |\n", o.McNemar.Note)
	fmt.Fprintln(&b, "")
	if len(o.UnpairedBefore) > 0 || len(o.UnpairedAfter) > 0 {
		fmt.Fprintln(&b, "> Unpaired queries (present in only one file):")
		if len(o.UnpairedBefore) > 0 {
			fmt.Fprintf(&b, "> - before-only: %s\n", strings.Join(o.UnpairedBefore, ", "))
		}
		if len(o.UnpairedAfter) > 0 {
			fmt.Fprintf(&b, "> - after-only: %s\n", strings.Join(o.UnpairedAfter, ", "))
		}
		fmt.Fprintln(&b, "")
	}

	fmt.Fprintln(&b, "## Per-Query Delta")
	fmt.Fprintln(&b, "")
	if len(o.PerQuery) == 0 {
		fmt.Fprintln(&b, "_No paired queries — per-query table empty._")
		fmt.Fprintln(&b, "")
	} else {
		fmt.Fprintln(&b, "| Query | Text | P_before | P_after | ΔP | R_before | R_after | ΔR | Hits | ΔHits | Gap | Blocked | Rank |")
		fmt.Fprintln(&b, "|---|---|---|---:|---:|---:|---:|---:|---:|---:|---|---|---|")
		for _, pq := range o.PerQuery {
			gapStr := ""
			if pq.BeforeGap != pq.AfterGap {
				gapStr = fmt.Sprintf("%v→%v", pq.BeforeGap, pq.AfterGap)
			} else if pq.BeforeGap {
				gapStr = "gap"
			}
			blockedStr := ""
			if pq.BeforeBlocked != pq.AfterBlocked {
				blockedStr = fmt.Sprintf("%v→%v", pq.BeforeBlocked, pq.AfterBlocked)
			} else if pq.BeforeBlocked {
				blockedStr = "blocked"
			}
			hitsStr := fmt.Sprintf("%d→%d", pq.BeforeHits, pq.AfterHits)
			rankStr := ""
			if pq.BeforeRankFirst != pq.AfterRankFirst {
				rankStr = fmt.Sprintf("%d→%d", pq.BeforeRankFirst, pq.AfterRankFirst)
			} else if pq.BeforeRankFirst != 0 {
				rankStr = fmt.Sprintf("%d", pq.BeforeRankFirst)
			}
			// Escape pipe in text
			txt := strings.ReplaceAll(pq.Text, "|", "\\|")
			if len(txt) > 40 {
				txt = txt[:37] + "..."
			}
			fmt.Fprintf(&b, "| %s | %s | %.3f | %.3f | %+6.3f | %.3f | %.3f | %+6.3f | %s | %+d | %s | %s | %s |\n",
				pq.ID, txt, pq.BeforePrecision, pq.AfterPrecision, pq.DeltaPrecision, pq.BeforeRecall, pq.AfterRecall, pq.DeltaRecall, hitsStr, pq.DeltaHits, gapStr, blockedStr, rankStr)
		}
		fmt.Fprintln(&b, "")
		fmt.Fprintln(&b, "_ΔP/R > threshold flagged as drift. McNemar uses hits>0 binary; per-query hits delta shown as simple delta when chi² not feasible._")
		fmt.Fprintln(&b, "")
	}

	fmt.Fprintln(&b, "## Interpretation")
	fmt.Fprintln(&b, "")
	if o.Drift {
		fmt.Fprintf(&b, "⚠️ **Drift detected**: %s exceeds %.1f%% threshold. Review corpus change or ranking regression. Check McNemar p-value for paired significance.\n", o.DriftReason, o.Threshold*100)
	} else {
		fmt.Fprintf(&b, "✅ **No drift**: |ΔP|=%.3f |ΔR|=%.3f within %.1f%% threshold. ", math.Abs(o.Delta.Precision), math.Abs(o.Delta.Recall), o.Threshold*100)
		if o.McNemar.Feasible && o.McNemar.Significant {
			fmt.Fprintln(&b, "Note: McNemar shows significant paired difference despite small Δ — investigate per-query regressions.")
		} else {
			fmt.Fprintln(&b, "McNemar also non-significant.")
		}
	}
	fmt.Fprintln(&b, "")
	fmt.Fprintf(&b, "_Generated via `go run ./internal/eval/drift.go --before %s --after %s`_\n", o.Before, o.After)

	return b.String()
}
