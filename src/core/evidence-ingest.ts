import type { Database } from "bun:sqlite";
import { writeTextFile } from "./fs";
import { type RuntimePaths } from "./runtime-paths";
import {
  buildSourceRecord,
  createEvidenceId,
  evidenceLocations,
  ensureEvidenceDirectories,
  sha256,
} from "./evidence-store";
import { findEvidenceIdByRawSha, insertEvidence, listEvidenceIds } from "./db-evidence";

export interface IngestEvidenceOptions {
  workspace: string;
  sourceType: string;
  origin: string;
  label: string;
  rawContent: string;
  nowIso?: string;
}

export interface IngestEvidenceResult {
  evidenceId: string;
  rawFile: string;
  duplicate?: boolean;
}

export function ingestEvidence(
  db: Database,
  paths: RuntimePaths,
  options: IngestEvidenceOptions,
): IngestEvidenceResult {
  const nowIso = options.nowIso ?? new Date().toISOString();

  // Workspace-scoped content dedup: identical raw content already ingested in this
  // workspace returns the existing id without creating a new row or file. The same
  // content in a different workspace is not a duplicate (isolation, I-2).
  const rawSha = sha256(options.rawContent);
  const duplicateId = findEvidenceIdByRawSha(db, options.workspace, rawSha);
  if (duplicateId) {
    return {
      evidenceId: duplicateId,
      rawFile: evidenceLocations(paths, options.workspace, duplicateId).rawFile,
      duplicate: true,
    };
  }

  const existingIds = listEvidenceIds(db, options.workspace);
  const evidenceId = createEvidenceId(nowIso, options.sourceType, options.label, existingIds);
  const locations = evidenceLocations(paths, options.workspace, evidenceId);

  const sourceRecord = buildSourceRecord({
    id: evidenceId,
    type: options.sourceType,
    origin: options.origin,
    label: options.label,
    workspace_at_ingest: options.workspace,
    ingested_at: nowIso,
    state: "ingested",
    raw_filename: "raw.md",
    rawContent: options.rawContent,
  });

  // Insert the row first (in a transaction) so a failed insert never leaves an
  // orphaned raw.md on disk. Full FS+SQLite atomicity is deferred to ISSUE-007.
  db.transaction(() => {
    insertEvidence(db, sourceRecord, nowIso);
  })();

  ensureEvidenceDirectories(locations);
  writeTextFile(locations.rawFile, options.rawContent, { overwrite: true });

  return {
    evidenceId,
    rawFile: locations.rawFile,
  };
}
