import type { Database } from "bun:sqlite";
import YAML from "js-yaml";
import { sha256, type EvidenceSourceRecord } from "./evidence-store";
import { assertValidEvidenceTransition, type EvidenceState } from "./evidence-state";

export interface DbEvidenceRow {
  id: string;
  workspace: string;
  source_type: string;
  origin: string;
  label: string;
  workspace_at_ingest: string;
  ingested_at: string;
  state: string;
  raw_filename: string;
  raw_sha256: string;
  source_sha256: string;
  state_updated_at: string;
}

export function insertEvidence(db: Database, record: EvidenceSourceRecord, nowIso: string): void {
  // `workspace` (part of the PK) and `workspace_at_ingest` are equal by construction:
  // both are set from the active workspace at ingest time. They are kept as separate
  // columns on purpose — the PK enforces per-workspace uniqueness, while
  // `workspace_at_ingest` feeds the immutable source integrity hash (source_sha256).
  // Dropping either would break a guarantee, so neither is removed.
  db.prepare(`
    INSERT INTO evidence_sources
      (id, workspace, source_type, origin, label, workspace_at_ingest, ingested_at, state, raw_filename, raw_sha256, source_sha256, state_updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `).run(
    record.id,
    record.workspace_at_ingest,
    record.type,
    record.origin,
    record.label,
    record.workspace_at_ingest,
    record.ingested_at,
    record.state,
    record.raw_filename,
    record.raw_sha256,
    record.source_sha256,
    nowIso,
  );
}

export function readEvidence(db: Database, workspace: string, id: string): DbEvidenceRow | null {
  return (
    (db
      .prepare("SELECT * FROM evidence_sources WHERE id = ? AND workspace = ?")
      .get(id, workspace) as DbEvidenceRow | null) ?? null
  );
}

export function findEvidenceIdByRawSha(db: Database, workspace: string, rawSha: string): string | null {
  const row = db
    .prepare("SELECT id FROM evidence_sources WHERE workspace = ? AND raw_sha256 = ? LIMIT 1")
    .get(workspace, rawSha) as { id: string } | null;
  return row?.id ?? null;
}

export function listEvidenceIds(db: Database, workspace: string): string[] {
  const rows = db
    .prepare("SELECT id FROM evidence_sources WHERE workspace = ? ORDER BY id ASC")
    .all(workspace) as Array<{ id: string }>;
  return rows.map((r) => r.id);
}

export function listEvidence(db: Database, workspace: string): DbEvidenceRow[] {
  return db
    .prepare("SELECT * FROM evidence_sources WHERE workspace = ? ORDER BY ingested_at ASC")
    .all(workspace) as DbEvidenceRow[];
}

export function updateEvidenceState(
  db: Database,
  workspace: string,
  id: string,
  nextState: EvidenceState,
  nowIso: string,
): void {
  const row = readEvidence(db, workspace, id);
  if (!row) {
    throw new Error(`Evidence not found: ${id} in workspace ${workspace}`);
  }
  assertValidEvidenceTransition(row.state as EvidenceState, nextState);
  db.prepare(
    "UPDATE evidence_sources SET state = ?, state_updated_at = ? WHERE id = ? AND workspace = ?",
  ).run(nextState, nowIso, id, workspace);
}

export function verifyEvidenceIntegrity(
  db: Database,
  workspace: string,
  id: string,
  rawContent: string,
): void {
  const row = readEvidence(db, workspace, id);
  if (!row) {
    throw new Error(`Evidence not found: ${id} in workspace ${workspace}`);
  }

  if (row.raw_sha256 !== sha256(rawContent)) {
    throw new Error("Immutable source violation: raw.md fingerprint mismatch");
  }

  // source_sha256 was computed at ingest time with state="ingested".
  // Reconstruct the exact original record shape to verify.
  const originalRecord = {
    id: row.id,
    type: row.source_type,
    origin: row.origin,
    label: row.label,
    workspace_at_ingest: row.workspace_at_ingest,
    ingested_at: row.ingested_at,
    state: "ingested",
    raw_filename: row.raw_filename,
    raw_sha256: row.raw_sha256,
  };
  const expectedSourceSha = sha256(YAML.dump(originalRecord, { noRefs: true }));
  if (row.source_sha256 !== expectedSourceSha) {
    throw new Error("Immutable source violation: source fingerprint mismatch");
  }
}
