// `zbrain lease` CLI — acquire / release / list advisory write leases.

import { Command } from "commander";
import { assertRuntimeReady, createCommandContext } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { initDb } from "../core/db";
import { acquireLease, releaseLease, getLease, listLeases } from "../core/concurrency";
import { getSessionId } from "../core/session";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface LeaseCommandOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
  ttlMs?: number;
  holder?: string;
}

export async function runLeaseAcquire(target: string, options: LeaseCommandOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const db = initDb(context.paths.runtimeDir);
  const { workspace, path } = parseTarget(target);
  const holder = options.holder ?? getSessionId();
  const lease = acquireLease(db, { workspace, path, holder, ttlMs: options.ttlMs });
  ui.note(
    [
      `workspace: ${lease.workspace}`,
      `path: ${lease.path}`,
      `holder: ${lease.holder}`,
      `expires_at: ${lease.expiresAt}`,
    ].join("\n"),
    "Lease acquired",
  );
}

export async function runLeaseRelease(target: string, options: LeaseCommandOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const db = initDb(context.paths.runtimeDir);
  const { workspace, path } = parseTarget(target);
  const holder = options.holder ?? getSessionId();
  const released = releaseLease(db, workspace, path, holder);
  ui.note(released ? "released" : "no matching lease (wrong holder or already expired)", "Lease release");
}

export async function runLeaseList(options: { workspace?: string } & LeaseCommandOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const db = initDb(context.paths.runtimeDir);
  const leases = listLeases(db, options.workspace);
  if (leases.length === 0) {
    ui.note("no active leases", "zbrain lease list");
    return;
  }
  const lines = leases.map((l) => `${l.workspace} :: ${l.path}\n  holder=${l.holder} expires=${l.expiresAt}`);
  ui.note(lines.join("\n"), "Active leases");
}

function parseTarget(target: string): { workspace: string; path: string } {
  const sep = target.indexOf("::");
  if (sep === -1) {
    throw new Error("Lease target must be `<workspace>::<path>` (e.g. `research::wiki/axioms/auth.md`)");
  }
  return { workspace: target.slice(0, sep), path: target.slice(sep + 2) };
}

export function registerLeaseCommand(program: Command): void {
  const lease = program
    .command("lease")
    .description("Manage advisory write leases (TTL'd)");
  lease
    .command("acquire <target>")
    .description("Acquire a lease on <workspace>::<path>")
    .option("--ttl <ms>", "lease TTL in ms (default 60000)", "60000")
    .option("--holder <id>", "lease holder id (default: session id)")
    .action((target: string, options: any) =>
      runLeaseAcquire(target, { ttlMs: Number(options.ttl), holder: options.holder }),
    );
  lease
    .command("release <target>")
    .description("Release a lease")
    .option("--holder <id>", "lease holder id (default: session id)")
    .action((target: string, options: any) =>
      runLeaseRelease(target, { holder: options.holder }),
    );
  lease
    .command("list")
    .description("List active leases")
    .option("--workspace <name>", "filter to a workspace")
    .action((options: any) => runLeaseList({ workspace: options.workspace }));
}
