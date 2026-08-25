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

func TestIndexStateNilAndValidation(t *testing.T) {
	// Nil checks for WriteIndexState, ReadIndexState, ReadIndexStateTx
	if err := WriteIndexState(nil, TrustInputManifest{}, RebuildState{}); err == nil {
		t.Fatalf("WriteIndexState(nil) error = nil")
	}
	if _, _, err := ReadIndexState(nil); err == nil {
		t.Fatalf("ReadIndexState(nil) error = nil")
	}
	if _, _, err := ReadIndexStateTx(nil); err == nil {
		t.Fatalf("ReadIndexStateTx(nil) error = nil")
	}
	// Direct validate functions
	if err := validateTrustInput(TrustInput{Path: "  ", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)}); err == nil {
		t.Fatalf("validateTrustInput(blank path) error = nil")
	}
	if err := validateTrustInput(TrustInput{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: -1, SHA256: strings.Repeat("a", 64)}); err == nil {
		t.Fatalf("validateTrustInput(negative length) error = nil")
	}
	if err := validateTrustInput(TrustInput{Path: "wiki/projects/a.md", Kind: "unknown", ByteLength: 1, SHA256: strings.Repeat("a", 64)}); err == nil {
		t.Fatalf("validateTrustInput(unknown kind) error = nil")
	}
	// validateTrustInput evidence path
	if err := validateTrustInput(TrustInput{Path: "evidence/sources/ev_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/raw", Kind: TrustInputKindEvidenceRaw, ByteLength: 1, SHA256: strings.Repeat("b", 64)}); err != nil {
		t.Fatalf("validateTrustInput(valid evidence raw) error = %v", err)
	}
	if err := validateTrustInput(TrustInput{Path: "evidence/sources/ev_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/wrong", Kind: TrustInputKindEvidenceRaw, ByteLength: 1, SHA256: strings.Repeat("b", 64)}); err == nil {
		t.Fatalf("validateTrustInput(bad evidence path) error = nil")
	}
	// validateRebuildState
	validDigest := strings.Repeat("a", 64)
	if err := validateRebuildState(RebuildState{Status: RebuildStatusClean, InvalidCount: 0, ManifestDigest: validDigest, RebuiltAt: "2026-08-04T05:00:00Z"}, validDigest); err != nil {
		t.Fatalf("validateRebuildState(clean valid) error = %v", err)
	}
	if err := validateRebuildState(RebuildState{Status: RebuildStatusClean, InvalidCount: 1, ManifestDigest: validDigest, RebuiltAt: "2026-08-04T05:00:00Z"}, validDigest); err == nil {
		t.Fatalf("validateRebuildState(clean with count) error = nil")
	}
	if err := validateRebuildState(RebuildState{Status: RebuildStatusRejected, InvalidCount: 0, ManifestDigest: validDigest, RebuiltAt: "2026-08-04T05:00:00Z"}, validDigest); err == nil {
		t.Fatalf("validateRebuildState(rejected zero) error = nil")
	}
	if err := validateRebuildState(RebuildState{Status: RebuildStatusRejected, InvalidCount: 1, ManifestDigest: "bad", RebuiltAt: "2026-08-04T05:00:00Z"}, validDigest); err == nil {
		t.Fatalf("validateRebuildState(bad digest) error = nil")
	}
	if err := validateRebuildState(RebuildState{Status: RebuildStatusClean, InvalidCount: 0, ManifestDigest: validDigest, RebuiltAt: "not-rfc3339"}, validDigest); err == nil {
		t.Fatalf("validateRebuildState(bad time) error = nil")
	}
	if err := validateRebuildState(RebuildState{Status: "unknown", InvalidCount: 0, ManifestDigest: validDigest, RebuiltAt: "2026-08-04T05:00:00Z"}, validDigest); err == nil {
		t.Fatalf("validateRebuildState(unknown status) error = nil")
	}
	// validateTrustInputManifest
	emptyManifest := TrustInputManifest{Entries: nil, Digest: trustInputManifestDigest(nil)}
	if err := validateTrustInputManifest(emptyManifest); err != nil {
		t.Fatalf("validateTrustInputManifest(empty) error = %v", err)
	}
	badManifest := TrustInputManifest{Entries: []TrustInput{{Path: "wiki/projects/b.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("b", 64)}, {Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)}}, Digest: "bad"}
	if err := validateTrustInputManifest(badManifest); err == nil {
		t.Fatalf("validateTrustInputManifest(unsorted) error = nil")
	}
}

func TestIndexStateSchemaValidation(t *testing.T) {
	// Test validateIndexStateSchema with wrong version
	db := newIndexStateTestDB(t)
	if _, err := db.Exec("pragma user_version = 99"); err != nil {
		t.Fatalf("pragma user_version error = %v", err)
	}
	if err := validateIndexStateSchema(db); err == nil {
		t.Fatalf("validateIndexStateSchema(wrong version) error = nil")
	}
	// Reset to correct version for next tests
	if _, err := db.Exec("pragma user_version = 3"); err != nil {
		t.Fatalf("reset version error = %v", err)
	}
	// Test requireIndexStateColumns with missing column (drop and recreate without not null)
	if _, err := db.Exec("drop table trust_inputs; create table trust_inputs (path text, kind text, byte_length integer, sha256 text)"); err != nil {
		t.Fatalf("recreate trust_inputs without not null error = %v", err)
	}
	if err := requireIndexStateColumns(db, "trust_inputs", map[string]string{"path": "text", "kind": "text", "byte_length": "integer", "sha256": "text"}); err == nil {
		t.Fatalf("requireIndexStateColumns(missing not null) error = nil")
	}
	// Test with wrong type
	db2 := newIndexStateTestDB(t)
	if _, err := db2.Exec("drop table trust_inputs; create table trust_inputs (path integer not null, kind text not null, byte_length integer not null, sha256 text not null)"); err != nil {
		t.Fatalf("recreate trust_inputs wrong type error = %v", err)
	}
	if err := requireIndexStateColumns(db2, "trust_inputs", map[string]string{"path": "text", "kind": "text", "byte_length": "integer", "sha256": "text"}); err == nil {
		t.Fatalf("requireIndexStateColumns(wrong type) error = nil")
	}
	// Test with missing column
	db3 := newIndexStateTestDB(t)
	if _, err := db3.Exec("drop table trust_inputs; create table trust_inputs (path text not null, kind text not null)"); err != nil {
		t.Fatalf("recreate trust_inputs missing columns error = %v", err)
	}
	if err := requireIndexStateColumns(db3, "trust_inputs", map[string]string{"path": "text", "kind": "text", "byte_length": "integer", "sha256": "text"}); err == nil {
		t.Fatalf("requireIndexStateColumns(missing column) error = nil")
	}
}

func TestIndexStateReadErrors(t *testing.T) {
	// Test readTrustInputManifest with entries that will be read sorted - should succeed
	db := newIndexStateTestDB(t)
	manifest := indexStateManifest([]TrustInput{
		{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)},
		{Path: "wiki/projects/b.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("b", 64)},
	})
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := tx.Exec("insert into trust_inputs(path, kind, byte_length, sha256) values (?, ?, ?, ?)", "wiki/projects/a.md", TrustInputKindClaim, 1, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("insert a error = %v", err)
	}
	if _, err := tx.Exec("insert into trust_inputs(path, kind, byte_length, sha256) values (?, ?, ?, ?)", "wiki/projects/b.md", TrustInputKindClaim, 1, strings.Repeat("b", 64)); err != nil {
		t.Fatalf("insert b error = %v", err)
	}
	if _, err := tx.Exec("insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (1, 'clean', 0, ?, '2026-08-04T05:00:00Z')", manifest.Digest); err != nil {
		t.Fatalf("insert rebuild_state error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if _, _, err := ReadIndexState(db); err != nil {
		t.Fatalf("ReadIndexState(valid sorted) error = %v", err)
	}
	// Now test with bad sha to trigger validation error
	dbBad := newIndexStateTestDB(t)
	tx2, err := dbBad.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := tx2.Exec("insert into trust_inputs(path, kind, byte_length, sha256) values (?, ?, ?, ?)", "wiki/projects/a.md", TrustInputKindClaim, 1, "bad-sha"); err != nil {
		t.Fatalf("insert bad sha error = %v", err)
	}
	if _, err := tx2.Exec("insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (1, 'clean', 0, ?, '2026-08-04T05:00:00Z')", manifest.Digest); err != nil {
		t.Fatalf("insert rebuild_state error = %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if _, _, err := ReadIndexState(dbBad); err == nil {
		t.Fatalf("ReadIndexState(bad sha) error = nil")
	}
	// Test readRebuildState with duplicate rows
	db2 := newIndexStateTestDB(t)
	manifest2 := indexStateManifest(nil)
	if _, err := db2.Exec("drop table rebuild_state; create table rebuild_state (id integer not null, status text not null, invalid_count integer not null, manifest_digest text not null, rebuilt_at text not null)"); err != nil {
		t.Fatalf("drop rebuild_state error = %v", err)
	}
	if _, err := db2.Exec("insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (1, 'clean', 0, ?, '2026-08-04T05:00:00Z')", manifest2.Digest); err != nil {
		t.Fatalf("insert rebuild_state 1 error = %v", err)
	}
	if _, err := db2.Exec("insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (2, 'clean', 0, ?, '2026-08-04T05:00:00Z')", manifest2.Digest); err != nil {
		t.Fatalf("insert rebuild_state 2 error = %v", err)
	}
	if _, _, err := ReadIndexState(db2); err == nil {
		t.Fatalf("ReadIndexState(duplicate rebuild_state) error = nil")
	}
	// Test readRebuildState with wrong singleton id
	db3 := newIndexStateTestDB(t)
	if _, err := db3.Exec("drop table rebuild_state; create table rebuild_state (id integer not null primary key, status text not null, invalid_count integer not null, manifest_digest text not null, rebuilt_at text not null)"); err != nil {
		t.Fatalf("drop rebuild_state error = %v", err)
	}
	if _, err := db3.Exec("delete from rebuild_state"); err != nil {
		t.Fatalf("delete rebuild_state error = %v", err)
	}
	if _, err := db3.Exec("insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (2, 'clean', 0, ?, '2026-08-04T05:00:00Z')", manifest2.Digest); err != nil {
		t.Fatalf("insert wrong id error = %v", err)
	}
	if _, _, err := ReadIndexState(db3); err == nil {
		t.Fatalf("ReadIndexState(wrong id) error = nil")
	}
}

func TestIndexStateWriteErrors(t *testing.T) {
	db := newIndexStateTestDB(t)
	// Test writeIndexState with invalid manifest (unsorted)
	unsorted := TrustInputManifest{
		Entries: []TrustInput{
			{Path: "wiki/projects/b.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("b", 64)},
			{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)},
		},
		Digest: strings.Repeat("a", 64),
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := WriteIndexState(tx, unsorted, cleanIndexState(unsorted)); err == nil {
		t.Fatalf("WriteIndexState(unsorted) error = nil")
	}
	_ = tx.Rollback()
	// Test with invalid state (clean with count)
	tx2, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	validManifest := indexStateManifest(nil)
	badState := RebuildState{Status: RebuildStatusClean, InvalidCount: 1, ManifestDigest: validManifest.Digest, RebuiltAt: "2026-08-04T05:00:00Z"}
	if err := WriteIndexState(tx2, validManifest, badState); err == nil {
		t.Fatalf("WriteIndexState(bad state) error = nil")
	}
	_ = tx2.Rollback()
	// Test with nil tx validate schema error (simulate by using db with wrong version)
	db4 := newIndexStateTestDB(t)
	if _, err := db4.Exec("pragma user_version = 99"); err != nil {
		t.Fatalf("pragma error = %v", err)
	}
	tx3, err := db4.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := WriteIndexState(tx3, validManifest, cleanIndexState(validManifest)); err == nil {
		t.Fatalf("WriteIndexState(wrong version) error = nil")
	}
	_ = tx3.Rollback()
}

func TestIndexStateSchemaOtherTables(t *testing.T) {
	db := newIndexStateTestDB(t)
	// rebuild_state without not null
	if _, err := db.Exec("drop table rebuild_state; create table rebuild_state (id integer, status text, invalid_count integer, manifest_digest text, rebuilt_at text)"); err != nil {
		t.Fatalf("drop rebuild_state error = %v", err)
	}
	if err := validateIndexStateSchema(db); err == nil {
		t.Fatalf("validateIndexStateSchema(rebuild_state no not null) error = nil")
	}
	db2 := newIndexStateTestDB(t)
	if _, err := db2.Exec("drop table trust_input_mtimes; create table trust_input_mtimes (path integer not null, modified_at integer not null, change_token integer not null)"); err != nil {
		t.Fatalf("drop trust_input_mtimes error = %v", err)
	}
	if err := validateIndexStateSchema(db2); err == nil {
		t.Fatalf("validateIndexStateSchema(trust_input_mtimes wrong type) error = nil")
	}
	db3 := newIndexStateTestDB(t)
	if _, err := db3.Exec("drop table trust_directories; create table trust_directories (path text not null)"); err != nil {
		t.Fatalf("drop trust_directories error = %v", err)
	}
	if err := validateIndexStateSchema(db3); err == nil {
		t.Fatalf("validateIndexStateSchema(trust_directories missing cols) error = nil")
	}
	// Test readTrustInputManifest with bad entry (invalid sha)
	db4 := newIndexStateTestDB(t)
	manifest := indexStateManifest([]TrustInput{{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)}})
	writeIndexStateTestState(t, db4, manifest, cleanIndexState(manifest))
	if _, err := db4.Exec("update trust_inputs set sha256 = 'bad'"); err != nil {
		t.Fatalf("update bad sha error = %v", err)
	}
	if _, _, err := ReadIndexState(db4); err == nil {
		t.Fatalf("ReadIndexState(bad sha) error = nil")
	}
	// Test readRebuildState with bad status
	db5 := newIndexStateTestDB(t)
	writeIndexStateTestState(t, db5, manifest, cleanIndexState(manifest))
	if _, err := db5.Exec("update rebuild_state set status = 'bad'"); err != nil {
		t.Fatalf("update bad status error = %v", err)
	}
	if _, _, err := ReadIndexState(db5); err == nil {
		t.Fatalf("ReadIndexState(bad status) error = nil")
	}
	// Test validateTrustInputManifest with duplicate path
	dupManifest := TrustInputManifest{
		Entries: []TrustInput{
			{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)},
			{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)},
		},
		Digest: trustInputManifestDigest([]TrustInput{
			{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)},
			{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)},
		}),
	}
	if err := validateTrustInputManifest(dupManifest); err == nil {
		t.Fatalf("validateTrustInputManifest(duplicate) error = nil")
	}
	// Test isSHA256Digest
	if isSHA256Digest(strings.ToUpper(strings.Repeat("a", 64))) {
		t.Fatalf("isSHA256Digest(upper) = true, want false")
	}
	if !isSHA256Digest(strings.Repeat("a", 64)) {
		t.Fatalf("isSHA256Digest(valid) = false")
	}
}
