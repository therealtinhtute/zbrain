import { readTextFile, writeTextFile } from "./fs";
import { type RuntimePaths } from "./runtime-paths";
import { evidenceLocations, parseSourceRecord, updateEvidenceIndex, verifySourceRecordIntegrity } from "./evidence-store";
import { assertWorkspaceLock, assertValidEvidenceTransition } from "./evidence-state";

export function analyzeEvidence(paths: RuntimePaths, options: { workspace: string; evidenceId: string; nowIso?: string }): string[] {
  const nowIso = options.nowIso ?? new Date().toISOString();
  const locations = evidenceLocations(paths, options.workspace, options.evidenceId);
  const rawContent = readTextFile(locations.rawFile);
  const sourceRecord = parseSourceRecord(readTextFile(locations.sourceFile));

  verifySourceRecordIntegrity(sourceRecord, rawContent);
  assertWorkspaceLock(sourceRecord.workspace_at_ingest, options.workspace);
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

  updateEvidenceIndex(locations.indexFile, options.evidenceId, "analyzed", nowIso);
  return written;
}
