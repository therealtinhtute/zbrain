import { createCommandContext, checkQmdInstalled, setupRuntime, summarizeExtraction } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface SetupCommandOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
}

export async function runSetup(options: SetupCommandOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);

  ui.intro("zbrain setup");
  const extractionSpinner = ui.spinner();
  extractionSpinner.start("Extracting bundled assets");
  const runtimeResult = setupRuntime(context.paths);
  extractionSpinner.stop("Assets extracted");

  const qmdSpinner = ui.spinner();
  qmdSpinner.start("Checking qmd");
  const qmdStatus = checkQmdInstalled();
  qmdSpinner.stop(qmdStatus.installed ? "qmd detected" : "qmd not found");

  ui.note(
    [
      summarizeExtraction(runtimeResult),
      `config_created: ${runtimeResult.configCreated ? "yes" : "no"}`,
      `qmd: ${qmdStatus.installed ? qmdStatus.version : "missing"}`,
    ].join("\n"),
    "Setup summary",
  );

  if (!qmdStatus.installed) {
    ui.info("Install qmd separately: npm i -g @tobilu/qmd");
  }

  ui.outro("Setup complete.");
}
