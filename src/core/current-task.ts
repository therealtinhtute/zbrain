import { ensureDir, writeTextFile } from "./fs";
import { join } from "node:path";
import { type RuntimePaths } from "./runtime-paths";
import { type RankedRetrievalResult, type RetrievalTier } from "./retrieval-ranking";

const tierLabels: Record<RetrievalTier, string> = {
  P0: "Axioms",
  P1: "Mental Models",
  P2: "Projects",
  P3: "Decisions",
};

export interface CurrentTaskInput {
  query: string;
  workspace: string;
  results: RankedRetrievalResult[];
}

function groupByTier(results: RankedRetrievalResult[]): Map<RetrievalTier, RankedRetrievalResult[]> {
  const grouped = new Map<RetrievalTier, RankedRetrievalResult[]>();
  for (const tier of ["P0", "P1", "P2", "P3"] as RetrievalTier[]) {
    grouped.set(tier, []);
  }

  for (const result of results) {
    grouped.get(result.tier)?.push(result);
  }

  return grouped;
}

export function generateCurrentTaskMarkdown({ query, workspace, results }: CurrentTaskInput): string {
  const grouped = groupByTier(results);
  const lines: string[] = [
    `# Wiki Context - ${query}`,
    "",
    `Generated: ${new Date().toISOString()}`,
    `Workspace: ${workspace}`,
    "Retrieval: qmd BM25 search",
    "",
    "## Search Keywords",
    query,
    "",
    "## Retrieved Docs (by priority)",
  ];

  for (const tier of ["P0", "P1", "P2", "P3"] as RetrievalTier[]) {
    const entries = grouped.get(tier) ?? [];
    if (entries.length === 0) {
      continue;
    }

    lines.push(`### ${tier} - ${tierLabels[tier]}`);
    lines.push("| Score | File | Preview |");
    lines.push("| --- | --- | --- |");
    for (const entry of entries) {
      lines.push(`| ${entry.score} | ${entry.path} | ${entry.snippet.replace(/\n/g, " ")} |`);
    }
    lines.push("");
  }

  lines.push("## Full Context");
  lines.push("");
  for (const entry of results) {
    lines.push(`### ${entry.path} (${entry.tier})`);
    lines.push(entry.body?.trim() || entry.snippet.trim() || "_No body returned._");
    lines.push("");
  }

  const missingTiers = (["P0", "P1", "P2", "P3"] as RetrievalTier[]).filter(
    (tier) => (grouped.get(tier) ?? []).length === 0,
  );

  lines.push("## Knowledge Gaps");
  if (results.length === 0) {
    lines.push("- No documents matched the active workspace query.");
  } else if (missingTiers.length === 0) {
    lines.push("- none");
  } else {
    for (const tier of missingTiers) {
      lines.push(`- No ${tierLabels[tier]} results (${tier}) for this query.`);
    }
  }

  lines.push("");
  return lines.join("\n");
}

export function currentTaskFilePath(paths: RuntimePaths): string {
  return join(paths.cwd, ".claude", "context", "current-task.md");
}

export function writeCurrentTask(paths: RuntimePaths, markdown: string): string {
  const filePath = currentTaskFilePath(paths);
  ensureDir(join(paths.cwd, ".claude", "context"));
  writeTextFile(filePath, markdown, { overwrite: true });
  return filePath;
}
