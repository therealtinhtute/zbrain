package runtime

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// LoopbackEmbedder is a local-only, deterministic embedder that produces
// fixed-dimension vectors from text using only local computation. It never
// contacts a network service.
type LoopbackEmbedder struct {
	Dimension int
	once      sync.Once
}

// NewLoopbackEmbedder returns a LoopbackEmbedder with the default dimension.
func NewLoopbackEmbedder() *LoopbackEmbedder {
	return &LoopbackEmbedder{Dimension: 384}
}

// Embed produces deterministic vectors for each input text. Each vector has
// Dimension components and is L2-normalized. No network I/O is performed.
func (e *LoopbackEmbedder) Embed(texts []string) ([][]float32, error) {
	e.once.Do(func() {
		if e.Dimension <= 0 {
			e.Dimension = 384
		}
	})
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vectors[i] = e.embedOne(text)
	}
	return vectors, nil
}

func (e *LoopbackEmbedder) embedOne(text string) []float32 {
	vec := make([]float32, e.Dimension)
	tokens := loopbackTokens(text)
	for _, token := range tokens {
		h := sha256.Sum256([]byte(token))
		// Use first 4 bytes as a uint32 index, then accumulate.
		idx := binary.LittleEndian.Uint32(h[:4]) % uint32(e.Dimension)
		vec[idx] += 1.0
		// Use second 4 bytes for a second bucket (bigram-like spread).
		idx2 := binary.LittleEndian.Uint32(h[4:8]) % uint32(e.Dimension)
		vec[idx2] += 0.5
	}
	// L2 normalize.
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}
	return vec
}

// loopbackTokens splits text into lowercased tokens, similar to queryTokens
// but kept independent so the embedder has no dependency on the search package.
func loopbackTokens(text string) []string {
	lower := strings.ToLower(text)
	fields := strings.FieldsFunc(lower, func(r rune) bool {
		return !isAlphaNum(r)
	})
	dedup := make(map[string]bool, len(fields))
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		if f == "" || dedup[f] {
			continue
		}
		dedup[f] = true
		tokens = append(tokens, f)
	}
	return tokens
}

func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z')
}

// EmbeddingStore manages embedding vectors in a separate SQLite file within
// the indexes directory. It does not modify the main index schema or bump
// user_version.
type EmbeddingStore struct {
	Paths Paths
}

// EmbeddingRecord represents one stored embedding vector.
type EmbeddingRecord struct {
	ClaimID   string
	Dimension int
	Vector    []float32
}

// DatabasePath returns the path to the embedding SQLite file for a workspace.
func (store EmbeddingStore) DatabasePath(workspace string) string {
	return filepath.Join(store.Paths.IndexesDir, workspace+".embeddings.sqlite")
}

