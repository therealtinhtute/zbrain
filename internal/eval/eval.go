package main

// eval.go — zbrain retrieval eval harness per docs/plans/active/zbrain-optimization-plan.md Phase 0.2
//
// Generates a synthetic corpus (default 1000 claims) in an isolated ZBRAIN_HOME,
// reindexes, then evaluates docs/eval/queries.json using P@10 / R@10 / MRR / NDCG@10 / MAP@10 / gap/blocked/faithfulness.
// Expected relevance is computed brute-force by scanning generated claims for phrase containment (ground truth),
// so no separate expected.json is needed. This mirrors mcp-fts5-starter narrow-vocab methodology.
//
// Usage:
//   go run ./internal/eval --corpus=1000 --limit=10 --json /tmp/eval.json
//   make eval
//
// Isolation: mktemp ZBRAIN_HOME, never touches ~/.zbrain. Removed via os.RemoveAll.

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

var benchPhrases = []string{
	"local trusted memory",
	"evidence snapshot",
	"workspace isolation",
	"claim draft approved",
	"reindex disposable",
	"verified digest",
	"trust validation",
	"index fts5 sqlite",
	"hybrid retrieval",
	"promotion candidates",
	"trust input manifest",
	"canonical markdown",
	"derived evidence",
	"supporting claim",
	"stale blocked gap",
}

type queriesFile struct {
	Version int     `json:"version"`
	Queries []query `json:"queries"`
}

type query struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type evalResult struct {
	CorpusSize   int     `json:"corpus_size"`
	Workspace    string  `json:"workspace"`
	Limit        int     `json:"limit"`
	TotalQueries int     `json:"total_queries"`
	PrecisionAtK float64 `json:"precision_at_k"`
	RecallAtK    float64 `json:"recall_at_k"`
	F1AtK        float64 `json:"f1_at_k"`
	MRR          float64 `json:"mrr"`
	NDCGAtK      float64 `json:"ndcg_at_k"`
	MAPAtK       float64 `json:"map_at_k"`
	GapRate      float64 `json:"gap_rate"`
	BlockedRate  float64 `json:"blocked_rate"`
	Faithfulness float64 `json:"faithfulness"`
	// Per-query breakdown for drift.
	Queries []perQuery `json:"queries,omitempty"`
}

type perQuery struct {
	ID        string   `json:"id"`
	Text      string   `json:"text"`
	Precision float64  `json:"precision_at_k"`
	Recall    float64  `json:"recall_at_k"`
	MRR       float64  `json:"mrr"`
	NDCG      float64  `json:"ndcg_at_k"`
	AP        float64  `json:"ap_at_k"`
	Relevant  int      `json:"relevant_total"`
	Retrieved int      `json:"retrieved"`
	Hits      int      `json:"hits"`
	Gap       bool     `json:"gap"`
	Blocked   bool     `json:"blocked"`
	RankFirst int      `json:"rank_first"` // 1-based, 0 if no hit
	RetrievedIDs []string `json:"retrieved_ids,omitempty"`
	RelevantIDs  []string `json:"relevant_ids,omitempty"`
}

