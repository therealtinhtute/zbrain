package runtime

import (
	"database/sql"
	"math"
	"reflect"
	"testing"
)

func TestLoopbackEmbedderDeterministic(t *testing.T) {
	embedder := NewLoopbackEmbedder()
	vectors, err := embedder.Embed([]string{"trusted local memory retrieval"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 {
		t.Fatalf("Embed() returned %d vectors, want 1", len(vectors))
	}
	if len(vectors[0]) != embedder.Dimension {
		t.Fatalf("vector dimension = %d, want %d", len(vectors[0]), embedder.Dimension)
	}
	again, err := embedder.Embed([]string{"trusted local memory retrieval"})
	if err != nil {
		t.Fatalf("Embed() again error = %v", err)
	}
	if !reflect.DeepEqual(vectors[0], again[0]) {
		t.Fatalf("Embed() is not deterministic for identical input")
	}
}

func TestLoopbackEmbedderDistinctInputs(t *testing.T) {
	embedder := NewLoopbackEmbedder()
	first, err := embedder.Embed([]string{"trusted local memory retrieval"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	second, err := embedder.Embed([]string{"quantum entanglement probability"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if reflect.DeepEqual(first[0], second[0]) {
		t.Fatalf("Embed() produced identical vectors for distinct inputs")
	}
}

func TestLoopbackEmbedderNormalized(t *testing.T) {
	embedder := NewLoopbackEmbedder()
	vectors, err := embedder.Embed([]string{"local-first trusted memory", "", "x"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	for i, vector := range vectors {
		var sumSquares float64
		for _, component := range vector {
			sumSquares += float64(component) * float64(component)
		}
		if math.Abs(sumSquares-1.0) > 1e-4 && sumSquares != 0 {
			t.Fatalf("vector %d not L2-normalized: sum of squares = %f", i, sumSquares)
		}
	}
}

func TestLoopbackEmbedderSearchVectors(t *testing.T) {
	paths := indexTestPaths(t)
	store := EmbeddingStore{Paths: paths}
	records := []EmbeddingRecord{
		{ClaimID: "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Dimension: 384, Vector: make([]float32, 384)},
		{ClaimID: "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Dimension: 384, Vector: make([]float32, 384)},
	}
	// First claim embeds "quantum entanglement observer", second embeds "weather report".
	embedder := NewLoopbackEmbedder()
	vectors, err := embedder.Embed([]string{"quantum entanglement observer", "weather report"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	copy(records[0].Vector, vectors[0])
	copy(records[1].Vector, vectors[1])
	if err := store.StoreVectors("research", records); err != nil {
		t.Fatalf("StoreVectors() error = %v", err)
	}

	ids, err := store.SearchVectors("research", "quantum entanglement observer", 10)
	if err != nil {
		t.Fatalf("SearchVectors() error = %v", err)
	}
	if len(ids) == 0 || ids[0] != records[0].ClaimID {
		t.Fatalf("SearchVectors() = %v, want %q first", ids, records[0].ClaimID)
	}

	// Missing database falls back to nil.
	ids, err = store.SearchVectors("missing", "quantum", 10)
	if err != nil {
		t.Fatalf("SearchVectors(missing) error = %v", err)
	}
	if ids != nil {
		t.Fatalf("SearchVectors(missing) = %v, want nil", ids)
	}

	// Empty store falls back to nil.
	if err := store.Close("research"); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	ids, err = store.SearchVectors("research", "quantum", 10)
	if err != nil {
		t.Fatalf("SearchVectors(empty) error = %v", err)
	}
	if ids != nil {
		t.Fatalf("SearchVectors(empty) = %v, want nil", ids)
	}
}

func TestLoopbackEmbedderStoreRoundTrip(t *testing.T) {
	paths := indexTestPaths(t)
	store := EmbeddingStore{Paths: paths}
	records := []EmbeddingRecord{
		{ClaimID: "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Dimension: 2, Vector: []float32{0.6, 0.8}},
		{ClaimID: "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Dimension: 2, Vector: []float32{1, 0}},
	}
	if err := store.StoreVectors("research", records); err != nil {
		t.Fatalf("StoreVectors() error = %v", err)
	}
	count, err := store.Count("research")
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("Count() = %d, want 2", count)
	}
	if err := store.Close("research"); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	count, err = store.Count("research")
	if err != nil {
		t.Fatalf("Count() after Close error = %v", err)
	}
	if count != 0 {
		t.Fatalf("Count() after Close = %d, want 0", count)
	}
}

func TestLoopbackEmbedderStoreEmptyClears(t *testing.T) {
	paths := indexTestPaths(t)
	store := EmbeddingStore{Paths: paths}
	if err := store.StoreVectors("research", []EmbeddingRecord{
		{ClaimID: "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Dimension: 2, Vector: []float32{0.6, 0.8}},
	}); err != nil {
		t.Fatalf("StoreVectors() error = %v", err)
	}
	if err := store.StoreVectors("research", nil); err != nil {
		t.Fatalf("StoreVectors(nil) error = %v", err)
	}
	count, err := store.Count("research")
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("Count() after empty store = %d, want 0", count)
	}
}

func TestLoopbackEmbedderRebuildStoresVectors(t *testing.T) {
	paths := indexTestPaths(t)
	claimStore := ClaimStore{Paths: paths, Now: fixedIndexNow}
	approved := indexClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Loopback Embedding Memory", ClaimBasisOwner)
	if _, err := claimStore.WriteDraft("research", approved); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := claimStore.Approve("research", approved.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	idx := IndexStore{Paths: paths}

	// Rebuild without --embed must not produce embeddings.
	summary, err := idx.RebuildWithOptions("research", RebuildOptions{Embedding: false})
	if err != nil {
		t.Fatalf("RebuildWithOptions(no embed) error = %v", err)
	}
	if summary.Embedding.Indexed != 0 {
		t.Fatalf("summary.Embedding.Indexed = %d, want 0 without --embed", summary.Embedding.Indexed)
	}
	count, err := (EmbeddingStore{Paths: paths}).Count("research")
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("embedding count = %d, want 0 without --embed", count)
	}

	// Rebuild with --embed stores vectors for approved claims.
	summary, err = idx.RebuildWithOptions("research", RebuildOptions{Embedding: true})
	if err != nil {
		t.Fatalf("RebuildWithOptions(embed) error = %v", err)
	}
	if summary.Embedding.Strategy != "loopback" {
		t.Fatalf("summary.Embedding.Strategy = %q, want loopback", summary.Embedding.Strategy)
	}
	if summary.Embedding.Indexed != 1 {
		t.Fatalf("summary.Embedding.Indexed = %d, want 1", summary.Embedding.Indexed)
	}
	if summary.Embedding.Eligible != 1 {
		t.Fatalf("summary.Embedding.Eligible = %d, want 1", summary.Embedding.Eligible)
	}
	count, err = (EmbeddingStore{Paths: paths}).Count("research")
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("embedding count = %d, want 1", count)
	}

	// Rebuild without --embed clears previously stored embeddings.
	summary, err = idx.RebuildWithOptions("research", RebuildOptions{Embedding: false})
	if err != nil {
		t.Fatalf("RebuildWithOptions(no embed again) error = %v", err)
	}
	count, err = (EmbeddingStore{Paths: paths}).Count("research")
	if err != nil {
		t.Fatalf("Count() after rebuild error = %v", err)
	}
	if count != 0 {
		t.Fatalf("embedding count after plain rebuild = %d, want 0", count)
	}
}

func TestLoopbackEmbedderRebuildPreservesSchema(t *testing.T) {
	paths := indexTestPaths(t)
	claimStore := ClaimStore{Paths: paths, Now: fixedIndexNow}
	approved := indexClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Loopback Schema Memory", ClaimBasisOwner)
	if _, err := claimStore.WriteDraft("research", approved); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := claimStore.Approve("research", approved.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.RebuildWithOptions("research", RebuildOptions{Embedding: true}); err != nil {
		t.Fatalf("RebuildWithOptions() error = %v", err)
	}
	// The main index database must remain readable with the same schema version.
	if err := idx.CheckFresh("research"); err != nil {
		t.Fatalf("CheckFresh() error = %v", err)
	}
	databasePath, err := idx.DatabasePath("research")
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow("pragma user_version").Scan(&version); err != nil {
		t.Fatalf("user_version scan error = %v", err)
	}
	if version != indexSchemaVersion {
		t.Fatalf("user_version = %d, want %d (no schema bump)", version, indexSchemaVersion)
	}
}
