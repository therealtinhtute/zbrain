import { existsSync } from "node:fs";
import { evidenceLocations, parseSourceRecord } from "./evidence-store";
import { readTextFile } from "./fs";
import { type RuntimePaths } from "./runtime-paths";

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

export function listEvidenceItems(paths: RuntimePaths, workspace: string): EvidenceListItem[] {
  const locations = evidenceLocations(paths, workspace, "seed");
  if (!existsSync(locations.indexFile)) {
    return [];
  }

  const rows = readTextFile(locations.indexFile)
    .split("\n")
    .map((line) => line.match(/^\| ([^|]+) \| ([^|]+) \| ([^|]+) \|$/))
    .filter((match): match is RegExpMatchArray => match !== null)
    .map((match) => ({
      id: match[1]!.trim(),
      state: match[2]!.trim(),
      updatedAt: match[3]!.trim(),
    }))
    .filter((row) => row.id !== "id" && row.id !== "---" && row.id !== "seed");

  return rows.flatMap((row) => {
    const itemLocations = evidenceLocations(paths, workspace, row.id);
    if (!existsSync(itemLocations.sourceFile)) {
      return [];
    }

    const sourceRecord = parseSourceRecord(readTextFile(itemLocations.sourceFile));
    return [{
      ...row,
      label: sourceRecord.label ?? null,
      sourceType: sourceRecord.type ?? null,
      nextCommand: nextCommandByState[row.state]?.(row.id) ?? null,
    }];
  });
}