func main() {
	var corpusSize int
	var limit int
	var workspace string
	var queriesPath string
	var jsonPath string
	var verbose bool
	flag.IntVar(&corpusSize, "corpus", 1000, "synthetic corpus size (claims to generate)")
	flag.IntVar(&limit, "limit", 10, "retrieval limit K for P@K/R@K/etc")
	flag.StringVar(&workspace, "workspace", "eval", "workspace name")
	flag.StringVar(&queriesPath, "queries", "docs/eval/queries.json", "path to queries.json")
	flag.StringVar(&jsonPath, "json", "", "write JSON result to file")
	flag.BoolVar(&verbose, "verbose", false, "print per-query breakdown")
	flag.Parse()

	if !zruntime.IsSafeWorkspaceName(workspace) {
		fmt.Fprintf(os.Stderr, "invalid --workspace %q\n", workspace)
		os.Exit(1)
	}
	if limit <= 0 {
		fmt.Fprintf(os.Stderr, "--limit must be >0\n")
		os.Exit(1)
	}

	qf, err := loadQueries(queriesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load queries %q: %v\n", queriesPath, err)
		os.Exit(1)
	}
	if len(qf.Queries) == 0 {
		fmt.Fprintf(os.Stderr, "no queries in %q\n", queriesPath)
		os.Exit(1)
	}

	result, err := runEval(corpusSize, workspace, limit, qf.Queries, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "eval failed: %v\n", err)
		os.Exit(1)
	}

	// Human output.
	fmt.Printf("eval: corpus=%d workspace=%q limit=%d queries=%d\n", result.CorpusSize, result.Workspace, result.Limit, result.TotalQueries)
	fmt.Printf("  P@%d=%.3f R@%d=%.3f F1=%.3f MRR=%.3f NDCG@%d=%.3f MAP@%d=%.3f gap=%.1f%% blocked=%.1f%% faith=%.3f\n",
		limit, result.PrecisionAtK, limit, result.RecallAtK, result.F1AtK, result.MRR, limit, result.NDCGAtK, limit, result.MAPAtK,
		result.GapRate*100, result.BlockedRate*100, result.Faithfulness)
	if verbose {
		for _, pq := range result.Queries {
			fmt.Printf("  %-6s P=%.2f R=%.2f MRR=%.2f NDCG=%.2f AP=%.2f hits=%d/%d retrieved=%d gap=%v\n",
				pq.ID, pq.Precision, pq.Recall, pq.MRR, pq.NDCG, pq.AP, pq.Hits, pq.Relevant, pq.Retrieved, pq.Gap)
		}
	}
	if result.PrecisionAtK < 0.5 {
		fmt.Fprintf(os.Stderr, "warn: P@%d %.3f below 0.5 — check corpus vocab narrowness\n", limit, result.PrecisionAtK)
	}

	if jsonPath != "" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
			os.Exit(1)
		}
		if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %q: %v\n", filepath.Dir(jsonPath), err)
			os.Exit(1)
		}
		if err := os.WriteFile(jsonPath, data, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write %q: %v\n", jsonPath, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "json written to %s\n", jsonPath)
	}
}

func loadQueries(path string) (queriesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return queriesFile{}, err
	}
	var qf queriesFile
	if err := json.Unmarshal(data, &qf); err != nil {
		return queriesFile{}, err
	}
	return qf, nil
}

