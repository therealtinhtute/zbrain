import { assertRuntimeReady, createCommandContext, summarizeExtraction, updateRuntime } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface UpdateCommandOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
}

export async function runUpdate(options: UpdateCommandOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);

  ui.intro("zbrain update");
  const spinner = ui.spinner();
  spinner.start("Refreshing bundled assets");
  const result = updateRuntime(context.paths);
  spinner.stop("Assets refreshed");
  ui.note(summarizeExtraction(result), "Update summary");
  ui.outro("Assets updated.");
}