// Close removes the embedding database for a workspace.
func (store EmbeddingStore) Close(workspace string) error {
	path := store.DatabasePath(workspace)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// StoreVectors writes embedding vectors for the given claims. Any existing
// embeddings for the workspace are replaced atomically via rename.
func (store EmbeddingStore) StoreVectors(workspace string, records []EmbeddingRecord) error {
	if len(records) == 0 {
		return store.Close(workspace)
	}
	if err := ensureDirectoryMode(store.Paths.IndexesDir, runtimeDirectoryMode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(store.Paths.IndexesDir, workspace+".embeddings.sqlite.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Close(); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`create table embeddings (
		claim_id text not null primary key,
		vector blob not null,
		dimension integer not null
	)`); err != nil {
		return fmt.Errorf("create embeddings table: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, rec := range records {
		vec := make([]byte, 4*len(rec.Vector))
		for i, v := range rec.Vector {
			binary.LittleEndian.PutUint32(vec[4*i:], math.Float32bits(v))
		}
		if _, err := tx.Exec("insert into embeddings(claim_id, vector, dimension) values (?, ?, ?)",
			rec.ClaimID, vec, rec.Dimension); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store embedding for %q: %w", rec.ClaimID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}
	if err := ensureFileMode(tmpPath, derivedIndexMode); err != nil {
		return err
	}
	target := store.DatabasePath(workspace)
	return os.Rename(tmpPath, target)
}

// SearchVectors returns the top-k most similar claim IDs for the given query.
// Returns nil, nil when no embedding database exists (caller falls back to lexical).
func (store EmbeddingStore) SearchVectors(workspace, query string, limit int) ([]string, error) {
	path := store.DatabasePath(workspace)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query("select claim_id, vector, dimension from embeddings")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type storedVector struct {
		claimID string
		vector  []float32
	}
	var stored []storedVector
	for rows.Next() {
		var claimID string
		var blob []byte
		var dim int
		if err := rows.Scan(&claimID, &blob, &dim); err != nil {
			return nil, err
		}
		if len(blob) != 4*dim {
			return nil, fmt.Errorf("embedding vector byte length mismatch for %q: %d bytes, want %d", claimID, len(blob), 4*dim)
		}
		vec := make([]float32, dim)
		for i := range vec {
			vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[4*i:]))
		}
		stored = append(stored, storedVector{claimID: claimID, vector: vec})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(stored) == 0 {
		return nil, nil
	}
	embedder := NewLoopbackEmbedder()
	queryVecs, err := embedder.Embed([]string{query})
	if err != nil {
		return nil, err
	}
	queryVec := queryVecs[0]
	type scoredClaim struct {
		id    string
		score float64
	}
	scored := make([]scoredClaim, len(stored))
	for i, s := range stored {
		if len(s.vector) != len(queryVec) {
			return nil, fmt.Errorf("embedding dimension mismatch for %q: %d, want %d", s.claimID, len(s.vector), len(queryVec))
		}
		var dot float64
		for j := range queryVec {
			dot += float64(queryVec[j]) * float64(s.vector[j])
		}
		scored[i] = scoredClaim{id: s.claimID, score: dot}
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].id < scored[j].id
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	ids := make([]string, len(scored))
	for i, s := range scored {
		ids[i] = s.id
	}
	return ids, nil
}

// Count returns the number of stored embeddings for a workspace.
func (store EmbeddingStore) Count(workspace string) (int, error) {
	path := store.DatabasePath(workspace)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()
	var count int
	if err := db.QueryRow("select count(*) from embeddings").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// Summary reports the optional embedding sidecar coverage without making it a
// prerequisite for lexical retrieval or workspace status.
func (store EmbeddingStore) Summary(workspace string, eligible int) EmbeddingSummary {
	summary := EmbeddingSummary{
		Strategy: "lexical",
		Degraded: "embeddings not configured",
	}
	count, err := store.Count(workspace)
	if err != nil {
		summary.Degraded = "embedding sidecar unavailable"
		return summary
	}
	if count == 0 {
		return summary
	}
	summary.Strategy = "loopback"
	summary.Model = "zbrain/loopback-v1"
	summary.Indexed = count
	summary.Eligible = eligible
	summary.Degraded = ""
	return summary
}

// RebuildOptions controls optional behavior during index rebuild.
type RebuildOptions struct {
	// Embedding enables loopback embedding during rebuild.
	Embedding bool
}

// RebuildWithOptions rebuilds the index with optional embedding.
func (store IndexStore) RebuildWithOptions(workspace string, opts RebuildOptions) (IndexSummary, error) {
	summary, err := store.Rebuild(workspace)
	if err != nil {
		return summary, err
	}
	if !opts.Embedding {
		// A non-embedding rebuild must not leave stale vectors behind; the
		// derived index changed and old embeddings no longer correspond to it.
		if err := (EmbeddingStore(store)).Close(workspace); err != nil {
			return summary, fmt.Errorf("clear stale embeddings: %w", err)
		}
		return summary, nil
	}
	return store.embedIndexedClaims(workspace, summary)
}

// embedIndexedClaims opens the published index, reads all approved claims,
// runs the loopback embedder, and stores the vectors.
func (store IndexStore) embedIndexedClaims(workspace string, summary IndexSummary) (IndexSummary, error) {
	databasePath, err := store.DatabasePath(workspace)
	if err != nil {
		return summary, err
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return summary, err
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query("select id, title, description, tags, body from claims where status = ?", string(ClaimStatusApproved))
	if err != nil {
		return summary, err
	}
	defer func() { _ = rows.Close() }()
	type claimText struct {
		ID    string
		Title string
		Desc  string
		Tags  string
		Body  string
	}
	var claims []claimText
	for rows.Next() {
		var c claimText
		if err := rows.Scan(&c.ID, &c.Title, &c.Desc, &c.Tags, &c.Body); err != nil {
			return summary, err
		}
		claims = append(claims, c)
	}
	if err := rows.Err(); err != nil {
		return summary, err
	}
	if len(claims) == 0 {
		_ = (EmbeddingStore(store)).Close(workspace)
		summary.Embedding.Indexed = 0
		summary.Embedding.Eligible = 0
		summary.Embedding.Strategy = "loopback"
		summary.Embedding.Model = "zbrain/loopback-v1"
		summary.Embedding.Degraded = ""
		return summary, nil
	}
	texts := make([]string, len(claims))
	for i, c := range claims {
		texts[i] = c.Title + " " + c.Desc + " " + c.Tags + " " + c.Body
	}
	embedder := NewLoopbackEmbedder()
	vectors, err := embedder.Embed(texts)
	if err != nil {
		return summary, fmt.Errorf("embed claims: %w", err)
	}
	records := make([]EmbeddingRecord, len(claims))
	for i, c := range claims {
		records[i] = EmbeddingRecord{
			ClaimID:   c.ID,
			Dimension: embedder.Dimension,
			Vector:    vectors[i],
		}
	}
	embStore := EmbeddingStore(store)
	if err := embStore.StoreVectors(workspace, records); err != nil {
		return summary, fmt.Errorf("store embeddings: %w", err)
	}
	summary.Embedding.Indexed = len(records)
	summary.Embedding.Eligible = len(records)
	summary.Embedding.Strategy = "loopback"
	summary.Embedding.Model = "zbrain/loopback-v1"
	summary.Embedding.Degraded = ""
	return summary, nil
}
