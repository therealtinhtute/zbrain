//! index_state.rs — port of internal/runtime/index_state.go: the published
//! trust-input manifest and rebuild outcome stored inside the index database.

use std::collections::HashSet;

use rusqlite::Connection;
use serde::Serialize;

use crate::index::IndexError;
use crate::manifest::{TrustInput, TrustInputManifest, trust_input_manifest_digest};

pub const INDEX_SCHEMA_VERSION: i64 = 3;
pub const INDEX_STATE_SINGLETON: i64 = 1;

pub const REBUILD_STATUS_CLEAN: &str = "clean";
pub const REBUILD_STATUS_REJECTED: &str = "rejected";

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct RebuildState {
    pub status: String,
    pub invalid_count: i64,
    pub manifest_digest: String,
    pub rebuilt_at: String,
}

/// WriteIndexState replaces the manifest and rebuild outcome in the caller's
/// transaction. The transaction must be committed by the caller to publish
/// both together.
pub fn write_index_state(
    tx: &rusqlite::Transaction<'_>,
    manifest: &TrustInputManifest,
    state: &RebuildState,
) -> Result<(), IndexError> {
    validate_index_state_schema(tx)?;
    validate_trust_input_manifest(manifest)
        .map_err(|err| IndexError::Message(format!("validate trust input manifest: {err}")))?;
    validate_rebuild_state(state, &manifest.digest)
        .map_err(|err| IndexError::Message(format!("validate rebuild state: {err}")))?;

    tx.execute("delete from trust_inputs", [])
        .map_err(|err| IndexError::Message(format!("clear trust inputs: {err}")))?;
    for entry in &manifest.entries {
        tx.execute(
            "insert into trust_inputs(path, kind, byte_length, sha256) values (?, ?, ?, ?)",
            rusqlite::params![entry.path, entry.kind, entry.byte_length, entry.sha256],
        )
        .map_err(|err| {
            IndexError::Message(format!("write trust input {:?}: {err}", entry.path))
        })?;
    }
    tx.execute("delete from rebuild_state", [])
        .map_err(|err| IndexError::Message(format!("clear rebuild state: {err}")))?;
    tx.execute(
        "insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (?, ?, ?, ?, ?)",
        rusqlite::params![
            INDEX_STATE_SINGLETON,
            state.status,
            state.invalid_count,
            state.manifest_digest,
            state.rebuilt_at
        ],
    )
    .map_err(|err| IndexError::Message(format!("write rebuild state: {err}")))?;
    Ok(())
}

/// ReadIndexState reads and validates one published index generation.
pub fn read_index_state(
    conn: &Connection,
) -> Result<(TrustInputManifest, RebuildState), IndexError> {
    validate_index_state_schema(conn)?;
    let manifest = read_trust_input_manifest(conn)
        .map_err(|err| IndexError::Message(format!("{err}")))?;
    let state = read_rebuild_state(conn).map_err(|err| IndexError::Message(format!("{err}")))?;
    validate_rebuild_state(&state, &manifest.digest)
        .map_err(|err| IndexError::Message(format!("validate rebuild state: {err}")))?;
    Ok((manifest, state))
}

/// ReadIndexStateTx reads and validates state visible inside a transaction.
/// A Transaction dereferences to Connection, so callers pass `&tx`.
pub fn read_index_state_tx(
    tx: &rusqlite::Transaction<'_>,
) -> Result<(TrustInputManifest, RebuildState), IndexError> {
    read_index_state(tx)
}

