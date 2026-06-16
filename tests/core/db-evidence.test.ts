import { describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { initDb } from "../../src/core/db";
import {
  insertEvidence,
  readEvidence,
  listEvidence,
  updateEvidenceState,
  verifyEvidenceIntegrity,
} from "../../src/core/db-evidence";
import { buildSourceRecord } from "../../src/core/evidence-store";

function setup() {
  const dir = mkdtempSync(join(tmpdir(), "zbrain-ev-"));
  const db = initDb(dir);
  return { dir, db };
}

function teardown(dir: string, db: ReturnType<typeof initDb>) {
  db.close();
  rmSync(dir, { recursive: true, force: true });
}

const NOW = "2026-06-16T00:00:00.000Z";
const RAW_CONTENT = "# Test Content\n\nHello world.";

function makeRecord(id = "2026-06-16-web-test") {
  return buildSourceRecord({
    id,
    type: "web",
    origin: "https://example.com",
    label: "test",
    workspace_at_ingest: "research",
    ingested_at: NOW,
    state: "ingested",
    raw_filename: "raw.md",
    rawContent: RAW_CONTENT,
  });
}

describe("insertEvidence / readEvidence", () => {
  test("roundtrip: insert and read back", () => {
    const { dir, db } = setup();
    try {
      const record = makeRecord();
      insertEvidence(db, record, NOW);
      const row = readEvidence(db, "research", record.id);
      expect(row?.id).toBe(record.id);
      expect(row?.source_type).toBe("web");
      expect(row?.origin).toBe("https://example.com");
      expect(row?.label).toBe("test");
      expect(row?.workspace_at_ingest).toBe("research");
      expect(row?.state).toBe("ingested");
      expect(row?.raw_sha256).toBe(record.raw_sha256);
      expect(row?.source_sha256).toBe(record.source_sha256);
    } finally {
      teardown(dir, db);
    }
  });

  test("returns null for missing evidence", () => {
    const { dir, db } = setup();
    try {
      expect(readEvidence(db, "research", "nonexistent")).toBeNull();
    } finally {
      teardown(dir, db);
    }
  });

  test("workspace isolation: same id different workspace", () => {
    const { dir, db } = setup();
    try {
      const record = makeRecord();
      insertEvidence(db, record, NOW);
      expect(readEvidence(db, "finance", record.id)).toBeNull();
    } finally {
      teardown(dir, db);
    }
  });
});

describe("listEvidence", () => {
  test("returns only items for the given workspace", () => {
    const { dir, db } = setup();
    try {
      const r1 = buildSourceRecord({
        id: "ev-1",
        type: "web",
        origin: "https://a.com",
        label: "a",
        workspace_at_ingest: "research",
        ingested_at: NOW,
        state: "ingested",
        raw_filename: "raw.md",
        rawContent: "a",
      });
      const r2 = buildSourceRecord({
        id: "ev-2",
        type: "paste",
        origin: "cli",
        label: "b",
        workspace_at_ingest: "finance",
        ingested_at: NOW,
        state: "ingested",
        raw_filename: "raw.md",
        rawContent: "b",
      });
      insertEvidence(db, r1, NOW);
      insertEvidence(db, r2, NOW);
      const items = listEvidence(db, "research");
      expect(items).toHaveLength(1);
      expect(items[0]?.id).toBe("ev-1");
    } finally {
      teardown(dir, db);
    }
  });
});

describe("updateEvidenceState", () => {
  test("valid transition updates state", () => {
    const { dir, db } = setup();
    try {
      const record = makeRecord();
      insertEvidence(db, record, NOW);
      updateEvidenceState(db, "research", record.id, "analyzed", NOW);
      const row = readEvidence(db, "research", record.id);
      expect(row?.state).toBe("analyzed");
    } finally {
      teardown(dir, db);
    }
  });

  test("invalid transition throws", () => {
    const { dir, db } = setup();
    try {
      const record = makeRecord();
      insertEvidence(db, record, NOW);
      expect(() =>
        updateEvidenceState(db, "research", record.id, "applied", NOW),
      ).toThrow("Invalid evidence transition");
    } finally {
      teardown(dir, db);
    }
  });

  test("throws for missing evidence", () => {
    const { dir, db } = setup();
    try {
      expect(() =>
        updateEvidenceState(db, "research", "nonexistent", "analyzed", NOW),
      ).toThrow("Evidence not found");
    } finally {
      teardown(dir, db);
    }
  });
});

describe("verifyEvidenceIntegrity", () => {
  test("passes for correct raw content", () => {
    const { dir, db } = setup();
    try {
      const record = makeRecord();
      insertEvidence(db, record, NOW);
      // Advance state — fingerprint must still pass (computed against ingested state)
      updateEvidenceState(db, "research", record.id, "analyzed", NOW);
      expect(() =>
        verifyEvidenceIntegrity(db, "research", record.id, RAW_CONTENT),
      ).not.toThrow();
    } finally {
      teardown(dir, db);
    }
  });

  test("fails when raw content is tampered", () => {
    const { dir, db } = setup();
    try {
      const record = makeRecord();
      insertEvidence(db, record, NOW);
      expect(() =>
        verifyEvidenceIntegrity(db, "research", record.id, "tampered content"),
      ).toThrow("raw.md fingerprint mismatch");
    } finally {
      teardown(dir, db);
    }
  });

  test("throws for missing evidence", () => {
    const { dir, db } = setup();
    try {
      expect(() =>
        verifyEvidenceIntegrity(db, "research", "nonexistent", RAW_CONTENT),
      ).toThrow("Evidence not found");
    } finally {
      teardown(dir, db);
    }
  });
});
