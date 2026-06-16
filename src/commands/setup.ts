import { createCommandContext, checkQmdInstalled, setupRuntime, summarizeExtraction } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { initDb } from "../core/db";
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

  const dbSpinner = ui.spinner();
  dbSpinner.start("Initializing database");
  initDb(context.paths.runtimeDir);
  dbSpinner.stop("Database ready");

  const qmdSpinner = ui.spinner();
  qmdSpinner.start("Checking qmd");
  const qmdStatus = checkQmdInstalled();
  qmdSpinner.stop(qmdStatus.installed ? "qmd detected" : "qmd not found");

  const searchProviders = [
    { name: "Exa", envKey: "EXA_API_KEY" },
    { name: "Brave", envKey: "BRAVE_API_KEY" },
    { name: "Firecrawl", envKey: "FIRECRAWL_API_KEY" },
    { name: "Tavily", envKey: "TAVILY_API_KEY" },
  ];
  const configuredProviders = searchProviders.filter((p) => process.env[p.envKey]);
  const searchProviderNote =
    configuredProviders.length > 0
      ? `search providers: ${configuredProviders.map((p) => p.name).join(", ")}`
      : "search providers: none detected (zbrain:research falls back to built-in WebSearch)";

  ui.note(
    [
      summarizeExtraction(runtimeResult),
      `config_created: ${runtimeResult.configCreated ? "yes" : "no"}`,
      `qmd: ${qmdStatus.installed ? qmdStatus.version : "missing"}`,
      searchProviderNote,
    ].join("\n"),
    "Setup summary",
  );

  if (!qmdStatus.installed) {
    ui.info("Install qmd separately: npm i -g @tobilu/qmd");
  }

  if (configuredProviders.length === 0) {
    ui.info(
      "No web search API key detected. Set EXA_API_KEY, BRAVE_API_KEY, FIRECRAWL_API_KEY, or TAVILY_API_KEY for better zbrain:research results.",
    );
  }

  ui.outro("Setup complete.");
}
