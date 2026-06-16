import type { Database } from "bun:sqlite";
import { readTextFile, writeTextFile } from "./fs";
import { type RuntimePaths } from "./runtime-paths";
import { evidenceLocations } from "./evidence-store";
import { readEvidence, verifyEvidenceIntegrity, updateEvidenceState } from "./db-evidence";
import { assertWorkspaceLock, assertValidEvidenceTransition } from "./evidence-state";

export function analyzeEvidence(
  db: Database,
  paths: RuntimePaths,
  options: { workspace: string; evidenceId: string; nowIso?: string },
): string[] {
  const nowIso = options.nowIso ?? new Date().toISOString();
  const locations = evidenceLocations(paths, options.workspace, options.evidenceId);
  const rawContent = readTextFile(locations.rawFile);
  const row = readEvidence(db, options.workspace, options.evidenceId);

  if (!row) {
    throw new Error(`Evidence not found: ${options.evidenceId} in workspace ${options.workspace}`);
  }

  verifyEvidenceIntegrity(db, options.workspace, options.evidenceId, rawContent);
  assertWorkspaceLock(row.workspace_at_ingest, options.workspace);
  assertValidEvidenceTransition("ingested", "analyzed");

  const preview = rawContent.split("\n").slice(0, 5).join("\n");
  const files = [
    ["01-summary.md", `# Summary\n\nEvidence: ${options.evidenceId}\n\n${preview}\n`],
    ["02-contradictions.md", `# Contradictions\n\n- None identified automatically for ${options.evidenceId}.\n`],
    ["04-questions.md", `# Questions\n\n- P0: What fact must be verified from this source?\n- P1: What framework needs confirmation?\n- P2: What project-level detail needs validation?\n- P3: What decision context remains open?\n`],
    ["08-gaps.md", `# Gaps\n\n- Reviewer input still required before apply.\n`],
  ] as const;

  const written: string[] = [];
  for (const [fileName, contents] of files) {
    const filePath = `${locations.analysisDir}/${fileName}`;
    writeTextFile(filePath, contents, { overwrite: true });
    written.push(filePath);
  }

  updateEvidenceState(db, options.workspace, options.evidenceId, "analyzed", nowIso);
  return written;
}
