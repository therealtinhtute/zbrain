import { test, expect } from "bun:test";
import pkg from "../package.json";

test("smoke: package name is zbrain", () => {
  expect(pkg.name).toBe("zbrain");
});

test("smoke: package has a test script", () => {
  expect(typeof pkg.scripts?.test).toBe("string");
  expect(pkg.scripts.test).toContain("bun test");
});

test("smoke: package version is parseable semver", () => {
  expect(pkg.version).toMatch(/^\d+\.\d+\.\d+/);
});
