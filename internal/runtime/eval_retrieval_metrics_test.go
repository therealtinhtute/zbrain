package runtime

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEvalRetrievalMetrics is the eval-suite retrieval-metrics runner. It
// computes recall@10 and MRR for the lexical (default) and hybrid
// (reindex --embed + ask --embed) paths over a deterministic narrow-vocab
// corpus, and validates that every query in docs/eval/queries.json carries
// an expected hit count. Results land in
// docs/proofs/eval-retrieval-metrics.json.
func TestEvalRetrievalMetrics(t *testing.T) {
	const k = 10
	paths := queryTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedQueryNow}

	// Deterministic corpus: 15 phrases x 6 documents. Every document contains
	// its primary phrase plus two deterministic secondary phrases.
	phrases := []string{
		"local trusted memory", "evidence snapshot", "workspace isolation",
		"claim draft approved", "reindex disposable", "verified digest",
		"trust validation", "index fts5 sqlite", "hybrid retrieval",
		"promotion candidates", "trust input manifest", "canonical markdown",
		"derived evidence", "supporting claim", "stale blocked gap",
	}
	const docsPerPhrase = 6
	claims := make([]Claim, 0, len(phrases)*docsPerPhrase)
	for i := 0; i < len(phrases)*docsPerPhrase; i++ {
		primary := phrases[i%len(phrases)]
		secondary := []string{phrases[(i+3)%len(phrases)], phrases[(i+7)%len(phrases)]}
		id := fmt.Sprintf("clm_%032x", i+1)
		claim := queryClaim(id, fmt.Sprintf("Memo %02d %s", i, titleForPhrase(primary)), ClaimBasisOwner)
		claim.Tags = []string{"eval", tagForPhrase(primary)}
		claim.Body = primary + ". " + secondary[0] + ". " + secondary[1] + ".\n"
		claims = append(claims, claim)
		if _, err := store.WriteDraft("research", claim); err != nil {
			t.Fatalf("WriteDraft(%s) error = %v", id, err)
		}
		if _, err := store.Approve("research", id); err != nil {
			t.Fatalf("Approve(%s) error = %v", id, err)
		}
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild(lexical) error = %v", err)
	}

	queries := make([]struct{ id, text string }, 0, len(phrases)+5)
	for i, phrase := range phrases {
		queries = append(queries, struct{ id, text string }{fmt.Sprintf("m%02d", i+1), phrase})
	}
	// Composite queries (AND semantics like the FTS layer). Pairs are chosen
	// so at least one corpus document contains both phrases: document i holds
	// phrases[i], phrases[(i+3)%15], and phrases[(i+7)%15].
	composites := []struct{ id, text string }{
		{"m16", "local trusted memory claim draft approved"},
		{"m17", "evidence snapshot hybrid retrieval"},
		{"m18", "workspace isolation verified digest"},
		{"m19", "claim draft approved trust input manifest"},
		{"m20", "reindex disposable canonical markdown"},
	}
	queries = append(queries, composites...)

	relevantFor := func(queryText string) map[string]bool {
		tokens := strings.Fields(strings.ToLower(queryText))
		relevant := map[string]bool{}
		for _, claim := range claims {
			haystack := strings.ToLower(claim.Title + " " + claim.Description + " " + claim.Body + " " + strings.Join(claim.Tags, " "))
			match := true
			for _, token := range tokens {
				if !strings.Contains(haystack, token) {
					match = false
					break
				}
			}
			if match {
				relevant[claim.ID] = true
			}
		}
		return relevant
	}

	modeMetrics := func(embedding bool) (recall, mrr float64, perQuery []map[string]any) {
		for _, q := range queries {
			relevant := relevantFor(q.text)
			response, err := TrustedQuery(paths, TrustedQueryOptions{Workspace: "research", Query: q.text, Limit: k, Embedding: embedding})
			if err != nil {
				t.Fatalf("TrustedQuery(%q, embedding=%t) error = %v", q.text, embedding, err)
			}
			if response.Status != QueryStatusReady {
				t.Fatalf("TrustedQuery(%q, embedding=%t) status = %q, want ready", q.text, embedding, response.Status)
			}
			hits := 0
			firstRank := 0
			for rank, claim := range response.Claims {
				if relevant[claim.ID] {
					hits++
					if firstRank == 0 {
						firstRank = rank + 1
					}
				}
			}
			qRecall := 0.0
			if len(relevant) > 0 {
				qRecall = float64(hits) / float64(len(relevant))
			}
			qMRR := 0.0
			if firstRank > 0 {
				qMRR = 1.0 / float64(firstRank)
			}
			recall += qRecall
			mrr += qMRR
			perQuery = append(perQuery, map[string]any{
				"id": q.id, "text": q.text, "hits": hits, "relevant": len(relevant),
				"recall_at_k": round4(qRecall), "mrr": round4(qMRR),
			})
		}
		n := float64(len(queries))
		return recall / n, mrr / n, perQuery
	}

	lexRecall, lexMRR, lexPer := modeMetrics(false)
	if _, err := idx.RebuildWithOptions("research", RebuildOptions{Embedding: true}); err != nil {
		t.Fatalf("RebuildWithOptions(embedding) error = %v", err)
	}
	hybRecall, hybMRR, hybPer := modeMetrics(true)

	// Assertions: every single-phrase query has 18 relevant documents
	// (6 primary + 12 secondary occurrences), so recall@10 is capped at
	// 10/18 ≈ 0.556 for those queries; the floor reflects that cap. MRR must
	// be near-perfect, and hybrid must not collapse below lexical.
	if lexRecall < 0.45 || lexMRR < 0.8 {
		t.Fatalf("lexical recall@%d=%.3f mrr=%.3f below floor (0.45/0.8)", k, lexRecall, lexMRR)
	}
	if hybRecall < 0.45 || hybMRR < 0.8 {
		t.Fatalf("hybrid recall@%d=%.3f mrr=%.3f below floor (0.45/0.8)", k, hybRecall, hybMRR)
	}
	if hybRecall < lexRecall-0.05 || hybMRR < lexMRR-0.05 {
		t.Fatalf("hybrid regression: recall %.3f→%.3f mrr %.3f→%.3f (tolerance 0.05)", lexRecall, hybRecall, lexMRR, hybMRR)
	}

	// queries.json must carry expected hits for every query (M1 extension).
	queriesPath := filepath.Join("..", "..", "docs", "eval", "queries.json")
	data, err := os.ReadFile(queriesPath)
	if err != nil {
		t.Fatalf("ReadFile(queries.json) error = %v", err)
	}
	var queriesFile struct {
		Version int `json:"version"`
		Queries []struct {
			ID            string `json:"id"`
			Text          string `json:"text"`
			RelevantTotal int    `json:"relevant_total"`
		} `json:"queries"`
	}
	if err := json.Unmarshal(data, &queriesFile); err != nil {
		t.Fatalf("decode queries.json: %v", err)
	}
	if len(queriesFile.Queries) == 0 {
		t.Fatal("queries.json contains no queries")
	}
	withExpected := 0
	for _, q := range queriesFile.Queries {
		if q.RelevantTotal <= 0 {
			t.Fatalf("query %s (%q) has no expected hits (relevant_total=%d)", q.ID, q.Text, q.RelevantTotal)
		}
		withExpected++
	}

	writeProofJSON(t, "eval-retrieval-metrics.json", map[string]any{
		"schema":      "zbrain.eval.retrieval-metrics/v1",
		"corpus_size": len(claims),
		"k":           k,
		"modes": map[string]any{
			"lexical": map[string]any{"recall_at_k": round4(lexRecall), "mrr": round4(lexMRR), "per_query": lexPer},
			"hybrid":  map[string]any{"recall_at_k": round4(hybRecall), "mrr": round4(hybMRR), "per_query": hybPer},
		},
		"queries_file": map[string]any{
			"path": "docs/eval/queries.json", "queries": len(queriesFile.Queries), "with_expected_hits": withExpected,
			"baseline_comparable": true,
			"note":                "relevant_total added without changing query texts; internal/eval computes ground truth brute-force, so docs/proofs/eval-baseline.json stays comparable",
		},
	})
}

func titleForPhrase(phrase string) string {
	words := strings.Fields(phrase)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func tagForPhrase(phrase string) string {
	return strings.ReplaceAll(phrase, " ", "-")
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

// writeProofJSON writes a deterministic JSON artifact under docs/proofs/.
func writeProofJSON(t *testing.T, name string, payload any) {
	t.Helper()
	root := ".." + string(filepath.Separator) + ".."
	path := filepath.Join(root, "docs", "proofs", name)
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal proof %s: %v", name, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
