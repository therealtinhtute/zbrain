---
name: zbrain:research
description: Search the web for a topic, fetch top sources, and record each as an evidence item in the active workspace. Use before zbrain:ingest to populate evidence from the internet or docs.
argument-hint: "[topic]"
disable-model-invocation: true
version: "1.2.0"
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
zbrain:research "topic" --source brave
zbrain:research "topic" --source tavily
zbrain:research "topic" --source websearch
zbrain:research "topic" --source context7
zbrain:research --url https://example.com
zbrain:research --urls ./urls.txt
zbrain:research "topic" --limit 5
```

Default source: Exa (auto-fallback: Brave → Tavily → WebSearch). Default limit: 3.

## Source Dispatch Table

| Input / State | Search Method | Tool / Command |
|---------------|---------------|----------------|
| Topic string (default), Exa available | Exa semantic search | `mcp__exa__web_search_exa` |
| `--source exa` | Force Exa | `mcp__exa__web_search_exa` |
| `--source brave` | Force Brave | REST `api.search.brave.com` + `$BRAVE_API_KEY` |
| `--source tavily` | Force Tavily | Tavily MCP (requires auth) |
| `--source websearch` | Force built-in | `WebSearch` |
| `--source context7` | Library / framework docs | `mcp__claude_ai_Context7__resolve-library-id` → `mcp__claude_ai_Context7__query-docs` |
| `--url https://...` | Single URL, skip search | go directly to Step 5 |
| `--urls ./file.txt` | Batch URLs from file | go directly to Step 5 |
| Exa returns 0 results | Auto-fallback to Brave | check `$BRAVE_API_KEY` |
| Brave also unavailable | Auto-fallback to Tavily | try Tavily MCP |
| All search providers unavailable | Last resort | `WebSearch` |

To add a new source: add one row to this table and implement its handler in Step 3. Nothing else changes.

## Flow

Print each step inline as it completes.

```
🥷 zbrain:research — "{topic}"

Step 0/6  Providers   Exa ✓  Brave ✓  Firecrawl ✓  Tavily -  WebSearch ✓
Step 1/6  Workspace   {workspace}
Step 2/6  Search      {method} → {n} candidates
Step 3/6  Filter      {kept} selected  ({dropped} dropped: {reasons})
Step 4/6  Fetch       ✓ {url1}  ✓ {url2}  ✗ {url3 — reason}
Step 5/6  Learn       {evid-id-1}  {evid-id-2}
Step 6/6  Report

Evidence created: {n} · Workspace: {workspace}
Next: zbrain:ingest analyze {first-evid-id}
```

### Step 0 — Provider Check

Detect which search and fetch providers are reachable. Run before Step 1.

**Search providers (in priority order):**
1. **Exa** — call `mcp__exa__web_search_exa` with query `"zbrain provider check"`, limit 1. Mark ✓ if it returns without error; ✗ if it errors or is unavailable.
2. **Brave** — check `$BRAVE_API_KEY` env var. Mark ✓ if set, ✗ if missing.
3. **Tavily** — check if Tavily MCP tools are available in session. Mark ✓ if reachable, - if not.
4. **WebSearch** — always mark ✓ (built-in, always available as last resort).

**Fetch providers (checked silently, affects Step 4 cascade):**
- **Exa fetch** — available if Exa search ✓ above.
- **Firecrawl** — available if `$FIRECRAWL_API_KEY` is set.
- **Proxy cascade** (defuddle.md / r.jina.ai / local) — always available.

**Abort condition:** if all of Exa, Brave, and Tavily are ✗ AND WebSearch is also unavailable, stop and report: "No search provider available. Install Exa MCP or set BRAVE_API_KEY / TAVILY_API_KEY."

If only WebSearch is available, continue with a warning: "Falling back to built-in WebSearch — results may be less semantic than Exa."

### Step 1 — Workspace
Resolve active workspace from `~/.zbrain/projects.json` by matching the current project root, fallback to `~/.zbrain/config.yml`. Stop and report if neither resolves.

### Step 2 — Search

Apply the source dispatch table above.

- **Exa (default):** query = `{topic} documentation OR tutorial OR guide`. Retrieve 5–8 candidates. If Exa returns zero results, retry once with Brave (if `$BRAVE_API_KEY` set) or `WebSearch`.
- **Brave:** `curl -sL "https://api.search.brave.com/res/v1/web/search?q={encoded_query}&count=8" -H "Accept: application/json" -H "X-Subscription-Token: $BRAVE_API_KEY"`. Extract `web.results[].url`.
- **Tavily:** use Tavily MCP search tool with the topic as query. Retrieve 5–8 candidates.
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

Route each URL to the right method. Full command details in `references/fetch-methods.md`.

**Routing table:**

| URL pattern | Method |
|-------------|--------|
| `github.com/*/blob/*`, `raw.githubusercontent.com` | Raw: `curl -sL https://raw.githubusercontent.com/{user}/{repo}/{branch}/{path}` or `gh api` for private repos |
| `*.pdf` or URL ending in `.pdf` | Skip Exa fetch; use Firecrawl or `curl -sL "https://r.jina.ai/{url}"` |
| `mp.weixin.qq.com`, `x.com`, `twitter.com` | Proxy cascade only (`r.jina.ai`); never use Exa fetch directly |
| Everything else | Standard fetch cascade (see below) |

**Standard fetch cascade (stop at first non-empty, LLM-readable result):**

1. **`mcp__exa__web_fetch_exa`** — MCP tool, LLM-optimized. Call with the URL. Validate: response must have >5 non-HTML lines (reject if lines start with `<` or `<!DOCTYPE`). If invalid, fall through.
2. **Firecrawl scrape** (if `$FIRECRAWL_API_KEY` set) — best for JS-heavy pages and SPAs:
   ```bash
   curl -sL -X POST "https://api.firecrawl.dev/v1/scrape" \
     -H "Authorization: Bearer $FIRECRAWL_API_KEY" \
     -H "Content-Type: application/json" \
     -d '{"url":"{url}","formats":["markdown"],"onlyMainContent":true}'
   ```
   Extract `.data.markdown` from the JSON response.
3. **`bash scripts/fetch.sh --use-proxy "{url}"`** — defuddle.md → r.jina.ai → local extractor. Always available. See `references/fetch-methods.md` for full details.

**Token-saving rules (enforce on every fetch):**
- Never output raw fetched content to the conversation — pass it directly to Step 5.
- Truncate to 200 lines before passing to `zbrain:learn`. Longer pages carry boilerplate that inflates evidence with noise.
- Prefer Exa fetch and Firecrawl: both strip navigation and return article body only. Less content = fewer tokens = cleaner evidence.
- If the fetched Markdown is empty after truncation, treat as fetch failure.

On failure for a single source: mark `✗ {url} — {reason}` and continue. Never abort the full run for one failed fetch.

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
- Skip Firecrawl tier silently if `$FIRECRAWL_API_KEY` is not set — do not error.
</instructions>
