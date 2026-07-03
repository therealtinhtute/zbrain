// YAML frontmatter parser/serializer.
// Hand-rolled for the limited key set used by Note (id, tier, status, ...).
// Uses `js-yaml` (already in package.json) for the YAML subset.

import YAML from "js-yaml";

const FENCE = "---\n";
const CLOSE_MARKER = "\n---\n";

export interface ParsedMarkdown {
  frontmatter: Record<string, unknown>;
  body: string;
}

export function parseFrontmatter(markdown: string): ParsedMarkdown {
  if (!markdown.startsWith(FENCE)) {
    return { frontmatter: {}, body: markdown };
  }

  const end = markdown.indexOf(CLOSE_MARKER, FENCE.length);
  if (end === -1) {
    return { frontmatter: {}, body: markdown };
  }

  const yamlText = markdown.slice(FENCE.length, end);
  const body = markdown.slice(end + CLOSE_MARKER.length);

  let frontmatter: Record<string, unknown> = {};
  try {
    const parsed = YAML.load(yamlText);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      frontmatter = parsed as Record<string, unknown>;
    }
  } catch {
    // malformed frontmatter passes through unchanged
  }

  return { frontmatter, body };
}

export function serializeMarkdown(frontmatter: Record<string, unknown>, body: string): string {
  if (Object.keys(frontmatter).length === 0) {
    return body;
  }
  const yamlText = YAML.dump(frontmatter, { noRefs: true }).trimEnd();
  return `${FENCE}${yamlText}${CLOSE_MARKER}${body}`;
}
