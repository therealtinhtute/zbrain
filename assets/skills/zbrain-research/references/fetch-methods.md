# Fetch Methods Reference — zbrain:research

## Tier 1: Exa MCP Fetch

Call `mcp__exa__web_fetch_exa` with the URL. Returns LLM-optimized clean content.

**Validate response**: must have >5 non-HTML lines. If response is empty or looks like raw HTML (lines starting with `<`, `<!DOCTYPE`), treat as failure and fall through to the next tier.

Skip this tier for: `.pdf` URLs, `raw.githubusercontent.com`, and `github.com/*/blob/*`.

## Tier 2: Firecrawl Scrape (requires FIRECRAWL_API_KEY)

```bash
curl -sL -X POST "https://api.firecrawl.dev/v1/scrape" \
  -H "Authorization: Bearer $FIRECRAWL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"url":"{url}","formats":["markdown"],"onlyMainContent":true}'
```

Returns JSON. Extract the `.data.markdown` field. Best tier for JS-rendered pages, SPAs, and paywalled news with a JavaScript gate.

If `FIRECRAWL_API_KEY` is not set, skip this tier silently.

## Tier 3: Proxy Cascade (scripts/fetch.sh)

Try in order. Success = non-empty output, >5 readable lines. If a proxy returns empty, an error page, or fewer than 5 lines, try the next:

### 3a. defuddle.md

```bash
curl -sL "https://defuddle.md/{url}"
```

Strips nav/ads, returns clean Markdown with YAML frontmatter. Try this first.

### 3b. r.jina.ai

```bash
curl -sL "https://r.jina.ai/{url}"
```

Wide coverage, preserves image links. Use if defuddle.md returns empty or errors.

### 3c. Local tools (last resort)

```bash
npx agent-fetch "{url}" --json
# or
defuddle parse "{url}" -m
```

`agent-fetch --json` returns JSON — extract the Markdown-bearing field. `defuddle parse -m` outputs Markdown directly. Raw JSON is not valid final output.

The full cascade is wrapped in `scripts/fetch.sh`:

```bash
bash scripts/fetch.sh --use-proxy "{url}"
```

## GitHub Raw Content

GitHub file URLs render heavy HTML. Prefer raw:

```bash
# Raw file (fastest)
curl -sL "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{path}"

# Via gh CLI (works with private repos)
gh api repos/{user}/{repo}/contents/{path} --jq '.content' | base64 -d
```

Use the proxy cascade only for GitHub pages that are not raw file views (e.g. issue threads, README renders).

## PDF to Markdown

### Remote PDF URL

r.jina.ai handles PDF URLs directly:

```bash
curl -sL "https://r.jina.ai/{pdf_url}"
```

If that fails, download and extract locally:

```bash
curl -sL "{pdf_url}" -o /tmp/input.pdf
pdftotext -layout /tmp/input.pdf -
```

### Local PDF

```bash
# Best quality (requires: pip install marker-pdf)
marker_single /path/to/file.pdf --output_dir /tmp/zbrain-fetch

# Fast, text-heavy PDFs (requires: brew install poppler)
pdftotext -layout /path/to/file.pdf - | sed 's/\f/\n---\n/g'

# No-dependency fallback
python3 -c "
import pypdf, sys
r = pypdf.PdfReader(sys.argv[1])
print('\n\n'.join(p.extract_text() for p in r.pages))
" /path/to/file.pdf
```