fn validate_index_state_schema(conn: &Connection) -> Result<(), IndexError> {
    let version: i64 = conn
        .query_row("pragma user_version", [], |row| row.get(0))
        .map_err(|err| IndexError::Message(format!("read index schema version: {err}")))?;
    if version != INDEX_SCHEMA_VERSION {
        return Err(IndexError::Message(format!(
            "unsupported index schema version {version}; want {INDEX_SCHEMA_VERSION}"
        )));
    }
    require_index_state_columns(
        conn,
        "trust_inputs",
        &[
            ("path", "text"),
            ("kind", "text"),
            ("byte_length", "integer"),
            ("sha256", "text"),
        ],
    )
    .map_err(|err| IndexError::Message(format!("validate trust_inputs schema: {err}")))?;
    require_index_state_columns(
        conn,
        "trust_input_mtimes",
        &[
            ("path", "text"),
            ("modified_at", "integer"),
            ("change_token", "integer"),
        ],
    )
    .map_err(|err| {
        IndexError::Message(format!("validate trust input freshness schema: {err}"))
    })?;
    require_index_state_columns(
        conn,
        "trust_directories",
        &[
            ("path", "text"),
            ("modified_at", "integer"),
            ("change_token", "integer"),
        ],
    )
    .map_err(|err| {
        IndexError::Message(format!("validate trust directory freshness schema: {err}"))
    })?;
    require_index_state_columns(
        conn,
        "rebuild_state",
        &[
            ("id", "integer"),
            ("status", "text"),
            ("invalid_count", "integer"),
            ("manifest_digest", "text"),
            ("rebuilt_at", "text"),
        ],
    )
    .map_err(|err| IndexError::Message(format!("validate rebuild_state schema: {err}")))?;
    Ok(())
}

fn require_index_state_columns(
    conn: &Connection,
    table: &str,
    expected: &[(&str, &str)],
) -> Result<(), IndexError> {
    let mut statement = conn
        .prepare(&format!("pragma table_info({table})"))
        .map_err(IndexError::Rusqlite)?;
    let mut found: HashSet<String> = HashSet::with_capacity(expected.len());
    let mut rows = statement.query([])?;
    while let Some(row) = rows.next()? {
        let name: String = row.get(1)?;
        let column_type: String = row.get(2)?;
        let not_null: i64 = row.get(3)?;
        let Some((_, want_type)) = expected.iter().find(|(expected_name, _)| *expected_name == name) else {
            continue;
        };
        if !column_type.trim().eq_ignore_ascii_case(want_type) {
            return Err(IndexError::Message(format!(
                "column {name:?} has type {column_type:?}; want {want_type:?}"
            )));
        }
        if not_null != 1 {
            return Err(IndexError::Message(format!(
                "column {name:?} must be not null"
            )));
        }
        found.insert(name);
    }
    for (name, _) in expected {
        if !found.contains(*name) {
            return Err(IndexError::Message(format!("missing column {name:?}")));
        }
    }
    Ok(())
}

fn read_trust_input_manifest(conn: &Connection) -> Result<TrustInputManifest, IndexError> {
    let mut statement = conn
        .prepare("select path, kind, byte_length, sha256 from trust_inputs order by path")
        .map_err(|err| IndexError::Message(format!("read trust inputs: {err}")))?;
    let mut rows = statement.query([])?;
    let mut entries: Vec<TrustInput> = Vec::new();
    let mut previous_path = String::new();
    while let Some(row) = rows.next()? {
        let entry = TrustInput {
            path: row.get(0)?,
            kind: row.get(1)?,
            byte_length: row.get(2)?,
            sha256: row.get(3)?,
        };
        if !previous_path.is_empty() && entry.path <= previous_path {
            return Err(IndexError::Message(format!(
                "trust inputs are not unique and sorted at {:?}",
                entry.path
            )));
        }
        validate_trust_input(&entry).map_err(|err| {
            IndexError::Message(format!("validate trust input {:?}: {err}", entry.path))
        })?;
        previous_path = entry.path.clone();
        entries.push(entry);
    }
    let digest = trust_input_manifest_digest(&entries);
    let manifest = TrustInputManifest { entries, digest };
    validate_trust_input_manifest(&manifest)
        .map_err(|err| IndexError::Message(format!("validate trust input manifest: {err}")))?;
    Ok(manifest)
}

fn read_rebuild_state(conn: &Connection) -> Result<RebuildState, IndexError> {
    let mut statement = conn
        .prepare("select id, status, invalid_count, manifest_digest, rebuilt_at from rebuild_state")
        .map_err(|err| IndexError::Message(format!("read rebuild state: {err}")))?;
    let mut rows = statement.query([])?;
    let mut state: Option<RebuildState> = None;
    while let Some(row) = rows.next()? {
        if state.is_some() {
            return Err(IndexError::Message(
                "rebuild_state must contain exactly one row; found more than one".into(),
            ));
        }
        let id: i64 = row.get(0)?;
        let status: String = row.get(1)?;
        state = Some(RebuildState {
            status,
            invalid_count: row.get(2)?,
            manifest_digest: row.get(3)?,
            rebuilt_at: row.get(4)?,
        });
        if id != INDEX_STATE_SINGLETON {
            return Err(IndexError::Message(format!(
                "rebuild_state singleton id = {id}; want {INDEX_STATE_SINGLETON}"
            )));
        }
    }
    let Some(state) = state else {
        return Err(IndexError::Message(
            "rebuild_state must contain exactly one row; found 0".into(),
        ));
    };
    Ok(state)
}

