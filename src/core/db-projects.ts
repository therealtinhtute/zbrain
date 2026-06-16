import type { Database } from "bun:sqlite";
import { projectBindingSchema, projectRegistrySchema } from "../schemas/config";
import type { ProjectBinding, ProjectRegistry } from "../schemas/config";

export function upsertProject(db: Database, binding: ProjectBinding, nowIso: string): void {
  db.prepare(`
    INSERT INTO projects (project_root, workspace, context_file, runtimes, secondary_workspaces, created_at, updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(project_root) DO UPDATE SET
      workspace            = excluded.workspace,
      context_file         = excluded.context_file,
      runtimes             = excluded.runtimes,
      secondary_workspaces = excluded.secondary_workspaces,
      updated_at           = excluded.updated_at
  `).run(
    binding.project_root,
    binding.workspace,
    binding.context_file,
    JSON.stringify(binding.runtimes ?? []),
    JSON.stringify(binding.secondary_workspaces ?? []),
    nowIso,
    nowIso,
  );
}

export function readProject(db: Database, projectRoot: string): ProjectBinding | null {
  const row = db.prepare("SELECT * FROM projects WHERE project_root = ?").get(projectRoot) as
    | Record<string, unknown>
    | null;
  return row ? parseProjectRow(row) : null;
}

export function listProjects(db: Database): ProjectBinding[] {
  const rows = db
    .prepare("SELECT * FROM projects ORDER BY project_root ASC")
    .all() as Record<string, unknown>[];
  return rows.map(parseProjectRow);
}

export function readProjectRegistry(db: Database): ProjectRegistry {
  return projectRegistrySchema.parse({ projects: listProjects(db) });
}

function parseProjectRow(row: Record<string, unknown>): ProjectBinding {
  return projectBindingSchema.parse({
    ...row,
    runtimes: JSON.parse(row.runtimes as string),
    secondary_workspaces: JSON.parse(row.secondary_workspaces as string),
  });
}
