# Release Notes

## Build

Generate embedded runtime assets, then compile the standalone executable:

```bash
bun run build
```

This produces `dist/zbrain`.

## Packaging Strategy

Validated in this repo:

- standalone compile via `bun build --compile`
- binary smoke checks with `./dist/zbrain --help`
- binary runtime extraction with `ZBRAIN_HOME=/tmp/... ./dist/zbrain setup`

Platform packaging guidance:

- macOS arm64: run `bun run build` on a macOS arm64 runner
- Linux x64: run `bun run build` on a Linux x64 runner
- Windows x64: run `bun run build` on a Windows x64 runner

Inference:

- local `bun build --help` exposes `--compile` for standalone executables and Windows-specific compile flags
- the local help output in this environment does not expose an explicit cross-platform target triple for standalone executables
- the reproducible manual path is therefore one native build per target platform class

## Binary Smoke Checks

```bash
./dist/zbrain --help
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup
```

Expected outcomes:

- help output lists `setup`, `init`, `workspace`, and `update`
- `setup` extracts `engine/`, `templates/`, `commands/`, `agents/`, and `workspaces/`

## Release Checklist

1. Run `bun test --run`
2. Run `bunx tsc --noEmit`
3. Run `bun run build`
4. Run the binary smoke checks above
5. Publish platform-native binaries from matching runners