pub(crate) fn validate_trust_input_manifest(
    manifest: &TrustInputManifest,
) -> Result<(), IndexError> {
    let mut previous_path = String::new();
    for (index, entry) in manifest.entries.iter().enumerate() {
        validate_trust_input(entry)
            .map_err(|err| IndexError::Message(format!("entry {index}: {err}")))?;
        if !previous_path.is_empty() && entry.path <= previous_path {
            return Err(IndexError::Message(
                "entries must be strictly sorted by unique path".into(),
            ));
        }
        previous_path = entry.path.clone();
    }
    let expected_digest = trust_input_manifest_digest(&manifest.entries);
    if !is_sha256_digest(&manifest.digest) {
        return Err(IndexError::Message(
            "manifest digest must be a lowercase SHA-256 digest".into(),
        ));
    }
    if manifest.digest != expected_digest {
        return Err(IndexError::Message(
            "manifest digest does not match entries".into(),
        ));
    }
    Ok(())
}

pub(crate) fn validate_trust_input(entry: &TrustInput) -> Result<(), IndexError> {
    if entry.path.is_empty() || entry.path.trim() != entry.path {
        return Err(IndexError::Message(
            "path must be non-empty and trimmed".into(),
        ));
    }
    if entry.path.contains('\\') || entry.path.contains('\0') || entry.path.starts_with('/') {
        return Err(IndexError::Message(
            "path must be a slash-normalized relative path".into(),
        ));
    }
    if !is_canonical_slash_path(&entry.path) || entry.path == "." || entry.path.starts_with("../")
    {
        return Err(IndexError::Message(
            "path must be canonical and remain relative".into(),
        ));
    }
    if entry.byte_length < 0 {
        return Err(IndexError::Message(
            "byte length must not be negative".into(),
        ));
    }
    if !is_sha256_digest(&entry.sha256) {
        return Err(IndexError::Message(
            "sha256 must be a lowercase SHA-256 digest".into(),
        ));
    }
    let parts: Vec<&str> = entry.path.split('/').collect();
    match entry.kind.as_str() {
        crate::manifest::TRUST_INPUT_KIND_CLAIM => {
            if parts.len() < 3
                || parts[0] != "wiki"
                || !crate::claims::is_known_wiki_tier(parts[1])
                || !entry.path.ends_with(".md")
            {
                return Err(IndexError::Message(
                    "claim path must be wiki/<tier>/**/*.md".into(),
                ));
            }
        }
        crate::manifest::TRUST_INPUT_KIND_EVIDENCE_METADATA
        | crate::manifest::TRUST_INPUT_KIND_EVIDENCE_RAW => {
            if parts.len() != 4 || parts[0] != "evidence" || parts[1] != "sources" || parts[2].is_empty()
            {
                return Err(IndexError::Message(
                    "evidence path must be evidence/sources/<id>/<file>".into(),
                ));
            }
            let want_name = if entry.kind == crate::manifest::TRUST_INPUT_KIND_EVIDENCE_RAW {
                "raw"
            } else {
                "source.yaml"
            };
            if parts[3] != want_name {
                return Err(IndexError::Message(format!(
                    "evidence {} path must end in {want_name:?}",
                    entry.kind
                )));
            }
        }
        other => {
            return Err(IndexError::Message(format!(
                "kind {other:?} is not supported"
            )));
        }
    }
    Ok(())
}

// Go pathpkg.Clean(p) == p for slash paths means: no empty components, no
// "." or ".." components, not rooted.
fn is_canonical_slash_path(path: &str) -> bool {
    !path.is_empty() && path.split('/').all(|part| !part.is_empty() && part != "." && part != "..")
}

