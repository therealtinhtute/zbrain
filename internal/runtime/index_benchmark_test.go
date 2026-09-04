package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestAskP95At100K(t *testing.T) {
	if os.Getenv("ZBRAIN_BENCH_100K") != "1" {
		t.Skip("set ZBRAIN_BENCH_100K=1 to run the 100k claim benchmark")
	}
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	for i := 0; i < 100000; i++ {
		claim := indexClaim(fmt.Sprintf("clm_%032x", i+1), fmt.Sprintf("Benchmark Claim %06d", i), ClaimBasisOwner)
		claim.Tags = []string{"benchmark"}
		claim.Body = fmt.Sprintf("benchmark corpus body shard %03d repeated local memory retrieval token\n", i%1000)
		if _, err := store.WriteDraft("research", claim); err != nil {
			t.Fatalf("WriteDraft(%d) error = %v", i, err)
		}
		if _, err := store.Approve("research", claim.ID); err != nil {
			t.Fatalf("Approve(%d) error = %v", i, err)
		}
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	durations := make([]time.Duration, 0, 40)
	for i := 0; i < 40; i++ {
		start := time.Now()
		response, err := TrustedQuery(paths, TrustedQueryOptions{Query: fmt.Sprintf("shard %03d local", i%1000), Limit: 10})
		if err != nil {
			t.Fatalf("TrustedQuery(%d) error = %v", i, err)
		}
		if len(response.Claims) == 0 {
			t.Fatalf("TrustedQuery(%d) returned no results: %#v", i, response)
		}
		durations = append(durations, time.Since(start))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[int(float64(len(durations))*0.95)-1]
	p50 := durations[len(durations)/2]
	t.Logf("100k trusted ask p95=%s samples=%d", p95, len(durations))
	if p95 > 2*time.Second {
		t.Fatalf("100k trusted ask p95=%s, want <=2s", p95)
	}
	// Eval-suite proof artifact (M3): record the measured percentiles under
	// docs/proofs/ so runs are diffable across commits.
	proof := map[string]any{
		"schema":       "zbrain.eval.perf-100k/v1",
		"corpus_size":  100000,
		"samples":      len(durations),
		"p50_ms":       float64(p50.Microseconds()) / 1000,
		"p95_ms":       float64(p95.Microseconds()) / 1000,
		"p99_ms":       float64(durations[len(durations)-1].Microseconds()) / 1000,
		"target_p95_s": 2,
		"pass":         p95 <= 2*time.Second,
	}
	data, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		t.Fatalf("marshal perf proof: %v", err)
	}
	data = append(data, '\n')
	proofPath := filepath.Join("..", "..", "docs", "proofs", "eval-perf-100k.json")
	if err := os.WriteFile(proofPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", proofPath, err)
	}
}

func TestAskP50P95P99(t *testing.T) {
	if os.Getenv("ZBRAIN_BENCH_100K") != "1" {
		t.Skip("set ZBRAIN_BENCH_100K=1 to run p50/p95/p99 benchmark")
	}
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	for i := 0; i < 100000; i++ {
		claim := indexClaim(fmt.Sprintf("clm_%032x", i+1), fmt.Sprintf("Benchmark Claim %06d", i), ClaimBasisOwner)
		claim.Tags = []string{"benchmark"}
		claim.Body = fmt.Sprintf("benchmark corpus body shard %03d repeated local memory retrieval token\n", i%1000)
		if _, err := store.WriteDraft("research", claim); err != nil {
			t.Fatalf("WriteDraft(%d) error = %v", i, err)
		}
		if _, err := store.Approve("research", claim.ID); err != nil {
			t.Fatalf("Approve(%d) error = %v", i, err)
		}
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	durations := make([]time.Duration, 0, 40)
	for i := 0; i < 40; i++ {
		start := time.Now()
		response, err := TrustedQuery(paths, TrustedQueryOptions{Query: fmt.Sprintf("shard %03d local", i%1000), Limit: 10})
		if err != nil {
			t.Fatalf("TrustedQuery(%d) error = %v", i, err)
		}
		if len(response.Claims) == 0 {
			t.Fatalf("TrustedQuery(%d) returned no results: %#v", i, response)
		}
		durations = append(durations, time.Since(start))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := durations[int(float64(len(durations))*0.50)]
	p95 := durations[int(float64(len(durations))*0.95)-1]
	p99 := durations[int(float64(len(durations))*0.99)-1]
	if len(durations) > 0 {
		// Clamp p99 index to last element if calculation exceeds bounds.
		idx99 := int(float64(len(durations)) * 0.99)
		if idx99 >= len(durations) {
			idx99 = len(durations) - 1
		}
		p99 = durations[idx99]
	}
	t.Logf("100k trusted ask p50=%s p95=%s p99=%s samples=%d", p50, p95, p99, len(durations))
	if p95 > 2*time.Second {
		t.Fatalf("100k trusted ask p95=%s, want <=2s", p95)
	}
}
