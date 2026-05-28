---
name: zbrain:learn
description: Actively research a topic via web search, fetch sources as clean Markdown, filter for quality, and create one evidence item per source in the active workspace. Use when you want to explore something new, not when you already have a source in hand.
disable-model-invocation: true
version: "3.0.0"
---

<role>
Act as a research driver. Search the web for a topic, fetch primary sources as LLM-readable Markdown, filter for quality, and create one evidence item per source via zbrain:ingest. Route the user to zbrain:state when done.
</role>

<security>
- Read only from the active workspace — never cross workspace boundaries
- Do not apply knowledge directly — always create evidence items via zbrain:ingest
- Do not fabricate sources — only cite material you have actually fetched
- Do not ingest a login page, paywall, or empty page as an evidence item
</security>

<instructions>
## Input

Takes a topic, question, or learning goal as the argument:

```
zbrain:learn "how BM25 ranking works"
zbrain:learn "workspace isolation patterns in CLI tools"
zbrain:learn "mental models for decision-making under uncertainty"
```

If no argument is given, prompt the user for a topic before proceeding.

---

## Phase 1 — Workspace Scan

1. Resolve active workspace from `<cwd>/.claude/zbrain.json` or `~/.zbrain/config.yml`. Stop if workspace cannot be resolved.
2. Run `zbrain:ask "{topic}"` to surface existing workspace knowledge.
3. Identify gaps: what is not yet covered, what is outdated, what is worth researching.
4. If the workspace already has thorough coverage of the topic, report it and ask the user to confirm before continuing.

---

## Phase 2 — Web Discovery

1. Use the available web search tool to search for the topic.
   - If no web search tool is installed: ask the user to provide 3–10 URLs directly and skip to Phase 3.
2. Run 2–3 searches covering different angles of the topic (e.g., "X overview", "X implementation", "X research paper").
3. Collect 15–25 candidate URLs. Output as a list — do not fetch content yet.
4. Prefer: academic papers, official documentation, official product/lab blogs, repositories by authors of the topic.
5. Deprioritize: generic explainer blogs, aggregator pages, SEO listicles, paywalled abstracts.

---

## Phase 3 — Filter

Apply the primary-source filter before fetching anything:

| Keep | Skip |
|------|------|
| Paper or report by the people who built/defined the concept | Third-party explainer or summary |
| Official documentation or changelog | Blog post that only links to papers |
| Repository authored by the creator | Aggregator or "top 10" list |
| Post by a practitioner showing real implementation | Abstract with no accessible full text |

Target: **5–10 sources**. Cut the rest.

When two sources cover the same point, keep the one closer to the primary author. If unsure, keep both and note the overlap.

Present the filtered list to the user. Ask for confirmation before fetching.

---

## Phase 4 — Fetch & Ingest

For each confirmed source URL:

### 4a. Fetch as Markdown

Use the proxy cascade from `references/fetch.md`. Try in order until one returns non-empty, readable content:

1. `defuddle.md` — preferred output quality
2. `r.jina.ai` — wide coverage, preserves image links
3. Native `WebFetch` — fallback, degrades on JS-heavy or paywalled pages

**Paywall check:** Inspect the first 10 lines of the fetched content. If any of these appear — "Subscribe", "Sign in", "Continue reading", "Create an account" — stop, skip this URL, and warn the user. Do not ingest the login page.

**Output format per source:**

```
Title:  {title}
Author: {author or organization}
Source: {domain}
URL:    {original url}

{full Markdown content}
```

### 4b. Create Evidence Item

Call `zbrain:ingest` with the fetched Markdown as the source material. This creates:
- `evidence/sources/{id}/raw.md` — the fetched Markdown (immutable after ingest)
- `evidence/sources/{id}/source.yaml` — metadata: id, title, url, source_type: web, workspace_at_ingest, ingested_at, state: ingested

Record the created evidence ID.

### 4c. Repeat

Process each URL sequentially. If a fetch fails after all proxy attempts, skip the URL and log it in the failure list.

---

## Phase 5 — Report

Output a summary:

```
Research complete for: "{topic}"
Active workspace: {name}

Created {n} evidence items:
  {id}  "{title}"  ({domain})
  ...

Skipped ({n}):
  {url}  — {reason: paywall / fetch failed / filtered}

Next step: run zbrain:state to see pipeline progress and queue items for analysis.
```

---

## Invariants

- Never apply updates directly — always route through `zbrain:ingest`.
- Never ingest login pages, empty pages, or paywall gates.
- Never suppress contradictions with existing workspace knowledge — surface them in Phase 1.
- Primary-source filter runs before fetching — do not fetch low-quality sources.
- If a source URL already exists in `_index.md`, skip it and note the duplicate.
</instructions>

<references>
Load as needed from `{baseDir}/references/`:
- `fetch.md` — proxy cascade commands for URL-to-Markdown conversion
</references>