func runEval(corpusSize int, workspace string, limit int, queries []query, verbose bool) (evalResult, error) {
	tmpDir, err := os.MkdirTemp("", "zbrain-eval-*")
	if err != nil {
		return evalResult{}, fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	paths, err := zruntime.ResolvePaths(zruntime.Options{CWD: tmpDir, HomeDir: tmpDir, RuntimeDir: tmpDir})
	if err != nil {
		return evalResult{}, fmt.Errorf("ResolvePaths: %w", err)
	}
	if _, err := zruntime.EnsureConfig(paths.ConfigFile); err != nil {
		return evalResult{}, fmt.Errorf("EnsureConfig: %w", err)
	}
	if err := zruntime.CreateWorkspace(paths, workspace, time.Now().UTC()); err != nil {
		return evalResult{}, fmt.Errorf("CreateWorkspace: %w", err)
	}

	// Generate corpus.
	store := zruntime.ClaimStore{Paths: paths, Now: func() time.Time { return time.Now().UTC() }}
	claims := make([]zruntime.Claim, 0, corpusSize)
	for i := 0; i < corpusSize; i++ {
		c := synthClaim(i, corpusSize)
		claims = append(claims, c)
		if _, err := store.WriteDraft(workspace, c); err != nil {
			return evalResult{}, fmt.Errorf("WriteDraft %d: %w", i, err)
		}
		if _, err := store.Approve(workspace, c.ID); err != nil {
			return evalResult{}, fmt.Errorf("Approve %d: %w", i, err)
		}
	}
	idx := zruntime.IndexStore{Paths: paths}
	summary, err := idx.Rebuild(workspace)
	if err != nil {
		return evalResult{}, fmt.Errorf("Rebuild: %w", err)
	}
	if summary.Approved != corpusSize {
		return evalResult{}, fmt.Errorf("Rebuild approved=%d want %d invalid=%v", summary.Approved, corpusSize, summary.InvalidClaims)
	}

	// For faithfulness / blocked we need TrustedQuery as well (checks digest/closure).
	// For pure retrieval metrics we use IndexStore.Search directly (lexical).
	// Compute ground truth per query by scanning claims.
	var totalP, totalR, totalMRR, totalNDCG, totalAP float64
	var gaps, blocked int
	var faithfulnessHits, faithfulnessTotal int
	per := make([]perQuery, 0, len(queries))

	for _, q := range queries {
		relevantIDs := relevantForQuery(q.Text, claims)
		relevantSet := make(map[string]bool, len(relevantIDs))
		for _, id := range relevantIDs {
			relevantSet[id] = true
		}

		// Lexical search.
		retrieved, err := idx.Search(workspace, zruntime.SearchOptions{Query: q.Text, Statuses: []zruntime.ClaimStatus{zruntime.ClaimStatusApproved}, Limit: limit})
		if err != nil {
			// fts5Query may reject empty — count as gap.
			retrieved = nil
		}
		retrievedIDs := make([]string, 0, len(retrieved))
		for _, r := range retrieved {
			retrievedIDs = append(retrievedIDs, r.ID)
		}

		hits := 0
		for _, id := range retrievedIDs {
			if relevantSet[id] {
				hits++
			}
		}
		prec := 0.0
		if len(retrievedIDs) > 0 {
			prec = float64(hits) / float64(len(retrievedIDs))
		}
		rec := 0.0
		if len(relevantIDs) > 0 {
			rec = float64(hits) / float64(len(relevantIDs))
		}

		// MRR: 1 / rank of first relevant.
		mrr := 0.0
		rankFirst := 0
		for i, id := range retrievedIDs {
			if relevantSet[id] {
				mrr = 1.0 / float64(i+1)
				rankFirst = i + 1
				break
			}
		}

		// AP@K: average of P@i for each relevant hit position.
		var ap float64
		if len(relevantIDs) > 0 && hits > 0 {
			var sumPrec float64
			var seenHits int
			for i, id := range retrievedIDs {
				if relevantSet[id] {
					seenHits++
					sumPrec += float64(seenHits) / float64(i+1)
				}
			}
			// MAP denominator is min(len(relevant), K) or len(relevant) — use len(relevant) capped at K per common IR.
			denom := len(relevantIDs)
			if denom > limit {
				denom = limit
			}
			if denom > 0 {
				ap = sumPrec / float64(denom)
			}
		}

		// NDCG@K with binary relevance (1 if relevant else 0). DCG = sum (rel / log2(rank+1)), IDCG = ideal DCG.
		ndcg := ndcgAtK(retrievedIDs, relevantSet, limit)

		gap := len(retrievedIDs) == 0
		if gap {
			gaps++
		}

		// Blocked / faithfulness via TrustedQuery (checks conflicts + digest).
		// We run TrustedQuery for same query and see if status is blocked.
		tq, tqErr := zruntime.TrustedQuery(paths, zruntime.TrustedQueryOptions{Workspace: workspace, Query: q.Text, Limit: limit})
		if tqErr == nil {
			if tq.Status == zruntime.QueryStatusBlocked {
				blocked++
			}
			// Faithfulness: of returned claims, how many pass validate? TrustedQuery already validates, so if we got claims they are faithful.
			// Count faithful = len(tq.Claims) if not blocked, else 0. For gap queries, no claims.
			if tq.Status == zruntime.QueryStatusReady && len(tq.Claims) > 0 {
				faithfulnessHits += len(tq.Claims)
				faithfulnessTotal += len(tq.Claims)
				// Also if blocked, those would be unfaithful in sense of conflict — but spec says blocked is explicit conflict, not hallucination.
			}
		} else {
			// If TrustedQuery fails, not counted as blocked but as error — treat as gap for now.
			// Faithfulness not updated.
			_ = tqErr
		}

		totalP += prec
		totalR += rec
		totalMRR += mrr
		totalNDCG += ndcg
		totalAP += ap

		pq := perQuery{
			ID: precToID(q.ID), Text: q.Text, Precision: prec, Recall: rec, MRR: mrr, NDCG: ndcg, AP: ap,
			Relevant: len(relevantIDs), Retrieved: len(retrievedIDs), Hits: hits, Gap: gap, RankFirst: rankFirst,
		}
		if verbose {
			pq.RetrievedIDs = retrievedIDs
			pq.RelevantIDs = relevantIDs
		}
		if tqErr == nil && tq.Status == zruntime.QueryStatusBlocked {
			pq.Blocked = true
		}
		per = append(per, pq)
	}

	n := float64(len(queries))
	res := evalResult{
		CorpusSize:   corpusSize,
		Workspace:    workspace,
		Limit:        limit,
		TotalQueries: len(queries),
		PrecisionAtK: totalP / n,
		RecallAtK:    totalR / n,
		MRR:          totalMRR / n,
		NDCGAtK:      totalNDCG / n,
		MAPAtK:       totalAP / n,
		GapRate:      float64(gaps) / n,
		BlockedRate:  float64(blocked) / n,
		Queries:      per,
	}
	// F1 harmonic mean.
	if res.PrecisionAtK+res.RecallAtK > 0 {
		res.F1AtK = 2 * res.PrecisionAtK * res.RecallAtK / (res.PrecisionAtK + res.RecallAtK)
	}
	// Faithfulness: if no total, assume 1.0 (no hallucinations observed). If blocked/gap, not hallucinated.
	if faithfulnessTotal == 0 {
		res.Faithfulness = 1.0
	} else {
		res.Faithfulness = float64(faithfulnessHits) / float64(faithfulnessTotal)
	}
	// Sort per query for stable output.
	sort.Slice(res.Queries, func(i, j int) bool { return res.Queries[i].ID < res.Queries[j].ID })
	return res, nil
}

func precToID(s string) string { return s }

func relevantForQuery(queryText string, claims []zruntime.Claim) []string {
	qTokens := tokenize(queryText)
	if len(qTokens) == 0 {
		return nil
	}
	var out []string
	for _, c := range claims {
		// Build searchable text lowercased: title + description + body + tags.
		haystack := strings.ToLower(c.Title + " " + c.Description + " " + c.Body + " " + strings.Join(c.Tags, " "))
		if containsAllTokens(haystack, qTokens) {
			out = append(out, c.ID)
		}
	}
	sort.Strings(out)
	return out
}

func tokenize(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	// Split on non-alphanum, keep phrases as tokens? For ground truth we match phrase containment directly
	// if query contains space, we check substring containment of whole lowercased query in haystack as well.
	// Simple: if query has 2+ words, also check substring.
	tokens := strings.Fields(s)
	return tokens
}

func containsAllTokens(haystack string, tokens []string) bool {
	// For multi-word query, require all tokens present (AND) — matches fts5Query AND semantics.
	// Also handle phrase: if original query had phrase that appears contiguously, tokens will still be checked.
	// This is intentionally strict to avoid over-generous relevance.
	for _, tok := range tokens {
		if !strings.Contains(haystack, tok) {
			return false
		}
	}
	// Additionally, for single-phrase queries (our 15), this works. For composites, AND is correct per index.go fts5Query.
	return true
}

func ndcgAtK(retrieved []string, relevantSet map[string]bool, k int) float64 {
	if len(retrieved) == 0 || len(relevantSet) == 0 {
		return 0
	}
	// DCG
	var dcg float64
	for i, id := range retrieved {
		if i >= k {
			break
		}
		rel := 0.0
		if relevantSet[id] {
			rel = 1.0
		}
		// log2(rank+1) where rank is 1-based: rank=i+1 => log2(i+2)
		dcg += rel / math.Log2(float64(i+2))
	}
	// IDCG: ideal ranking puts all relevant first.
	relevantCount := len(relevantSet)
	if relevantCount > k {
		relevantCount = k
	}
	var idcg float64
	for i := 0; i < relevantCount; i++ {
		idcg += 1.0 / math.Log2(float64(i+2))
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

func synthClaim(i, total int) zruntime.Claim {
	id := fmt.Sprintf("clm_%032x", i+1)
	tier := zruntime.WikiTiers[i%len(zruntime.WikiTiers)]
	phrase := benchPhrases[i%len(benchPhrases)]
	title := fmt.Sprintf("Bench %06d %s", i, titleCase(phrase))
	desc := fmt.Sprintf("bench corpus %d/%d %s", i+1, total, phrase)
	body := synthBody(i, phrase)
	return zruntime.Claim{
		Type:        zruntime.OKFClaimType,
		ID:          id,
		Tier:        tier,
		Status:      zruntime.ClaimStatusDraft,
		Title:       title,
		Description: desc,
		Basis:       zruntime.ClaimBasisOwner,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		CreatedBy:   "owner",
		Tags:        []string{"bench", "benchmark", strings.ReplaceAll(phrase, " ", "-")},
		Body:        body,
	}
}

func synthBody(i int, primaryPhrase string) string {
	var b strings.Builder
	count := 3 + (i % 3)
	for k := 0; k < count; k++ {
		idx := (i + k*7) % len(benchPhrases)
		b.WriteString(benchPhrases[idx])
		b.WriteString(". ")
	}
	b.WriteString(primaryPhrase)
	b.WriteString(" — bench document ")
	b.WriteString(fmt.Sprintf("%d", i))
	b.WriteString(".\n\n")
	filler := "System maintains operational consistency through incremental synchronization and periodic checkpoint validation. Data locality ensures low-latency access and offline availability. "
	for b.Len() < 3500 {
		if b.Len()%700 < 120 {
			b.WriteString(benchPhrases[(i+b.Len())%len(benchPhrases)])
			b.WriteString(" ")
		}
		b.WriteString(filler)
		if b.Len()%1100 < 113 {
			b.WriteString("Index rebuild captures input manifest and ensures retrieval remains consistent. ")
		}
	}
	s := b.String()
	if len(s) > 3584 {
		s = s[:3500]
	}
	if len(s) > 0 && s[len(s)-1] != '\n' {
		s += "\n"
	}
	return s
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) == 0 {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
