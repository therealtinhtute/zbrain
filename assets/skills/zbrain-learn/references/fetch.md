# Fetch Reference — URL to LLM-Readable Markdown

Use this reference during Phase 4 of `zbrain:learn`. Try methods in order; stop at the first that returns non-empty, readable content (≥ 5 lines of real prose).

## Proxy Cascade

### 1. defuddle.md (try first)

```bash
curl -sL "https://defuddle.md/{url}"
```

Returns clean Markdown with YAML frontmatter. Best output quality for articles and documentation. If the response is empty, fewer than 5 lines, or contains only navigation HTML, treat as failed.

### 2. r.jina.ai (try second)

```bash
curl -sL "https://r.jina.ai/{url}"
```

Wide coverage. Preserves image URLs. Works on most web pages including some JS-rendered content.

### 3. Native WebFetch (fallback)

Use the environment's built-in `WebFetch` tool as a last resort. Coverage degrades on:
- JavaScript-rendered single-page apps
- Paywalled articles
- Pages requiring login

When falling back to `WebFetch`, note in the Phase 5 report that coverage was degraded for that source.

## GitHub URLs

For GitHub file URLs (`github.com/user/repo/blob/...`), prefer raw content over the proxy cascade:

```bash
# Convert blob URL to raw URL manually, then:
curl -sL "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{path}"

# For private repos:
gh api repos/{user}/{repo}/contents/{path} --jq '.content' | base64 -d
```

Use the proxy cascade only for GitHub pages that are not raw file views (issue threads, wiki pages, README renders).

## PDF URLs

r.jina.ai handles PDF URLs directly:

```bash
curl -sL "https://r.jina.ai/{pdf_url}"
```

If that fails, download and extract locally:

```bash
curl -sL "{pdf_url}" -o /tmp/zblearn-input.pdf
pdftotext -layout /tmp/zblearn-input.pdf -
```

## Paywall Detection

After fetching, inspect the first 10 lines of the output. If any of these strings appear, the fetch returned a login or paywall page — skip this URL and warn the user:

- "Subscribe"
- "Sign in"
- "Continue reading"
- "Create an account"
- "Log in to read"
- "Members only"

Do not save or ingest paywall pages.

## Output Format

Once a successful fetch is obtained, format the content as:

```
Title:  {title from frontmatter or <title> tag}
Author: {author or organization, if available}
Source: {domain}
URL:    {original url}

{full Markdown body}
```

Truncate at 400 lines if the content is very long. Note truncation in the Phase 5 report.
