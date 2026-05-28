import { existsSync } from "node:fs";
import { join } from "node:path";

export interface SecondaryResolutionResult {
  resolved: string[];
  warnings: string[];
}

export function resolveSecondaryWorkspaces(workspacesDir: string, names: string[]): SecondaryResolutionResult {
  const resolved: string[] = [];
  const warnings: string[] = [];

  for (const name of names) {
    const workspacePath = join(workspacesDir, name);
    if (existsSync(workspacePath)) {
      resolved.push(name);
    } else {
      warnings.push(`Secondary workspace "${name}" not found at ${workspacePath} — skipping`);
    }
  }

  return { resolved, warnings };
}
