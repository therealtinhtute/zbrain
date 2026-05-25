import { describe, expect, test } from "bun:test";
import { createProgram } from "../src/index";

describe("createProgram", () => {
  test("registers the public CLI command surface", () => {
    const program = createProgram();
    const commandNames = program.commands.map((command) => command.name());

    expect(commandNames).toEqual(["setup", "init", "workspace", "update"]);
  });

  test("registers workspace create", () => {
    const program = createProgram();
    const workspace = program.commands.find((command) => command.name() === "workspace");

    expect(workspace?.commands.map((command) => command.name())).toEqual(["create"]);
  });
});
