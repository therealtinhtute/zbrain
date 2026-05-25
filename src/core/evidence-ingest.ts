import { writeTextFile } from "./fs";
import { type RuntimePaths } from "./runtime-paths";
import {
  buildSourceRecord,
  createEvidenceId,
  evidenceLocations,
  ensureEvidenceDirectories,
  initializeEvidenceIndex,
  listEvidenceIds,
  serializeSourceRecord,
  updateEvidenceIndex,
} from "./evidence-store";

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
  sourceFile: string;
}

export function ingestEvidence(paths: RuntimePaths, options: IngestEvidenceOptions): IngestEvidenceResult {
  const nowIso = options.nowIso ?? new Date().toISOString();
  const evidenceId = createEvidenceId(nowIso, options.sourceType, options.label, listEvidenceIds(paths, options.workspace));
  const locations = evidenceLocations(paths, options.workspace, evidenceId);

  ensureEvidenceDirectories(locations);
  initializeEvidenceIndex(locations.indexFile);
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

  writeTextFile(locations.sourceFile, serializeSourceRecord(sourceRecord), { overwrite: true });
  updateEvidenceIndex(locations.indexFile, evidenceId, "ingested", nowIso);

  return {
    evidenceId,
    rawFile: locations.rawFile,
    sourceFile: locations.sourceFile,
  };
}
