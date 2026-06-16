import { existsSync } from "node:fs";
import type { Database } from "bun:sqlite";
import YAML from "js-yaml";
import { validateQAGate, assertWorkspaceLock, assertCitationCoverage, type EvidenceQuestion } from "./evidence-state";
import { readTextFile, writeTextFile } from "./fs";
import { type RuntimePaths } from "./runtime-paths";
import {
  assertWorkspaceTarget,
  evidenceLocations,
} from "./evidence-store";
import { readEvidence, verifyEvidenceIntegrity, updateEvidenceState } from "./db-evidence";

export interface ApplyMutation {
  relativePath: string;
  content: string;
  citations: Array<{ questionId: string; wikiPath: string }>;
}

export interface ApplyEvidenceOptions {
  workspace: string;
  evidenceId: string;
  questions: EvidenceQuestion[];
  mutations: ApplyMutation[];
  nowIso?: string;
  reindex?: (workspace: string) => void;
  failAfterMutations?: number;
}

interface CheckpointState {
  evidence_id: string;
  status: string;
  completed_paths: string[];
  last_updated: string;
}

function writeCheckpoint(filePath: string, checkpoint: CheckpointState): void {
  writeTextFile(filePath, `${JSON.stringify(checkpoint, null, 2)}\n`, { overwrite: true });
}

function readCheckpoint(filePath: string, evidenceId: string, nowIso: string): CheckpointState {
  if (!existsSync(filePath)) {
    return {
      evidence_id: evidenceId,
      status: "not_started",
      completed_paths: [],
      last_updated: nowIso,
    };
  }

  return JSON.parse(readTextFile(filePath)) as CheckpointState;
}

function injectResourceIfMissing(content: string, resource: string): string {
  const match = content.match(/^---\n([\s\S]*?)\n---\n/);
  if (!match) return content;
  const fm = YAML.load(match[1]) as Record<string, unknown>;
  if (fm.resource && typeof fm.resource === "string" && fm.resource.trim()) return content;
  fm.resource = resource;
  const newFm = YAML.dump(fm, { noRefs: true }).trimEnd();
  return content.replace(match[0], `---\n${newFm}\n---\n`);
}

export function applyEvidence(
  db: Database,
  paths: RuntimePaths,
  options: ApplyEvidenceOptions,
): { applied: string[] } {
  const nowIso = options.nowIso ?? new Date().toISOString();
  const locations = evidenceLocations(paths, options.workspace, options.evidenceId);
  const rawContent = readTextFile(locations.rawFile);
  const row = readEvidence(db, options.workspace, options.evidenceId);

  if (!row) {
    throw new Error(`Evidence not found: ${options.evidenceId} in workspace ${options.workspace}`);
  }

  verifyEvidenceIntegrity(db, options.workspace, options.evidenceId, rawContent);
  assertWorkspaceLock(row.workspace_at_ingest, options.workspace);
  validateQAGate(options.questions);
  assertCitationCoverage(options.mutations.flatMap((mutation) => mutation.citations));

  const checkpoint = readCheckpoint(locations.checkpointFile, options.evidenceId, nowIso);
  checkpoint.status = "in_progress";
  checkpoint.last_updated = nowIso;
  writeCheckpoint(locations.checkpointFile, checkpoint);

  const applied: string[] = [];
  for (const mutation of options.mutations) {
    if (checkpoint.completed_paths.includes(mutation.relativePath)) {
      continue;
    }

    const target = assertWorkspaceTarget(locations.workspaceRoot, mutation.relativePath);
    const content = target.endsWith(".md")
      ? injectResourceIfMissing(mutation.content, row.origin)
      : mutation.content;
    writeTextFile(target, content, { overwrite: true });
    checkpoint.completed_paths.push(mutation.relativePath);
    checkpoint.last_updated = nowIso;
    writeCheckpoint(locations.checkpointFile, checkpoint);
    applied.push(mutation.relativePath);

    if (
      typeof options.failAfterMutations === "number" &&
      applied.length >= options.failAfterMutations
    ) {
      throw new Error("Simulated apply interruption");
    }
  }

  checkpoint.status = "completed";
  checkpoint.last_updated = nowIso;
  writeCheckpoint(locations.checkpointFile, checkpoint);

  const manifest = {
    evidence_id: options.evidenceId,
    applied_at: nowIso,
    workspace: options.workspace,
    mutations: options.mutations.map((mutation) => mutation.relativePath),
  };
  writeTextFile(locations.manifestFile, YAML.dump(manifest, { noRefs: true }), { overwrite: true });
  updateEvidenceState(db, options.workspace, options.evidenceId, "applied", nowIso);
  options.reindex?.(options.workspace);

  return { applied: checkpoint.completed_paths };
}
