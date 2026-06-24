import { createHash } from "node:crypto";
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
  secondaryWorkspaces?: string[];
  results: RankedRetrievalResult[];
  nowIso?: string;
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

export function generateCurrentTaskMarkdown({ query, workspace, secondaryWorkspaces, results, nowIso }: CurrentTaskInput): string {
  const grouped = groupByTier(results);
  const hasMultiWorkspace = results.some((r) => r.workspace !== undefined);
  const cell = (value: string) => value.replace(/\n/g, " ").replace(/\|/g, "\\|");

  const lines: string[] = [
    `# Wiki Context - ${query}`,
    "",
    `Generated: ${nowIso ?? new Date().toISOString()}`,
    `Workspace: ${workspace}`,
  ];

  if (secondaryWorkspaces && secondaryWorkspaces.length > 0) {
    lines.push(`Secondary Workspaces: ${secondaryWorkspaces.join(", ")}`);
  }

  lines.push("Retrieval: qmd BM25 search", "", "## Search Keywords", query, "", "## Retrieved Docs (by priority)");

  for (const tier of ["P0", "P1", "P2", "P3"] as RetrievalTier[]) {
    const entries = grouped.get(tier) ?? [];
    if (entries.length === 0) {
      continue;
    }

    lines.push(`### ${tier} - ${tierLabels[tier]}`);
    if (hasMultiWorkspace) {
      lines.push("| Score | Workspace | File | Preview |");
      lines.push("| --- | --- | --- | --- |");
      for (const entry of entries) {
        const ws = entry.workspace ?? workspace;
        lines.push(`| ${entry.score} | ${ws} | ${cell(entry.path)} | ${cell(entry.snippet)} |`);
      }
    } else {
      lines.push("| Score | File | Preview |");
      lines.push("| --- | --- | --- |");
      for (const entry of entries) {
        lines.push(`| ${entry.score} | ${cell(entry.path)} | ${cell(entry.snippet)} |`);
      }
    }
    lines.push("");
  }

  lines.push("## Full Context");
  lines.push("");
  for (const entry of results) {
    const sectionLabel = hasMultiWorkspace
      ? `### [${entry.workspace ?? workspace}] ${entry.path} (${entry.tier})`
      : `### ${entry.path} (${entry.tier})`;
    lines.push(sectionLabel);
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

// V2: runtime writes per-session context files via `writeSessionContext`
// (see session.ts). This path is retained only for the legacy `context_file`
// field written into project bindings by `initProject` (AC-P1-9 partial;
// not yet wired to per-session directory).
export function currentTaskFilePath(paths: RuntimePaths): string {
  return join(paths.projectsDir, projectRuntimeKey(paths.cwd), "current-task.md");
}

export function projectRuntimeKey(projectRoot: string): string {
  return createHash("sha256").update(projectRoot).digest("hex").slice(0, 16);
}
