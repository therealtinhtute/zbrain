import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { initDb } from "../src/core/db";
import { acquireLease, releaseLease, getLease, listLeases, expireLeases } from "../src/core/concurrency";
import { getSessionId, writeSessionContext, readSessionContext, sessionContextPath, listSessionIds } from "../src/core/session";
import { resolveRuntimePaths } from "../src/core/runtime-paths";

let tempHome: string;
let paths: ReturnType<typeof resolveRuntimePaths>;
let db: ReturnType<typeof initDb>;

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-conc-"));
  paths = resolveRuntimePaths({ runtimeDir: tempHome });
  db = initDb(tempHome);
});

afterEach(() => {
  db.close();
  rmSync(tempHome, { recursive: true, force: true });
});

test("acquireLease + getLease: roundtrip", () => {
  const lease = acquireLease(db, { workspace: "research", path: "wiki/axioms/auth.md", holder: "agent-1" });
  expect(lease.holder).toBe("agent-1");
  const fetched = getLease(db, "research", "wiki/axioms/auth.md");
  expect(fetched).not.toBeNull();
  expect(fetched?.holder).toBe("agent-1");
});

test("acquireLease: short TTL auto-expires on get", () => {
  acquireLease(db, { workspace: "research", path: "x.md", holder: "h", ttlMs: 50 });
  // Sleep 80ms to let TTL pass.
  const start = Date.now();
  while (Date.now() - start < 80) { /* spin */ }
  const fetched = getLease(db, "research", "x.md");
  expect(fetched).toBeNull();
});

test("acquireLease: new acquire overwrites previous holder (advisory, not enforced)", () => {
  acquireLease(db, { workspace: "research", path: "y.md", holder: "first" });
  acquireLease(db, { workspace: "research", path: "y.md", holder: "second" });
  const fetched = getLease(db, "research", "y.md");
  expect(fetched?.holder).toBe("second");
});

test("releaseLease: only matching holder can release", () => {
  acquireLease(db, { workspace: "research", path: "z.md", holder: "owner" });
  expect(releaseLease(db, "research", "z.md", "wrong")).toBe(false);
  expect(getLease(db, "research", "z.md")?.holder).toBe("owner");
  expect(releaseLease(db, "research", "z.md", "owner")).toBe(true);
  expect(getLease(db, "research", "z.md")).toBeNull();
});

test("listLeases: only returns non-expired leases", () => {
  acquireLease(db, { workspace: "research", path: "a.md", holder: "h1" });
  acquireLease(db, { workspace: "research", path: "b.md", holder: "h2", ttlMs: 50 });
  // Force-expire 'b' via direct SQL (fast).
  db.prepare(`UPDATE leases SET expires_at = '2000-01-01T00:00:00.000Z' WHERE path = 'b.md'`).run();
  const active = listLeases(db, "research");
  expect(active.length).toBe(1);
  expect(active[0]?.path).toBe("a.md");
});

test("expireLeases: removes expired rows in batch", () => {
  acquireLease(db, { workspace: "research", path: "e1.md", holder: "h" });
  acquireLease(db, { workspace: "research", path: "e2.md", holder: "h" });
  db.prepare(`UPDATE leases SET expires_at = '2000-01-01T00:00:00.000Z'`).run();
  const removed = expireLeases(db, "2099-01-01T00:00:00.000Z");
  expect(removed).toBe(2);
  expect(listLeases(db).length).toBe(0);
});

test("getSessionId: returns UUID when no override", () => {
  const id = getSessionId();
  expect(id).toMatch(/^[0-9a-f-]{36}$/);
});

test("getSessionId: respects override and env", () => {
  const a = getSessionId("custom-id");
  expect(a).toBe("custom-id");
  process.env.ZBRAIN_SESSION_ID = "env-id";
  expect(getSessionId()).toBe("env-id");
  delete process.env.ZBRAIN_SESSION_ID;
});

test("writeSessionContext: atomic write, readable by id", () => {
  const target = writeSessionContext(paths, "/tmp/proj", "session-1", "# context\nhello");
  expect(target).toBe(sessionContextPath(paths, "/tmp/proj", "session-1"));
  const read = readSessionContext(paths, "/tmp/proj", "session-1");
  expect(read).toBe("# context\nhello");
});

test("writeSessionContext: two parallel sessions don't clobber each other", () => {
  writeSessionContext(paths, "/tmp/proj", "agent-A", "A's context");
  writeSessionContext(paths, "/tmp/proj", "agent-B", "B's context");
  expect(readSessionContext(paths, "/tmp/proj", "agent-A")).toBe("A's context");
  expect(readSessionContext(paths, "/tmp/proj", "agent-B")).toBe("B's context");
});

test("listSessionIds: enumerates sessions for a project", () => {
  writeSessionContext(paths, "/tmp/proj", "a", "x");
  writeSessionContext(paths, "/tmp/proj", "b", "y");
  expect(listSessionIds(paths, "/tmp/proj").sort()).toEqual(["a", "b"]);
});
