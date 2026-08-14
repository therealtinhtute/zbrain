# Bundled Assets

The root §assets/§ directory is the source of truth for runtime content bundled
into the Go binary through §assets/embed.go§.

§zbrain setup§ walks the embedded filesystem and copies these paths directly
under the selected runtime root:

- §README.md§
- §agents/§
- §engine/§
- §skills/§ and skill references
- §templates/§

The extractor skips any embedded §workspaces/§ seed content. Setup therefore
does not activate a workspace; §zbrain workspace create <name>§ creates the
selected workspace and §zbrain reindex§ creates its disposable SQLite FTS5
index under the runtime §indexes/§ directory.

When authoring assets, preserve the runtime contracts: skill files need
§name§, §description§, and §version§ frontmatter; templates must keep the
placeholder tokens expected by the Go scaffold logic; and evidence metadata
must keep the §source.yaml§ shape used by §internal/runtime/evidence.go§.
