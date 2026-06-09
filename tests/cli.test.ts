import { describe, expect, test } from "bun:test";
import { createProgram } from "../src/index";

describe("createProgram", () => {
  test("registers the public CLI command surface", () => {
    const program = createProgram();
    const commandNames = program.commands.map((command) => command.name());

    expect(commandNames).toEqual(["setup", "init", "workspace", "learn", "ingest", "ask", "update"]);
  });

  test("registers workspace create", () => {
    const program = createProgram();
    const workspace = program.commands.find((command) => command.name() === "workspace");

    expect(workspace?.commands.map((command) => command.name())).toEqual(["create"]);
  });

  test("registers ingest workflow commands", () => {
    const program = createProgram();
    const ingest = program.commands.find((command) => command.name() === "ingest");

    expect(ingest?.commands.map((command) => command.name())).toEqual(["list", "analyze", "qa", "apply"]);
  });
});
