import { describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { openDb, initDb, SCHEMA_VERSION } from "../../src/core/db";

function makeTempDir(): string {
  return mkdtempSync(join(tmpdir(), "zbrain-db-"));
}

describe("openDb", () => {
  test("creates zbrain.db in the runtime dir", () => {
    const dir = makeTempDir();
    try {
      const db = openDb(dir);
      db.close();
      expect(require("node:fs").existsSync(join(dir, "zbrain.db"))).toBe(true);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("WAL mode is enabled", () => {
    const dir = makeTempDir();
    try {
      const db = openDb(dir);
      const row = db.prepare("PRAGMA journal_mode").get() as { journal_mode: string };
      db.close();
      expect(row.journal_mode).toBe("wal");
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});

describe("initDb", () => {
  test("creates all four tables", () => {
    const dir = makeTempDir();
    try {
      const db = initDb(dir);
      const rows = db
        .prepare("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
        .all() as Array<{ name: string }>;
      db.close();
      const tableNames = rows.map((r) => r.name);
      expect(tableNames).toContain("projects");
      expect(tableNames).toContain("evidence_sources");
      expect(tableNames).toContain("sessions");
      expect(tableNames).toContain("queries");
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("sets schema version to 1", () => {
    const dir = makeTempDir();
    try {
      const db = initDb(dir);
      const row = db.prepare("PRAGMA user_version").get() as { user_version: number };
      db.close();
      expect(row.user_version).toBe(SCHEMA_VERSION);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("is idempotent — safe to call multiple times", () => {
    const dir = makeTempDir();
    try {
      const db1 = initDb(dir);
      db1.close();
      const db2 = initDb(dir);
      const row = db2.prepare("PRAGMA user_version").get() as { user_version: number };
      db2.close();
      expect(row.user_version).toBe(SCHEMA_VERSION);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("throws when schema version is newer than supported", () => {
    const dir = makeTempDir();
    try {
      const db = initDb(dir);
      db.exec(`PRAGMA user_version = ${SCHEMA_VERSION + 1}`);
      db.close();
      expect(() => initDb(dir)).toThrow("newer than supported");
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
