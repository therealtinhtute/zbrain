package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strconv"
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

type benchResult struct {
	CorpusSize   int     `json:"corpus_size"`
	Workspace    string  `json:"workspace"`
	IndexTimeMs  float64 `json:"index_time_ms"`
	IndexTime    string  `json:"index_time"`
	Throughput   float64 `json:"throughput_docs_per_sec"`
	DBSize       int64   `json:"db_size_bytes"`
	DBSizeHuman  string  `json:"db_size_human"`
	PeakHeap     uint64  `json:"peak_heap_bytes"`
	PeakHeapHuman string `json:"peak_heap_human"`
	QueryP50Ms   float64 `json:"query_p50_ms"`
	QueryP95Ms   float64 `json:"query_p95_ms"`
	QueryP99Ms   float64 `json:"query_p99_ms"`
	QueryP50     string  `json:"query_p50"`
	QueryP95     string  `json:"query_p95"`
	QueryP99     string  `json:"query_p99"`
	TotalQueries int     `json:"total_queries"`
	Approved     int     `json:"approved"`
	DBPath       string  `json:"db_path,omitempty"`
}

func main() {
	var sizesFlag string
	var jsonPath string
	var workspace string
	flag.StringVar(&sizesFlag, "sizes", "100,1000,10000", "comma-separated corpus sizes to benchmark (e.g., --sizes=100,1000)")
	flag.StringVar(&jsonPath, "json", "", "write JSON results to file (e.g., --json /tmp/b.json)")
	flag.StringVar(&workspace, "workspace", "bench", "workspace name to use (default bench)")
	flag.Parse()

	sizes, err := parseSizes(sizesFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --sizes: %v\n", err)
		os.Exit(1)
	}
	if len(sizes) == 0 {
		fmt.Fprintf(os.Stderr, "--sizes must not be empty\n")
		os.Exit(1)
	}
	if !zruntime.IsSafeWorkspaceName(workspace) {
		fmt.Fprintf(os.Stderr, "invalid --workspace %q: must use lowercase letters, numbers, or hyphens only\n", workspace)
		os.Exit(1)
	}

	results := make([]benchResult, 0, len(sizes))
	for _, n := range sizes {
		fmt.Fprintf(os.Stderr, "bench: corpus=%d workspace=%q ...\n", n, workspace)
		r, err := runBench(n, workspace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bench failed for size %d: %v\n", n, err)
			os.Exit(1)
		}
		results = append(results, r)
		fmt.Fprintf(os.Stderr, "bench: size=%d index=%s throughput=%.1f doc/s db=%s heap=%s p50=%s p95=%s p99=%s\n",
			r.CorpusSize, r.IndexTime, r.Throughput, r.DBSizeHuman, r.PeakHeapHuman, r.QueryP50, r.QueryP95, r.QueryP99)
	}

	printMarkdown(results)

	if jsonPath != "" {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal json: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(jsonPath, data, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write json %q: %v\n", jsonPath, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "json written to %s\n", jsonPath)
	}
}

func parseSizes(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	var out []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", p, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("size %d must be >0", n)
		}
		out = append(out, n)
	}
	return out, nil
}

