import type { Database } from "bun:sqlite";
import { writeTextFile } from "./fs";
import { type RuntimePaths } from "./runtime-paths";
import {
  buildSourceRecord,
  createEvidenceId,
  evidenceLocations,
  ensureEvidenceDirectories,
} from "./evidence-store";
import { insertEvidence, listEvidenceIds } from "./db-evidence";

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
}

export function ingestEvidence(
  db: Database,
  paths: RuntimePaths,
  options: IngestEvidenceOptions,
): IngestEvidenceResult {
  const nowIso = options.nowIso ?? new Date().toISOString();
  const existingIds = listEvidenceIds(db, options.workspace);
  const evidenceId = createEvidenceId(nowIso, options.sourceType, options.label, existingIds);
  const locations = evidenceLocations(paths, options.workspace, evidenceId);

  ensureEvidenceDirectories(locations);
  writeTextFile(locations.rawFile, options.rawContent, { overwrite: true });

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

  insertEvidence(db, sourceRecord, nowIso);

  return {
    evidenceId,
    rawFile: locations.rawFile,
  };
}
