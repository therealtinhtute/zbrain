# Phase Plan: P7 — Build + Release

Inputs: All previous phases complete
Depends on: P1-P6

---

## Wave 1: Build Pipeline

### Task 1.1: Finalize build.ts
- Update build.ts for production:
  - Compile src/index.ts with all dependencies bundled
  - Embed all assets/* files as importable strings
  - Output: ./zwiki (default target: current platform)
  - Add cross-compile targets: --target=bun-darwin-arm64, bun-darwin-x64, bun-linux-x64
- Add package.json scripts:
  - "build": "bun run build.ts"
  - "build:all": "bun run build.ts --all" (all platforms)
- **Verification**: `bun run build` produces ./zwiki binary
- **Verification**: `./zwiki --help` shows all commands
- **Verification**: `./zwiki --version` shows version from package.json
- **Touched**: build.ts, package.json

### Task 1.2: Asset embedding verification
- After build, test asset extraction:
  1. Remove ~/.zwiki/ if exists
  2. Run `./zwiki setup`
  3. Verify all files extracted to ~/.zwiki/
  4. Diff extracted files against assets/ source → must match
- **Verification**: All engine/, templates/, commands/, agents/ files present and identical
- **Touched**: (verification only, no files changed)

---

## Wave 2: Example Content

### Task 2.1: Seed programming workspace
- Create example entries in assets/ that will be part of the "programming" workspace template or documentation:
  - axioms/premature-optimization.md: "Premature optimization is the root of all evil — Knuth"
  - mental-models/yagni-kiss-dry.md: YAGNI, KISS, DRY frameworks with examples
  - projects/clean-code-book.md: Key takeaways from Clean Code (Robert Martin)
- Each entry has proper YAML frontmatter (title, priority, source, created_at)
- **Verification**: Entries parseable by markdown parser, frontmatter valid
- **Touched**: assets/ or tests/fixtures/ (example content for docs/tests)

---

## Wave 3: Documentation

### Task 3.1: README.md
- Write README.md (Vietnamese) covering:
  - Giới thiệu: zwiki là gì, cho ai
  - Cài đặt: download binary, zwiki setup, install qmd
  - Khởi tạo: zwiki workspace create, zwiki init
  - Sử dụng: /ask, /learn, /reflect, /workspace, /reindex
  - Kiến trúc: system overview diagram (from SPEC)
  - Cấu trúc thư mục: ~/.zwiki/ layout
  - Decisions: link to SPEC key decisions
- **Verification**: README renders correctly in GitHub markdown preview
- **Touched**: README.md

### Task 3.2: Architecture doc
- Write ARCHITECTURE.md (Vietnamese) with:
  - Retrieval pipeline diagram + explanation
  - Evidence pipeline diagram + explanation
  - Workspace isolation mechanism
  - qmd BM25 integration details
- **Verification**: Diagrams render in markdown
- **Touched**: ARCHITECTURE.md

---

## Wave 4: Release

### Task 4.1: Version + release prep
- Set version in package.json (0.1.0)
- Create CHANGELOG.md with MVP-1 release notes
- Tag release: git tag v0.1.0
- Build all platform binaries
- **Verification**: All 3 binaries exist (darwin-arm64, darwin-x64, linux-x64)
- **Touched**: package.json, CHANGELOG.md

### Task 4.2: Smoke test on clean environment
- In a temp directory (simulating new user):
  1. Copy binary to ~/.local/bin/zwiki
  2. `zwiki setup` → ~/.zwiki/ created
  3. Install qmd: `npm i -g @tobilu/qmd`
  4. `zwiki workspace create programming`
  5. Add example axiom manually
  6. `qmd --config-name zwiki index`
  7. `zwiki init` in a test project
  8. Open Claude Code → run `/ask "what is premature optimization?"` → should return the axiom
- **Verification**: Full user journey works on clean machine
- **Touched**: (verification only)

---

## Stop Conditions
- Binary too large (>100MB) → investigate Bun compile tree-shaking
- Assets not found at runtime → debug embedding approach

## Escalation
- Bun cross-compile doesn't work → build on each target platform separately
- GitHub Actions needed for CI → set up .github/workflows/release.yml (deferred if manual release acceptable for MVP)
