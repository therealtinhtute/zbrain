import type { Database } from "bun:sqlite";
import { listEvidence } from "./db-evidence";

export interface EvidenceListItem {
  id: string;
  state: string;
  updatedAt: string;
  label: string | null;
  sourceType: string | null;
  nextCommand: string | null;
}

const nextCommandByState: Record<string, (id: string) => string | null> = {
  ingested: (id) => `zbrain ingest analyze ${id}`,
  analyzed: (id) => `zbrain ingest qa ${id}`,
  qa_in_progress: (id) => `zbrain ingest qa ${id}`,
  qa_awaiting_external: (id) => `zbrain ingest qa ${id}`,
  qa_done: (id) => `zbrain ingest apply ${id}`,
  applied: () => null,
  archived: () => null,
};

export function listEvidenceItems(db: Database, workspace: string): EvidenceListItem[] {
  const rows = listEvidence(db, workspace);
  return rows.map((row) => ({
    id: row.id,
    state: row.state,
    updatedAt: row.state_updated_at,
    label: row.label,
    sourceType: row.source_type,
    nextCommand: nextCommandByState[row.state]?.(row.id) ?? null,
  }));
}