pub(crate) fn validate_rebuild_state(
    state: &RebuildState,
    manifest_digest: &str,
) -> Result<(), IndexError> {
    match state.status.as_str() {
        REBUILD_STATUS_CLEAN => {
            if state.invalid_count != 0 {
                return Err(IndexError::Message(
                    "clean state must have zero invalid claims".into(),
                ));
            }
        }
        REBUILD_STATUS_REJECTED => {
            if state.invalid_count <= 0 {
                return Err(IndexError::Message(
                    "rejected state must have at least one invalid claim".into(),
                ));
            }
        }
        other => {
            return Err(IndexError::Message(format!(
                "status {other:?} is not supported"
            )));
        }
    }
    if state.manifest_digest != manifest_digest || !is_sha256_digest(&state.manifest_digest) {
        return Err(IndexError::Message(
            "manifest digest is invalid or does not match trust inputs".into(),
        ));
    }
    if state.rebuilt_at.is_empty() {
        return Err(IndexError::Message("rebuilt_at is required".into()));
    }
    chrono::DateTime::parse_from_rfc3339(&state.rebuilt_at)
        .map_err(|err| IndexError::Message(format!("rebuilt_at must be RFC3339: {err}")))?;
    Ok(())
}

pub(crate) fn is_sha256_digest(value: &str) -> bool {
    value.len() == 64
        && value.bytes().all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::index::{create_index_schema, test_support::fixed_index_rfc3339};
    use crate::manifest::{TRUST_INPUT_KIND_CLAIM, TRUST_INPUT_KIND_EVIDENCE_METADATA, TRUST_INPUT_KIND_EVIDENCE_RAW};

    fn new_index_state_test_db() -> Connection {
        let conn = Connection::open_in_memory().unwrap();
        create_index_schema(&conn).unwrap();
        conn
    }

    fn index_state_manifest(entries: &[TrustInput]) -> TrustInputManifest {
        let mut sorted = entries.to_vec();
        sorted.sort_by(|left, right| left.path.cmp(&right.path));
        let digest = trust_input_manifest_digest(&sorted);
        TrustInputManifest {
            entries: sorted,
            digest,
        }
    }

    fn clean_index_state(manifest: &TrustInputManifest) -> RebuildState {
        RebuildState {
            status: REBUILD_STATUS_CLEAN.into(),
            invalid_count: 0,
            manifest_digest: manifest.digest.clone(),
            rebuilt_at: "2026-08-04T05:00:00Z".into(),
        }
    }

    fn write_index_state_test_state(
        conn: &Connection,
        manifest: &TrustInputManifest,
        state: &RebuildState,
    ) {
        let tx = conn.unchecked_transaction().unwrap();
        write_index_state(&tx, manifest, state).unwrap();
        tx.commit().unwrap();
    }

    #[test]
    fn index_rebuild_state() {
        let conn = new_index_state_test_db();
        let entries = vec![TrustInput {
            path: "wiki/projects/clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.md".into(),
            kind: TRUST_INPUT_KIND_CLAIM.into(),
            byte_length: 12,
            sha256: "a".repeat(64),
        }];
        let manifest = index_state_manifest(&entries);
        let state = RebuildState {
            status: REBUILD_STATUS_CLEAN.into(),
            invalid_count: 0,
            manifest_digest: manifest.digest.clone(),
            rebuilt_at: "2026-08-04T05:00:00Z".into(),
        };
        let tx = conn.unchecked_transaction().unwrap();
        write_index_state(&tx, &manifest, &state).unwrap();
        tx.commit().unwrap();

        let (got_manifest, got_state) = read_index_state(&conn).unwrap();
        assert_eq!(got_manifest, manifest);
        assert_eq!(got_state, state);
    }

    #[test]
    fn index_state_transaction_rollback() {
        let conn = new_index_state_test_db();
        let manifest = index_state_manifest(&[]);
        let tx = conn.unchecked_transaction().unwrap();
        write_index_state(&tx, &manifest, &clean_index_state(&manifest)).unwrap();
        read_index_state_tx(&tx).unwrap();
        tx.rollback().unwrap();
        assert!(read_index_state(&conn).is_err(), "want missing state after rollback");
    }

    #[test]
    fn index_trust_inputs() {
        let conn = new_index_state_test_db();
        let entries = [
            TrustInput {
                path: "evidence/sources/ev_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/raw".into(),
                kind: TRUST_INPUT_KIND_EVIDENCE_RAW.into(),
                byte_length: 20,
                sha256: "b".repeat(64),
            },
            TrustInput {
                path: "evidence/sources/ev_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/source.yaml".into(),
                kind: TRUST_INPUT_KIND_EVIDENCE_METADATA.into(),
                byte_length: 21,
                sha256: "c".repeat(64),
            },
            TrustInput {
                path: "wiki/axioms/clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.md".into(),
                kind: TRUST_INPUT_KIND_CLAIM.into(),
                byte_length: 22,
                sha256: "d".repeat(64),
            },
        ];
        let entries = vec![entries[2].clone(), entries[1].clone(), entries[0].clone()];
        let manifest = index_state_manifest(&entries);
        let state = clean_index_state(&manifest);
        let tx = conn.unchecked_transaction().unwrap();
        write_index_state(&tx, &manifest, &state).unwrap();
        tx.commit().unwrap();

        let count: i64 = conn
            .query_row("select count(*) from trust_inputs", [], |row| row.get(0))
            .unwrap();
        assert_eq!(count, entries.len() as i64);
        let (got_manifest, _) = read_index_state(&conn).unwrap();
        assert_eq!(got_manifest.digest, manifest.digest);
        assert_eq!(got_manifest.entries, manifest.entries);
    }

    #[test]
    fn index_state_missing() {
        let conn = new_index_state_test_db();
        conn.execute("delete from rebuild_state", []).unwrap();
        assert!(read_index_state(&conn).is_err());

        let old_db = Connection::open_in_memory().unwrap();
        old_db.execute("create table claims(id text)", []).unwrap();
        assert!(read_index_state(&old_db).is_err(), "want fail-closed error");
    }

    #[test]
    fn index_state_rejected() {
        let conn = new_index_state_test_db();
        let manifest = index_state_manifest(&[]);
        let state = RebuildState {
            status: REBUILD_STATUS_REJECTED.into(),
            invalid_count: 2,
            manifest_digest: manifest.digest.clone(),
            rebuilt_at: "2026-08-04T05:00:00Z".into(),
        };
        write_index_state_test_state(&conn, &manifest, &state);
        let (_, got_state) = read_index_state(&conn).unwrap();
        assert_eq!(got_state, state);

        conn.execute("update rebuild_state set status = 'unknown'", []).unwrap();
        assert!(read_index_state(&conn).is_err());
    }

    #[test]
    fn index_state_rejects_malformed_rows() {
        let updates = [
            ("negative-byte-length", "update trust_inputs set byte_length = -1"),
            ("bad-digest", "update trust_inputs set sha256 = 'not-a-digest'"),
            ("unknown-kind", "update trust_inputs set kind = 'unknown'"),
        ];
        for (name, update) in updates {
            let conn = new_index_state_test_db();
            let manifest = index_state_manifest(&[TrustInput {
                path: "wiki/projects/clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.md".into(),
                kind: TRUST_INPUT_KIND_CLAIM.into(),
                byte_length: 12,
                sha256: "a".repeat(64),
            }]);
            write_index_state_test_state(&conn, &manifest, &clean_index_state(&manifest));
            conn.execute(update, []).unwrap();
            assert!(read_index_state(&conn).is_err(), "{name}: want fail-closed error");
        }

        let conn = new_index_state_test_db();
        let manifest = index_state_manifest(&[]);
        write_index_state_test_state(&conn, &manifest, &clean_index_state(&manifest));
        conn.execute_batch(
            "drop table rebuild_state; create table rebuild_state (id integer not null, status text not null, invalid_count integer not null, manifest_digest text not null, rebuilt_at text not null)",
        )
        .unwrap();
        for i in 1..=2 {
            conn.execute(
                "insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (?, 'clean', 0, ?, '2026-08-04T05:00:00Z')",
                rusqlite::params![i, manifest.digest],
            )
            .unwrap();
        }
        assert!(read_index_state(&conn).is_err());
    }

    #[test]
    fn index_state_write_validation() {
        let conn = new_index_state_test_db();
        let valid_manifest = index_state_manifest(&[]);
        let valid_state = clean_index_state(&valid_manifest);
        let invalid_manifests = vec![
            TrustInputManifest {
                entries: vec![TrustInput {
                    path: "wiki/projects/note.md".into(),
                    kind: TRUST_INPUT_KIND_CLAIM.into(),
                    byte_length: 1,
                    sha256: "a".repeat(64),
                }],
                digest: "a".repeat(64),
            },
            TrustInputManifest {
                entries: vec![TrustInput {
                    path: "wiki/projects/note.md".into(),
                    kind: "unknown".into(),
                    byte_length: 1,
                    sha256: "a".repeat(64),
                }],
                digest: "a".repeat(64),
            },
        ];
        for manifest in &invalid_manifests {
            let tx = conn.unchecked_transaction().unwrap();
            assert!(write_index_state(&tx, manifest, &valid_state).is_err());
            tx.rollback().unwrap();
        }

        let mut bad_state = valid_state.clone();
        bad_state.status = "unknown".into();
        let tx = conn.unchecked_transaction().unwrap();
        assert!(write_index_state(&tx, &valid_manifest, &bad_state).is_err());
        tx.rollback().unwrap();
    }

    #[test]
    fn index_state_validation_functions() {
        // validateTrustInput cases (nil-tx nil-db cases are unrepresentable in
        // Rust's type system and skipped).
        let blank = TrustInput {
            path: "  ".into(),
            kind: TRUST_INPUT_KIND_CLAIM.into(),
            byte_length: 1,
            sha256: "a".repeat(64),
        };
        assert!(validate_trust_input(&blank).is_err());
        let negative = TrustInput {
            path: "wiki/projects/a.md".into(),
            kind: TRUST_INPUT_KIND_CLAIM.into(),
            byte_length: -1,
            sha256: "a".repeat(64),
        };
        assert!(validate_trust_input(&negative).is_err());
        let unknown_kind = TrustInput {
            path: "wiki/projects/a.md".into(),
            kind: "unknown".into(),
            byte_length: 1,
            sha256: "a".repeat(64),
        };
        assert!(validate_trust_input(&unknown_kind).is_err());
        let valid_evidence_raw = TrustInput {
            path: "evidence/sources/ev_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/raw".into(),
            kind: TRUST_INPUT_KIND_EVIDENCE_RAW.into(),
            byte_length: 1,
            sha256: "b".repeat(64),
        };
        assert!(validate_trust_input(&valid_evidence_raw).is_ok());
        let bad_evidence_path = TrustInput {
            path: "evidence/sources/ev_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/wrong".into(),
            kind: TRUST_INPUT_KIND_EVIDENCE_RAW.into(),
            byte_length: 1,
            sha256: "b".repeat(64),
        };
        assert!(validate_trust_input(&bad_evidence_path).is_err());

        let valid_digest = "a".repeat(64);
        let rebuilt_at = "2026-08-04T05:00:00Z";
        assert!(validate_rebuild_state(&RebuildState { status: REBUILD_STATUS_CLEAN.into(), invalid_count: 0, manifest_digest: valid_digest.clone(), rebuilt_at: rebuilt_at.into() }, &valid_digest).is_ok());
        assert!(validate_rebuild_state(&RebuildState { status: REBUILD_STATUS_CLEAN.into(), invalid_count: 1, manifest_digest: valid_digest.clone(), rebuilt_at: rebuilt_at.into() }, &valid_digest).is_err());
        assert!(validate_rebuild_state(&RebuildState { status: REBUILD_STATUS_REJECTED.into(), invalid_count: 0, manifest_digest: valid_digest.clone(), rebuilt_at: rebuilt_at.into() }, &valid_digest).is_err());
        assert!(validate_rebuild_state(&RebuildState { status: REBUILD_STATUS_REJECTED.into(), invalid_count: 1, manifest_digest: "bad".into(), rebuilt_at: rebuilt_at.into() }, &valid_digest).is_err());
        assert!(validate_rebuild_state(&RebuildState { status: REBUILD_STATUS_CLEAN.into(), invalid_count: 0, manifest_digest: valid_digest.clone(), rebuilt_at: "not-rfc3339".into() }, &valid_digest).is_err());
        assert!(validate_rebuild_state(&RebuildState { status: "unknown".into(), invalid_count: 0, manifest_digest: valid_digest.clone(), rebuilt_at: rebuilt_at.into() }, &valid_digest).is_err());

        let empty_manifest = TrustInputManifest {
            entries: Vec::new(),
            digest: trust_input_manifest_digest(&[]),
        };
        assert!(validate_trust_input_manifest(&empty_manifest).is_ok());
        let bad_manifest = TrustInputManifest {
            entries: vec![
                TrustInput { path: "wiki/projects/b.md".into(), kind: TRUST_INPUT_KIND_CLAIM.into(), byte_length: 1, sha256: "b".repeat(64) },
                TrustInput { path: "wiki/projects/a.md".into(), kind: TRUST_INPUT_KIND_CLAIM.into(), byte_length: 1, sha256: "a".repeat(64) },
            ],
            digest: "bad".into(),
        };
        assert!(validate_trust_input_manifest(&bad_manifest).is_err());
        let _ = fixed_index_rfc3339();
    }

    #[test]
    fn index_state_schema_validation() {
        let conn = new_index_state_test_db();
        conn.execute_batch("pragma user_version = 99").unwrap();
        assert!(validate_index_state_schema(&conn).is_err());
        conn.execute_batch("pragma user_version = 3").unwrap();

        conn.execute_batch("drop table trust_inputs; create table trust_inputs (path text, kind text, byte_length integer, sha256 text)").unwrap();
        let expected = [("path", "text"), ("kind", "text"), ("byte_length", "integer"), ("sha256", "text")];
        assert!(require_index_state_columns(&conn, "trust_inputs", &expected).is_err());

        let conn2 = new_index_state_test_db();
        conn2.execute_batch("drop table trust_inputs; create table trust_inputs (path integer not null, kind text not null, byte_length integer not null, sha256 text not null)").unwrap();
        assert!(require_index_state_columns(&conn2, "trust_inputs", &expected).is_err());

        let conn3 = new_index_state_test_db();
        conn3.execute_batch("drop table trust_inputs; create table trust_inputs (path text not null, kind text not null)").unwrap();
        assert!(require_index_state_columns(&conn3, "trust_inputs", &expected).is_err());
    }

    #[test]
    fn index_state_read_errors() {
        let conn = new_index_state_test_db();
        let manifest = index_state_manifest(&[
            TrustInput { path: "wiki/projects/a.md".into(), kind: TRUST_INPUT_KIND_CLAIM.into(), byte_length: 1, sha256: "a".repeat(64) },
            TrustInput { path: "wiki/projects/b.md".into(), kind: TRUST_INPUT_KIND_CLAIM.into(), byte_length: 1, sha256: "b".repeat(64) },
        ]);
        let tx = conn.unchecked_transaction().unwrap();
        tx.execute(
            "insert into trust_inputs(path, kind, byte_length, sha256) values (?, ?, ?, ?)",
            rusqlite::params!["wiki/projects/a.md", TRUST_INPUT_KIND_CLAIM, 1, "a".repeat(64)],
        )
        .unwrap();
        tx.execute(
            "insert into trust_inputs(path, kind, byte_length, sha256) values (?, ?, ?, ?)",
            rusqlite::params!["wiki/projects/b.md", TRUST_INPUT_KIND_CLAIM, 1, "b".repeat(64)],
        )
        .unwrap();
        tx.execute(
            "insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (1, 'clean', 0, ?, '2026-08-04T05:00:00Z')",
            rusqlite::params![manifest.digest],
        )
        .unwrap();
        tx.commit().unwrap();
        assert!(read_index_state(&conn).is_ok());

        let conn_bad = new_index_state_test_db();
        let tx2 = conn_bad.unchecked_transaction().unwrap();
        tx2.execute(
            "insert into trust_inputs(path, kind, byte_length, sha256) values (?, ?, ?, ?)",
            rusqlite::params!["wiki/projects/a.md", TRUST_INPUT_KIND_CLAIM, 1, "bad-sha"],
        )
        .unwrap();
        tx2.execute(
            "insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (1, 'clean', 0, ?, '2026-08-04T05:00:00Z')",
            rusqlite::params![manifest.digest],
        )
        .unwrap();
        tx2.commit().unwrap();
        assert!(read_index_state(&conn_bad).is_err());

        let conn2 = new_index_state_test_db();
        let manifest2 = index_state_manifest(&[]);
        conn2.execute_batch("drop table rebuild_state; create table rebuild_state (id integer not null, status text not null, invalid_count integer not null, manifest_digest text not null, rebuilt_at text not null)").unwrap();
        for i in 1..=2 {
            conn2.execute(
                "insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (?, 'clean', 0, ?, '2026-08-04T05:00:00Z')",
                rusqlite::params![i, manifest2.digest],
            )
            .unwrap();
        }
        assert!(read_index_state(&conn2).is_err());

        let conn3 = new_index_state_test_db();
        conn3.execute_batch("drop table rebuild_state; create table rebuild_state (id integer not null primary key, status text not null, invalid_count integer not null, manifest_digest text not null, rebuilt_at text not null)").unwrap();
        conn3.execute(
            "insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (2, 'clean', 0, ?, '2026-08-04T05:00:00Z')",
            rusqlite::params![manifest2.digest],
        )
        .unwrap();
        assert!(read_index_state(&conn3).is_err());
    }

    #[test]
    fn index_state_write_errors() {
        let conn = new_index_state_test_db();
        let unsorted = TrustInputManifest {
            entries: vec![
                TrustInput { path: "wiki/projects/b.md".into(), kind: TRUST_INPUT_KIND_CLAIM.into(), byte_length: 1, sha256: "b".repeat(64) },
                TrustInput { path: "wiki/projects/a.md".into(), kind: TRUST_INPUT_KIND_CLAIM.into(), byte_length: 1, sha256: "a".repeat(64) },
            ],
            digest: "a".repeat(64),
        };
        let tx = conn.unchecked_transaction().unwrap();
        assert!(write_index_state(&tx, &unsorted, &clean_index_state(&unsorted)).is_err());
        tx.rollback().unwrap();

        let tx2 = conn.unchecked_transaction().unwrap();
        let valid_manifest = index_state_manifest(&[]);
        let bad_state = RebuildState {
            status: REBUILD_STATUS_CLEAN.into(),
            invalid_count: 1,
            manifest_digest: valid_manifest.digest.clone(),
            rebuilt_at: "2026-08-04T05:00:00Z".into(),
        };
        assert!(write_index_state(&tx2, &valid_manifest, &bad_state).is_err());
        tx2.rollback().unwrap();

        let conn4 = new_index_state_test_db();
        conn4.execute_batch("pragma user_version = 99").unwrap();
        let tx3 = conn4.unchecked_transaction().unwrap();
        assert!(write_index_state(&tx3, &valid_manifest, &clean_index_state(&valid_manifest)).is_err());
        tx3.rollback().unwrap();
    }

    #[test]
    fn index_state_schema_other_tables() {
        let conn = new_index_state_test_db();
        conn.execute_batch("drop table rebuild_state; create table rebuild_state (id integer, status text, invalid_count integer, manifest_digest text, rebuilt_at text)").unwrap();
        assert!(validate_index_state_schema(&conn).is_err());

        let conn2 = new_index_state_test_db();
        conn2.execute_batch("drop table trust_input_mtimes; create table trust_input_mtimes (path integer not null, modified_at integer not null, change_token integer not null)").unwrap();
        assert!(validate_index_state_schema(&conn2).is_err());

        let conn3 = new_index_state_test_db();
        conn3.execute_batch("drop table trust_directories; create table trust_directories (path text not null)").unwrap();
        assert!(validate_index_state_schema(&conn3).is_err());

        let conn4 = new_index_state_test_db();
        let manifest = index_state_manifest(&[TrustInput { path: "wiki/projects/a.md".into(), kind: TRUST_INPUT_KIND_CLAIM.into(), byte_length: 1, sha256: "a".repeat(64) }]);
        write_index_state_test_state(&conn4, &manifest, &clean_index_state(&manifest));
        conn4.execute("update trust_inputs set sha256 = 'bad'", []).unwrap();
        assert!(read_index_state(&conn4).is_err());

        let conn5 = new_index_state_test_db();
        write_index_state_test_state(&conn5, &manifest, &clean_index_state(&manifest));
        conn5.execute("update rebuild_state set status = 'bad'", []).unwrap();
        assert!(read_index_state(&conn5).is_err());

        let dup_entries = vec![
            TrustInput { path: "wiki/projects/a.md".into(), kind: TRUST_INPUT_KIND_CLAIM.into(), byte_length: 1, sha256: "a".repeat(64) },
            TrustInput { path: "wiki/projects/a.md".into(), kind: TRUST_INPUT_KIND_CLAIM.into(), byte_length: 1, sha256: "a".repeat(64) },
        ];
        let dup_manifest = TrustInputManifest {
            entries: dup_entries.clone(),
            digest: trust_input_manifest_digest(&dup_entries),
        };
        assert!(validate_trust_input_manifest(&dup_manifest).is_err());

        assert!(!is_sha256_digest(&"A".repeat(64)));
        assert!(is_sha256_digest(&"a".repeat(64)));
    }
}
