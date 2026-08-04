package runtime

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

func TestIndexRebuildState(t *testing.T) {
	db := newIndexStateTestDB(t)
	entries := []TrustInput{
		{Path: "wiki/projects/clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.md", Kind: TrustInputKindClaim, ByteLength: 12, SHA256: strings.Repeat("a", 64)},
	}
	manifest := indexStateManifest(entries)
	state := RebuildState{
		Status:         RebuildStatusClean,
		InvalidCount:   0,
		ManifestDigest: manifest.Digest,
		RebuiltAt:      "2026-08-04T05:00:00Z",
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := WriteIndexState(tx, manifest, state); err != nil {
		t.Fatalf("WriteIndexState() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	gotManifest, gotState, err := ReadIndexState(db)
	if err != nil {
		t.Fatalf("ReadIndexState() error = %v", err)
	}
	if !reflect.DeepEqual(gotManifest, manifest) {
		t.Fatalf("manifest = %#v, want %#v", gotManifest, manifest)
	}
	if !reflect.DeepEqual(gotState, state) {
		t.Fatalf("state = %#v, want %#v", gotState, state)
	}
}

func TestIndexStateTransactionRollback(t *testing.T) {
	db := newIndexStateTestDB(t)
	manifest := indexStateManifest(nil)
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := WriteIndexState(tx, manifest, cleanIndexState(manifest)); err != nil {
		t.Fatalf("WriteIndexState() error = %v", err)
	}
	if _, _, err := ReadIndexStateTx(tx); err != nil {
		t.Fatalf("ReadIndexStateTx() error = %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if _, _, err := ReadIndexState(db); err == nil {
		t.Fatal("ReadIndexState() error = nil after rollback, want missing state")
	}
}

func TestIndexTrustInputs(t *testing.T) {
	db := newIndexStateTestDB(t)
	entries := []TrustInput{
		{Path: "evidence/sources/ev_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/raw", Kind: TrustInputKindEvidenceRaw, ByteLength: 20, SHA256: strings.Repeat("b", 64)},
		{Path: "evidence/sources/ev_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/source.yaml", Kind: TrustInputKindEvidenceMetadata, ByteLength: 21, SHA256: strings.Repeat("c", 64)},
		{Path: "wiki/axioms/clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.md", Kind: TrustInputKindClaim, ByteLength: 22, SHA256: strings.Repeat("d", 64)},
	}
	entries = []TrustInput{entries[2], entries[1], entries[0]}
	manifest := indexStateManifest(entries)
	state := cleanIndexState(manifest)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := WriteIndexState(tx, manifest, state); err != nil {
		t.Fatalf("WriteIndexState() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	var count int
	if err := db.QueryRow("select count(*) from trust_inputs").Scan(&count); err != nil {
		t.Fatalf("count trust_inputs error = %v", err)
	}
	if count != len(entries) {
		t.Fatalf("trust_inputs count = %d, want %d", count, len(entries))
	}
	gotManifest, _, err := ReadIndexState(db)
	if err != nil {
		t.Fatalf("ReadIndexState() error = %v", err)
	}
	if gotManifest.Digest != manifest.Digest || !reflect.DeepEqual(gotManifest.Entries, manifest.Entries) {
		t.Fatalf("manifest = %#v, want %#v", gotManifest, manifest)
	}
}

func TestIndexStateMissing(t *testing.T) {
	db := newIndexStateTestDB(t)
	if _, err := db.Exec("delete from rebuild_state"); err != nil {
		t.Fatalf("delete rebuild_state error = %v", err)
	}
	if _, _, err := ReadIndexState(db); err == nil {
		t.Fatal("ReadIndexState() error = nil, want missing rebuild state error")
	}

	oldDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open(old schema) error = %v", err)
	}
	defer oldDB.Close()
	if _, err := oldDB.Exec("create table claims(id text)"); err != nil {
		t.Fatalf("create old schema error = %v", err)
	}
	if _, _, err := ReadIndexState(oldDB); err == nil {
		t.Fatal("ReadIndexState(old schema) error = nil, want fail-closed error")
	}
}

func TestIndexStateRejected(t *testing.T) {
	db := newIndexStateTestDB(t)
	manifest := indexStateManifest(nil)
	state := RebuildState{
		Status:         RebuildStatusRejected,
		InvalidCount:   2,
		ManifestDigest: manifest.Digest,
		RebuiltAt:      "2026-08-04T05:00:00Z",
	}
	writeIndexStateTestState(t, db, manifest, state)

	_, gotState, err := ReadIndexState(db)
	if err != nil {
		t.Fatalf("ReadIndexState() error = %v", err)
	}
	if gotState != state {
		t.Fatalf("state = %#v, want %#v", gotState, state)
	}

	if _, err := db.Exec("update rebuild_state set status = 'unknown'"); err != nil {
		t.Fatalf("set unknown status error = %v", err)
	}
	if _, _, err := ReadIndexState(db); err == nil {
		t.Fatal("ReadIndexState(unknown status) error = nil, want fail-closed error")
	}
}

func TestIndexStateRejectsMalformedRows(t *testing.T) {
	tests := []struct {
		name   string
		update string
	}{
		{name: "negative-byte-length", update: "update trust_inputs set byte_length = -1"},
		{name: "bad-digest", update: "update trust_inputs set sha256 = 'not-a-digest'"},
		{name: "unknown-kind", update: "update trust_inputs set kind = 'unknown'"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newIndexStateTestDB(t)
			manifest := indexStateManifest([]TrustInput{{
				Path:       "wiki/projects/clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.md",
				Kind:       TrustInputKindClaim,
				ByteLength: 12,
				SHA256:     strings.Repeat("a", 64),
			}})
			writeIndexStateTestState(t, db, manifest, cleanIndexState(manifest))
			if _, err := db.Exec(test.update); err != nil {
				t.Fatalf("malformed update error = %v", err)
			}
			if _, _, err := ReadIndexState(db); err == nil {
				t.Fatal("ReadIndexState() error = nil, want fail-closed error")
			}
		})
	}

	db := newIndexStateTestDB(t)
	manifest := indexStateManifest(nil)
	writeIndexStateTestState(t, db, manifest, cleanIndexState(manifest))
	if _, err := db.Exec("drop table rebuild_state; create table rebuild_state (id integer not null, status text not null, invalid_count integer not null, manifest_digest text not null, rebuilt_at text not null)"); err != nil {
		t.Fatalf("replace rebuild_state error = %v", err)
	}
	for i := 1; i <= 2; i++ {
		if _, err := db.Exec("insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (?, 'clean', 0, ?, '2026-08-04T05:00:00Z')", i, manifest.Digest); err != nil {
			t.Fatalf("insert duplicate rebuild state error = %v", err)
		}
	}
	if _, _, err := ReadIndexState(db); err == nil {
		t.Fatal("ReadIndexState(duplicate state) error = nil, want fail-closed error")
	}
}

func TestIndexStateWriteValidation(t *testing.T) {
	db := newIndexStateTestDB(t)
	validManifest := indexStateManifest(nil)
	validState := cleanIndexState(validManifest)
	invalidManifests := []TrustInputManifest{
		{Entries: []TrustInput{{Path: "wiki/projects/note.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)}}, Digest: strings.Repeat("a", 64)},
		{Entries: []TrustInput{{Path: "wiki/projects/note.md", Kind: "unknown", ByteLength: 1, SHA256: strings.Repeat("a", 64)}}, Digest: strings.Repeat("a", 64)},
	}
	for i, manifest := range invalidManifests {
		t.Run("manifest-"+string(rune('a'+i)), func(t *testing.T) {
			tx, err := db.Begin()
			if err != nil {
				t.Fatalf("Begin() error = %v", err)
			}
			if err := WriteIndexState(tx, manifest, validState); err == nil {
				t.Fatal("WriteIndexState() error = nil, want validation error")
			}
			if err := tx.Rollback(); err != nil {
				t.Fatalf("Rollback() error = %v", err)
			}
		})
	}

	badState := validState
	badState.Status = RebuildStatus("unknown")
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := WriteIndexState(tx, validManifest, badState); err == nil {
		t.Fatal("WriteIndexState(unknown status) error = nil, want validation error")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
}

func newIndexStateTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := createIndexSchema(db); err != nil {
		t.Fatalf("createIndexSchema() error = %v", err)
	}
	return db
}

func indexStateManifest(entries []TrustInput) TrustInputManifest {
	sorted := make([]TrustInput, len(entries))
	copy(sorted, entries)
	// Manifest construction is intentionally independent from the caller's order.
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Path < sorted[i].Path {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return TrustInputManifest{Entries: sorted, Digest: trustInputManifestDigest(sorted)}
}

func cleanIndexState(manifest TrustInputManifest) RebuildState {
	return RebuildState{
		Status:         RebuildStatusClean,
		ManifestDigest: manifest.Digest,
		RebuiltAt:      "2026-08-04T05:00:00Z",
	}
}

func writeIndexStateTestState(t *testing.T, db *sql.DB, manifest TrustInputManifest, state RebuildState) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := WriteIndexState(tx, manifest, state); err != nil {
		t.Fatalf("WriteIndexState() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
}