func runBench(n int, workspace string) (benchResult, error) {
	tmpDir, err := os.MkdirTemp("", "zbrain-bench-*")
	if err != nil {
		return benchResult{}, fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Resolve isolated ZBRAIN_HOME.
	paths, err := zruntime.ResolvePaths(zruntime.Options{CWD: tmpDir, HomeDir: tmpDir, RuntimeDir: tmpDir})
	if err != nil {
		return benchResult{}, fmt.Errorf("ResolvePaths: %w", err)
	}
	if _, err := zruntime.EnsureConfig(paths.ConfigFile); err != nil {
		return benchResult{}, fmt.Errorf("EnsureConfig: %w", err)
	}
	if err := zruntime.CreateWorkspace(paths, workspace, time.Now().UTC()); err != nil {
		return benchResult{}, fmt.Errorf("CreateWorkspace(%q): %w", workspace, err)
	}

	store := zruntime.ClaimStore{Paths: paths, Now: func() time.Time { return time.Now().UTC() }}
	for i := 0; i < n; i++ {
		claim := synthClaim(i, n)
		if _, err := store.WriteDraft(workspace, claim); err != nil {
			return benchResult{}, fmt.Errorf("WriteDraft %d: %w", i, err)
		}
		if _, err := store.Approve(workspace, claim.ID); err != nil {
			return benchResult{}, fmt.Errorf("Approve %d (%s): %w", i, claim.ID, err)
		}
	}

	// Capture heap before rebuild.
	goruntime.GC()
	var before goruntime.MemStats
	goruntime.ReadMemStats(&before)

	idx := zruntime.IndexStore{Paths: paths}
	start := time.Now()
	summary, err := idx.Rebuild(workspace)
	elapsed := time.Since(start)
	if err != nil {
		return benchResult{}, fmt.Errorf("Rebuild: %w", err)
	}
	if summary.Approved != n {
		// Rebuild may have rejected some if validation failed — fail fast.
		return benchResult{}, fmt.Errorf("Rebuild approved=%d want %d (invalid=%d rejected=%v)", summary.Approved, n, summary.Invalid, summary.InvalidClaims)
	}

	goruntime.GC()
	var after goruntime.MemStats
	goruntime.ReadMemStats(&after)
	peakHeap := after.HeapAlloc
	if before.HeapAlloc > peakHeap {
		peakHeap = before.HeapAlloc
	}
	// Also consider Alloc as alternative.
	if after.Alloc > peakHeap {
		peakHeap = after.Alloc
	}

	dbPath := filepath.Join(paths.IndexesDir, workspace+".sqlite")
	info, err := os.Stat(dbPath)
	if err != nil {
		return benchResult{}, fmt.Errorf("stat db %q: %w", dbPath, err)
	}
	dbSize := info.Size()

	// Warm + measured queries.
	queries := benchPhrases
	// Warm-up: run each query once without measuring.
	for _, q := range queries {
		_, _ = idx.Search(workspace, zruntime.SearchOptions{Query: q, Statuses: []zruntime.ClaimStatus{zruntime.ClaimStatusApproved}, Limit: 10})
	}
	durations := make([]time.Duration, 0, len(queries)*3)
	for iter := 0; iter < 3; iter++ {
		for _, q := range queries {
			t0 := time.Now()
			results, err := idx.Search(workspace, zruntime.SearchOptions{Query: q, Statuses: []zruntime.ClaimStatus{zruntime.ClaimStatusApproved}, Limit: 10})
			d := time.Since(t0)
			if err != nil {
				return benchResult{}, fmt.Errorf("Search %q iter %d: %w", q, iter, err)
			}
			// Ensure we keep durations even if no results (still measures latency).
			_ = results
			durations = append(durations, d)
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := percentile(durations, 0.50)
	p95 := percentile(durations, 0.95)
	p99 := percentile(durations, 0.99)

	throughput := float64(n) / elapsed.Seconds()
	if elapsed == 0 {
		throughput = 0
	}

	return benchResult{
		CorpusSize:    n,
		Workspace:     workspace,
		IndexTimeMs:   float64(elapsed.Nanoseconds()) / 1e6,
		IndexTime:     elapsed.String(),
		Throughput:    throughput,
		DBSize:        dbSize,
		DBSizeHuman:   formatBytes(dbSize),
		PeakHeap:      peakHeap,
		PeakHeapHuman: formatBytes(int64(peakHeap)),
		QueryP50Ms:    float64(p50.Nanoseconds()) / 1e6,
		QueryP95Ms:    float64(p95.Nanoseconds()) / 1e6,
		QueryP99Ms:    float64(p99.Nanoseconds()) / 1e6,
		QueryP50:      p50.String(),
		QueryP95:      p95.String(),
		QueryP99:      p99.String(),
		TotalQueries:  len(durations),
		Approved:      summary.Approved,
		DBPath:        dbPath,
	}, nil
}

func synthClaim(i, total int) zruntime.Claim {
	id := fmt.Sprintf("clm_%032x", i+1)
	// Rotate tier for realistic distribution but keep deterministic.
	tier := zruntime.WikiTiers[i%len(zruntime.WikiTiers)]
	// Pick a primary phrase for title/description to ensure hit.
	phrase := benchPhrases[i%len(benchPhrases)]
	title := fmt.Sprintf("Bench %06d %s", i, titleCase(phrase))
	desc := fmt.Sprintf("bench corpus %d/%d %s", i+1, total, phrase)
	body := synthBody(i, phrase)
	return zruntime.Claim{
		Type:      zruntime.OKFClaimType,
		ID:        id,
		Tier:      tier,
		Status:    zruntime.ClaimStatusDraft,
		Title:     title,
		Description: desc,
		Basis:     zruntime.ClaimBasisOwner,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedBy: "owner",
		Tags:      []string{"bench", "benchmark", strings.ReplaceAll(phrase, " ", "-")},
		Body:      body,
	}
}

func synthBody(i int, primaryPhrase string) string {
	// Narrow vocab: each doc contains 3-5 of the 15 phrases, so hit rate >30%.
	// Avg 3.5KB per doc.
	var b strings.Builder
	// Estimate: 15 phrases * avg 18 bytes = 270, plus filler.
	count := 3 + (i % 3) // 3,4,5
	for k := 0; k < count; k++ {
		idx := (i + k*7) % len(benchPhrases)
		b.WriteString(benchPhrases[idx])
		b.WriteString(". ")
	}
	// Include primary phrase explicitly for query hit guarantee.
	b.WriteString(primaryPhrase)
	b.WriteString(" — bench document ")
	b.WriteString(strconv.Itoa(i))
	b.WriteString(".\n\n")

	filler := "System maintains operational consistency through incremental synchronization and periodic checkpoint validation. Data locality ensures low-latency access and offline availability. "
	// filler is neutral (no bench phrases) to keep relevance discriminative; phrases are injected separately.
	for b.Len() < 3500 {
		// Every 700 bytes inject a phrase to keep hit rate high (~30% per phrase).
		if b.Len()%700 < 120 {
			b.WriteString(benchPhrases[(i+b.Len())%len(benchPhrases)])
			b.WriteString(" ")
		}
		b.WriteString(filler)
		// Add variation with neutral sentence.
		if b.Len()%1100 < 113 {
			b.WriteString("Index rebuild captures input manifest and ensures retrieval remains consistent. ")
		}
	}
	s := b.String()
	if len(s) > 3584 {
		s = s[:3500]
	}
	// Ensure we end on a sentence.
	if len(s) > 0 && s[len(s)-1] != '\n' {
		s += "\n"
	}
	return s
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	// Use nearest-rank method consistent with index_benchmark_test: int(len * p) -1 not suitable for small N.
	// Use ceil(p*N) -1 or linear interpolation via index = int((len-1)*p)
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	prefix := "KMGTPE"[exp]
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), prefix)
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

func printMarkdown(results []benchResult) {
	fmt.Println("# Benchmark results")
	fmt.Println()
	fmt.Println("| Corpus size | Index time | Throughput | DB size | Peak heap | Query p50 | p95 | p99 |")
	fmt.Println("|---:|---|---|---|---|---|---|---|")
	for _, r := range results {
		fmt.Printf("| %d | %s | %.1f doc/s | %s | %s | %s | %s | %s |\n",
			r.CorpusSize, r.IndexTime, r.Throughput, r.DBSizeHuman, r.PeakHeapHuman, r.QueryP50, r.QueryP95, r.QueryP99)
	}
	fmt.Println()
	fmt.Println("_Generated via `go run ./scripts/bench-fts5.go --sizes=100,1000,10000` — single run, not vendor comparison._")
}
