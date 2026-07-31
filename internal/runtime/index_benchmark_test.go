package runtime

import (
	"fmt"
	"os"
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
		results, err := idx.Search("research", SearchOptions{Query: fmt.Sprintf("shard %03d local", i%1000), Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: 10})
		if err != nil {
			t.Fatalf("Search(%d) error = %v", i, err)
		}
		if len(results) == 0 {
			t.Fatalf("Search(%d) returned no results", i)
		}
		durations = append(durations, time.Since(start))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[int(float64(len(durations))*0.95)-1]
	t.Logf("100k claim search p95=%s samples=%d", p95, len(durations))
	if p95 > 2*time.Second {
		t.Fatalf("100k claim search p95=%s, want <=2s", p95)
	}
}
