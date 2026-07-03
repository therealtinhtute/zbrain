// Evidence service: thin wrapper used by MCP `remember` tool.
// Reuses existing evidence infrastructure (sha256, fingerprint, DB row).

import { buildSourceRecord, createEvidenceId, sha256, type EvidenceSourceRecord } from "./evidence-store";
import { insertEvidence } from "./db-evidence";
import type { Database } from "bun:sqlite";

export interface CreateEvidenceInput {
  workspace: string;
  sourceType: string;
  origin: string;
  label: string;
  rawContent: string;
  nowIso?: string;
}

export function createEvidenceRecord(input: CreateEvidenceInput): EvidenceSourceRecord {
  const nowIso = input.nowIso ?? new Date().toISOString();
  const id = createEvidenceId(nowIso, input.sourceType, input.label, []);
  return buildSourceRecord({
    id,
    type: input.sourceType,
    origin: input.origin,
    label: input.label,
    workspace_at_ingest: input.workspace,
    ingested_at: nowIso,
    state: "ingested",
    raw_filename: "raw.md",
    rawContent: input.rawContent,
  });
}

export { insertEvidence };
