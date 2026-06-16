---
name: zbrain:research
description: Search the web for a topic, fetch top sources, and record each as an evidence item in the active workspace. Use before zbrain:ingest to populate evidence from the internet or docs.
argument-hint: "[topic]"
disable-model-invocation: true
version: "1.1.0"
---

<role>
Act as a web research entry point for zbrain. Discover sources for a topic, fetch their content, and create evidence items via zbrain:learn. Do not analyze, QA, apply, or answer from the fetched content.
</role>

<security>
- Record evidence only in the active workspace; never cross workspace boundaries
- Do not record login pages, paywall gates, or empty pages as evidence
- Do not fabricate sources; only record content actually fetched
- Never analyze, apply, or answer from fetched content inside zbrain:research
</security>

<instructions>
## Input

```
zbrain:research "topic or question"
zbrain:research "topic" --source exa
zbrain:research "topic" --source websearch
zbrain:research "topic" --source context7
zbrain:research --url https://example.com
zbrain:research --urls ./urls.txt
zbrain:research "topic" --limit 5
```

Default source: Exa. Default limit: 3.

## Step 2 — Source Dispatch

Route based on input type. Each method must produce a list of candidate URLs. Steps 3–6 are identical regardless of method.

| Input | Method | Tool |
|-------|--------|------|
| Topic string (default) | Exa semantic search | `mcp__exa__web_search_exa` |
| Exa unavailable | WebSearch fallback | `WebSearch` |
| `--source websearch` | Force WebSearch | `WebSearch` |
| `--source context7` | Library / framework docs | `mcp__claude_ai_Context7__resolve-library-id` → `mcp__claude_ai_Context7__query-docs` |
| `--url https://...` | Single URL, skip search | go directly to Step 4 |
| `--urls ./file.txt` | Batch URLs from file | go directly to Step 4 |

To add a new source method: add one row to this table and implement its handler here. Nothing else changes.

## Flow

Print each step inline as it completes.

```
🥷 zbrain:research — "{topic}"

Step 1/6  Workspace   {workspace}
Step 2/6  Search      {method} → {n} candidates
Step 3/6  Filter      {kept} selected  ({dropped} dropped: {reasons})
Step 4/6  Fetch       ✓ {url1}  ✓ {url2}  ✗ {url3 — reason}
Step 5/6  Learn       {evid-id-1}  {evid-id-2}
Step 6/6  Report

Evidence created: {n} · Workspace: {workspace}
Next: zbrain:ingest analyze {first-evid-id}
```

### Step 1 — Workspace
Resolve active workspace from `~/.zbrain/projects.json` by matching the current project root, fallback to `~/.zbrain/config.yml`. Stop and report if neither resolves.

### Step 2 — Search
Apply the source dispatch table above.

- **Exa (default):** query = `{topic} documentation OR tutorial OR guide`. Retrieve 5–8 candidates. If Exa returns zero results, retry once with `WebSearch` regardless of `--source` flag.
- **WebSearch:** same query string. Retrieve 5–8 candidates.
- **Context7:** call `mcp__claude_ai_Context7__resolve-library-id` with the topic to get a library ID, then `mcp__claude_ai_Context7__query-docs` with that ID. Extract doc section URLs as candidates.
- **`--url`:** single URL goes directly to Step 4; skip Steps 2 and 3.
- **`--urls`:** read one URL per line from the file; go directly to Step 4.

### Step 3 — Filter
For each candidate URL, fetch the first 10 lines only. Drop if any of the following are found:
- Paywall signals: `Sign in`, `Subscribe`, `Continue reading`, `Create account`, `Log in to read`
- Empty body or HTTP error
- Duplicate domain + path already in the candidate list

Keep up to `--limit` survivors (default 3). If fewer than 2 survive filtering, lower the paywall threshold and retry all dropped candidates once before reporting zero results.

### Step 4 — Fetch

Use the same routing logic as the `/read` skill. Route each URL to the right method before fetching. Full command details: `~/.claude/skills/read/references/read-methods.md`.

**Routing table:**

| URL pattern | Method |
|-------------|--------|
| `github.com/*/blob/*`, `raw.githubusercontent.com` | Raw content: `curl -sL https://raw.githubusercontent.com/{user}/{repo}/{branch}/{path}` or `gh api` for private repos. Proxy cascade as fallback. |
| `*.pdf` or URL ending in `.pdf` | `curl -sL "https://r.jina.ai/{url}"` first; fallback: `pdftotext -layout /tmp/input.pdf -` |
| `feishu.cn`, `larksuite.com` | Feishu API script at `~/.claude/skills/read/scripts/fetch_feishu.py` |
| `mp.weixin.qq.com`, `x.com`, `twitter.com` | Proxy cascade only (`r.jina.ai`); never use `WebFetch` directly |
| Everything else | Proxy cascade (see below) |

**Proxy cascade (in order — stop at first non-empty result):**
1. `curl -sL "https://defuddle.md/{url}"` — strips nav/ads, clean Markdown with frontmatter
2. `curl -sL "https://r.jina.ai/{url}"` — wide coverage, preserves image links
3. `npx agent-fetch "{url}" --json` or `defuddle parse "{url}" -m -j` — local fallback; extract the Markdown field from JSON output

**Token-saving rules (enforce on every fetch):**
- Never output the raw fetched content to the conversation — pass it directly to Step 5.
- Truncate to 200 lines before passing to `zbrain:learn`. Longer pages carry boilerplate that inflates the evidence with noise.
- Prefer defuddle.md in the cascade: it strips navigation, ads, and footers, leaving only the article body. Less content = fewer tokens = cleaner evidence.
- If the fetched Markdown is empty after truncation, treat as fetch failure.

On failure for a single source: mark `✗ {url} — {reason}` in the output and continue. Never abort the full run for one failed fetch.

### Step 5 — Learn
For each successfully fetched source:
```
zbrain:learn --type web --origin {url} --label {page-title}
```
One evidence item per source. Do not modify `raw.md` or `source.yaml` after creation.

### Step 6 — Report
Print the summary block. List all created evidence IDs. Show the next command for the first ID.

## Invariants

- Never run ingest, QA, apply, or answer inside `zbrain:research`.
- Never record a source that failed Step 3 filtering.
- Never mix evidence across workspaces.
- `--url` / `--urls` bypass Steps 2 and 3 entirely.
- `raw.md` and `source.yaml` are immutable after Step 5 creates them.
</instructions>
