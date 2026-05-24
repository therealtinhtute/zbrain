#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Layer 1 validator for the wiki-template knowledge engine.

Source of truth for the Layer 1 rules referenced from
agents/pipeline/validator-rules.md. Standard-library only, cross-platform.

USAGE
-----
    python validate.py --file <path-to-code-file>
                       [--workspace <name>]
                       [--wiki-root <path>]
                       [--json | --pretty]

Examples
--------
    # Validate a Java file using workspace resolved from .claude/wiki.json
    python validate.py --file src/main/java/MyConsumer.java

    # Force a specific workspace and wiki root
    python validate.py --file MyConsumer.java \\
                       --workspace example-surgery \\
                       --wiki-root D:/tool/wiki-template

    # Run on the bundled test fixture (intentional violations)
    python validate.py --file scripts/test-fixtures/bad-consumer.java --pretty

EXIT CODE
---------
    0 — no errors (warnings allowed)
    1 — at least one error-severity violation
    2 — bad invocation (missing file, unresolved workspace, etc.)

OUTPUT
------
    JSON to stdout:
    {
      "violations": [
        {"rule": str, "severity": "error"|"warn",
         "file": str, "line": int, "snippet": str, "message": str}
      ],
      "summary": {"errors": N, "warnings": M}
    }

DESIGN NOTES
------------
* Pure regex / line-based scan — NO real parser. False positives are accepted;
  false negatives are minimised. This is a fast pre-flight check, not a compiler.
* Workspace rules: if `{ws}/agents/pipeline/validator-rules.md` exists, the
  script logs that workspace rules were detected. A real loader for `ws-`
  prefixed rules is a TODO (extension point: see `load_workspace_rules`).
* All paths are normalised to POSIX-style in the JSON output for stable diffs.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from pathlib import Path
from typing import Dict, List, Optional, Tuple


# ---------------------------------------------------------------------------
# Workspace + wiki_root resolution
# ---------------------------------------------------------------------------

def find_wiki_json(start_dir: Path) -> Optional[Path]:
    """Walk up from start_dir looking for .claude/wiki.json. Return path or None."""
    cur = start_dir.resolve()
    while True:
        candidate = cur / ".claude" / "wiki.json"
        if candidate.is_file():
            return candidate
        if cur.parent == cur:
            return None
        cur = cur.parent


def load_global_wiki_root() -> Optional[str]:
    """Read ~/.claude/wiki-global.json#wiki_root if it exists."""
    home = Path(os.path.expanduser("~"))
    p = home / ".claude" / "wiki-global.json"
    if not p.is_file():
        return None
    try:
        data = json.loads(p.read_text(encoding="utf-8"))
        return data.get("wiki_root")
    except (json.JSONDecodeError, OSError):
        return None


def resolve_wiki_root(wiki_json_path: Path, raw_value) -> Optional[Path]:
    """
    Resolve wiki_root per agents/system-prompt.md#wiki_root-resolution-rule.
      * absolute path  -> use as-is
      * relative path  -> resolve relative to project_root (parent of .claude/)
      * null/empty     -> fallback to ~/.claude/wiki-global.json#wiki_root

    project_root = wiki_json_path.parent.parent because wiki_json_path is
    always <root>/.claude/wiki.json, so .parent = <root>/.claude/ and
    .parent.parent = <root>.
    """
    if raw_value:
        p = Path(raw_value)
        if p.is_absolute():
            return p.resolve()
        project_root = wiki_json_path.parent.parent
        return (project_root / p).resolve()
    fallback = load_global_wiki_root()
    if fallback:
        return Path(fallback).expanduser().resolve()
    return None


def resolve_workspace_context(
    file_path: Path,
    cli_workspace: Optional[str],
    cli_wiki_root: Optional[str],
) -> Tuple[Optional[str], Optional[Path], Optional[str]]:
    """
    Returns (workspace_name, wiki_root_path, domain).
    Falls back gracefully — None values mean "not resolved, skip those checks".
    """
    workspace = cli_workspace
    wiki_root = Path(cli_wiki_root).resolve() if cli_wiki_root else None
    domain: Optional[str] = None

    wiki_json = find_wiki_json(file_path.parent if file_path.is_file()
                               else file_path)
    if wiki_json:
        try:
            cfg = json.loads(wiki_json.read_text(encoding="utf-8"))
        except (json.JSONDecodeError, OSError):
            cfg = {}
        if not workspace:
            workspace = cfg.get("workspace")
        if not wiki_root:
            wiki_root = resolve_wiki_root(wiki_json, cfg.get("wiki_root"))
        domain = cfg.get("domain")
    return workspace, wiki_root, domain


# ---------------------------------------------------------------------------
# Knowledge loaders (best-effort — silent failure -> empty result)
# ---------------------------------------------------------------------------

MQTT_TYPE_TABLE_HEADER = re.compile(
    r"^\|\s*Type\s*\|", re.IGNORECASE | re.MULTILINE
)
TABLE_ROW_TYPE = re.compile(r"^\|\s*`?([a-zA-Z0-9_\-]+)`?\s*\|")
WORKFLOW_STATE_TOKEN = re.compile(r"`([A-Z][A-Z0-9_]{2,})`")


def load_mqtt_registered_types(ws_root: Path) -> List[str]:
    """Parse the 'Registered Types' table from mqtt-topic-contract.md."""
    p = ws_root / "platform" / "contracts" / "mqtt-topic-contract.md"
    if not p.is_file():
        return []
    text = p.read_text(encoding="utf-8")
    # Find the 'Registered Types' section then read table rows
    m = re.search(r"##\s*Registered Types\s*\n(.+?)(?:\n##|\Z)",
                  text, re.DOTALL | re.IGNORECASE)
    if not m:
        return []
    block = m.group(1)
    types: List[str] = []
    for line in block.splitlines():
        if line.strip().startswith("|") and "Type" not in line and "---" not in line:
            mt = TABLE_ROW_TYPE.match(line.strip())
            if mt:
                types.append(mt.group(1))
    # Dedup, preserve order
    seen = set()
    out = []
    for t in types:
        if t not in seen:
            seen.add(t)
            out.append(t)
    return out


def load_workflow_states(ws_root: Path, domain: Optional[str]) -> List[str]:
    """Read {ws}/domains/{domain}/workflow.md and extract UPPER_CASE state tokens."""
    if not domain:
        # Heuristic: pick the only domain dir if exactly one exists.
        ddir = ws_root / "domains"
        if ddir.is_dir():
            subs = [p for p in ddir.iterdir() if p.is_dir()]
            if len(subs) == 1:
                domain = subs[0].name
    if not domain:
        return []
    p = ws_root / "domains" / domain / "workflow.md"
    if not p.is_file():
        return []
    text = p.read_text(encoding="utf-8")
    states = WORKFLOW_STATE_TOKEN.findall(text)
    seen = set()
    out = []
    for s in states:
        if s not in seen:
            seen.add(s)
            out.append(s)
    return out


def load_workspace_rules(ws_root: Path) -> List[str]:
    """
    Detect workspace-level validator rules. Currently a stub: returns the path
    if the file exists so it can be reported. Full rule loader is a TODO —
    parsing additive `ws-` rules from a markdown table is non-trivial and
    out of scope for the initial Layer 1 pass.
    """
    p = ws_root / "agents" / "pipeline" / "validator-rules.md"
    return [str(p)] if p.is_file() else []


# ---------------------------------------------------------------------------
# Helpers shared by rules
# ---------------------------------------------------------------------------

def _strip_line_comment(line: str) -> str:
    """Best-effort: drop `// ...` tail in Java/JS-like files. Not aware of
    strings, so a `//` inside a literal will incorrectly truncate — accepted
    as a Layer 1 limitation."""
    idx = line.find("//")
    if idx >= 0:
        return line[:idx]
    return line


def _all_string_literals(line: str) -> List[str]:
    """Return content of "..." and '...' string literals on a line.
    Naive — does not handle escaped quotes within strings."""
    return re.findall(r'"([^"\\]*(?:\\.[^"\\]*)*)"', line) + \
           re.findall(r"'([^'\\]*(?:\\.[^'\\]*)*)'", line)


# ---------------------------------------------------------------------------
# Rule implementations
# Each rule fn signature:
#   fn(file_path: Path, lines: List[str], ctx: dict) -> List[Violation]
# ---------------------------------------------------------------------------

def _vio(rule: str, severity: str, file_path: Path, lineno: int,
         snippet: str, message: str) -> Dict:
    return {
        "rule": rule,
        "severity": severity,
        "file": file_path.as_posix(),
        "line": lineno,
        "snippet": snippet.strip()[:200],
        "message": message,
    }


# Pattern: a quoted dotted topic-like literal "foo.bar.baz" with at least one dot.
KAFKA_TOPIC_LITERAL = re.compile(
    r'"([a-z][a-z0-9_]*(?:\.[a-z0-9_]+){1,})"'
)
# Whitelist hints — if the line clearly reads from a config source, skip it.
KAFKA_CONFIG_READ_HINT = re.compile(
    r"(@Value|getProperty|getString|getConfig|env\.|System\.getenv|"
    r"@ConfigurationProperties|propertySource|ConfigMap|application\.yml)",
    re.IGNORECASE,
)


def rule_kafka_hardcoded_topics(file_path: Path, lines: List[str],
                                ctx: Dict) -> List[Dict]:
    """LIMITATION: regex-only — distinguishing 'Java package name' from
    'Kafka topic' is heuristic. We require lowercase + at least one dot, and
    skip lines that look like config reads."""
    out = []
    for i, raw in enumerate(lines, start=1):
        line = _strip_line_comment(raw)
        if KAFKA_CONFIG_READ_HINT.search(line):
            continue
        # Only flag near a kafka API verb to reduce noise
        if not re.search(r"(KafkaTemplate|@KafkaListener|topics\s*=|"
                         r"send\s*\(|subscribe\s*\(|produce\s*\(|consume\s*\()",
                         line, re.IGNORECASE):
            # also catch obvious assignments to a `topic` variable
            if not re.search(r"\btopic[A-Z_a-z]*\s*=\s*\"", line):
                continue
        for m in KAFKA_TOPIC_LITERAL.finditer(line):
            literal = m.group(1)
            # ignore obvious package-like or class-like literals (skip if no
            # second dot OR contains uppercase)
            if any(ch.isupper() for ch in literal):
                continue
            out.append(_vio(
                "kafka-no-hardcoded-topic", "error", file_path, i, raw,
                f"Hardcoded Kafka topic literal '{literal}'. "
                "Read from config (@Value / properties)."
            ))
    return out


COMMIT_CALL = re.compile(r"\b(commitSync|commitAsync)\s*\(")
PROCESS_HINT = re.compile(
    r"\b(process|handle|forEach|for\s*\(|while\s*\(|onMessage|"
    r"send\s*\(|publish\s*\()", re.IGNORECASE
)


def rule_kafka_offset_commit_position(file_path: Path, lines: List[str],
                                      ctx: Dict) -> List[Dict]:
    """Flag commitSync/commitAsync that appears textually BEFORE any apparent
    processing call within the same enclosing brace block.

    LIMITATION: brace-depth tracking is naive (no string/comment awareness)."""
    out = []
    depth = 0
    # Track per-block: list of (lineno, kind) where kind in {"commit","process"}
    block_stack: List[List[Tuple[int, str]]] = [[]]
    for i, raw in enumerate(lines, start=1):
        line = _strip_line_comment(raw)
        # Record events first (so we attribute them to current block)
        if COMMIT_CALL.search(line):
            block_stack[-1].append((i, "commit"))
        if PROCESS_HINT.search(line) and not COMMIT_CALL.search(line):
            block_stack[-1].append((i, "process"))
        # Then update depth
        opens = line.count("{")
        closes = line.count("}")
        for _ in range(opens):
            block_stack.append([])
            depth += 1
        for _ in range(closes):
            if block_stack:
                events = block_stack.pop()
                # Inspect: is there any 'commit' before the first 'process'?
                first_process = next((ln for ln, k in events if k == "process"),
                                     None)
                for ln, k in events:
                    if k == "commit" and (first_process is None or
                                          ln < first_process):
                        snippet = lines[ln - 1] if ln - 1 < len(lines) else ""
                        out.append(_vio(
                            "kafka-commit-before-process", "error",
                            file_path, ln, snippet,
                            "Offset commit appears before message processing "
                            "in the enclosing block — data-loss risk."
                        ))
            if depth > 0:
                depth -= 1
            if not block_stack:
                block_stack.append([])
    return out


def rule_kafka_dlq_present(file_path: Path, lines: List[str],
                           ctx: Dict) -> List[Dict]:
    """If file looks like a Kafka consumer, require any of: 'dlq', 'deadLetter',
    '.dlq.', or '.dlq' to appear somewhere. Whole-file check, single warning."""
    text = "\n".join(lines)
    is_consumer = bool(re.search(
        r"(@KafkaListener|KafkaConsumer|ConsumerRecord|@StreamListener)",
        text)) or bool(re.search(r"\bpoll\s*\(", text))
    if not is_consumer:
        return []
    if re.search(r"(?i)(dlq|deadLetter|\.dlq\.|\.dlq\b)", text):
        return []
    return [_vio(
        "kafka-dlq-required", "error", file_path, 1, lines[0] if lines else "",
        "Kafka consumer detected but no DLQ branch found "
        "(no reference to 'dlq' / 'deadLetter')."
    )]


PER_MESSAGE_LOOP = re.compile(
    r"\bfor\s*\(\s*\w[\w<>,\s]*\s+\w+\s*:\s*(\w*[Mm]essages?\w*)\s*\)"
)
BATCH_HINT = re.compile(r"(batch|max\.poll\.records|MAX_POLL_RECORDS|"
                        r"setBatchListener|@KafkaListener\([^)]*containerFactory)",
                        re.IGNORECASE)


def rule_kafka_batch_processing(file_path: Path, lines: List[str],
                                ctx: Dict) -> List[Dict]:
    """Heuristic: a `for (X x : messages)` style loop without a batch hint
    nearby in file. Warn-level — false positives common."""
    text = "\n".join(lines)
    if not BATCH_HINT.search(text):
        return []
    out = []
    for i, raw in enumerate(lines, start=1):
        m = PER_MESSAGE_LOOP.search(raw)
        if m:
            out.append(_vio(
                "kafka-batch-processing", "warn", file_path, i, raw,
                f"Per-message loop over '{m.group(1)}' in batch-mode "
                "consumer — verify batch processing intent."
            ))
    return out


MQTT_INLINE_TOPIC = re.compile(
    r'("topic/[^"]*"\s*\+|\+\s*"topic/[^"]*"|'
    r'String\.format\s*\(\s*"topic/[^"]*"|'
    r'`topic/[^`]*\$\{|f"topic/[^"]*\{)'
)
MQTT_PUBLISH_CALL = re.compile(
    r"\b(publish|sendToTopic|mqttClient\.publish)\s*\("
)
MQTT_TOPIC_HELPER_HINT = re.compile(
    r"(buildTopic|topicFor|MqttTopic\.|TopicFormatter|topicHelper)",
    re.IGNORECASE
)


def rule_mqtt_no_inline_construction(file_path: Path, lines: List[str],
                                     ctx: Dict) -> List[Dict]:
    """Flag concat/format/template-string building of a literal starting with
    'topic/'. Skip if a known helper is referenced on the same line."""
    out = []
    for i, raw in enumerate(lines, start=1):
        line = _strip_line_comment(raw)
        if MQTT_TOPIC_HELPER_HINT.search(line):
            continue
        if MQTT_INLINE_TOPIC.search(line):
            out.append(_vio(
                "mqtt-no-inline-topic", "error", file_path, i, raw,
                "MQTT topic appears to be built inline. "
                "Use the contract format helper instead."
            ))
    return out


MQTT_TOPIC_LITERAL = re.compile(r'"topic/[^"]*/up/([a-zA-Z0-9_\-]+)"')


def rule_mqtt_only_registered_types(file_path: Path, lines: List[str],
                                    ctx: Dict) -> List[Dict]:
    """For any literal matching topic/.../up/<type>, ensure <type> is registered."""
    registered = ctx.get("mqtt_types") or []
    if not registered:
        return []
    out = []
    for i, raw in enumerate(lines, start=1):
        for m in MQTT_TOPIC_LITERAL.finditer(raw):
            t = m.group(1)
            # skip placeholders like {type} or ${...}
            if t.startswith("{") or t.startswith("$"):
                continue
            if t not in registered:
                out.append(_vio(
                    "mqtt-unregistered-type", "error", file_path, i, raw,
                    f"MQTT type '{t}' is not in the registered types list "
                    f"({', '.join(registered)})."
                ))
    return out


STATE_TOKEN_RE = re.compile(r'"([A-Z][A-Z0-9_]{2,})"')
# Likely-not-a-state common literals to ignore (avoid noise)
STATE_IGNORE = {
    "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS",
    "TRUE", "FALSE", "NULL", "OK", "ERROR", "WARN", "INFO", "DEBUG",
    "UTF_8", "UTF8", "ISO_8859_1", "US_ASCII",
    "SUCCESS", "FAILURE",
}


def rule_domain_no_new_states(file_path: Path, lines: List[str],
                              ctx: Dict) -> List[Dict]:
    """Flag UPPER_CASE_SNAKE string literals that are NOT in the workflow doc.

    LIMITATION: any UPPER_CASE token that is actually e.g. a header name or
    enum unrelated to the workflow will be flagged. Mitigated by ignore list
    and by requiring the token to look state-like (>=2 chars, has a digit
    or underscore OR ends in -ED/-ING/-AL)."""
    states = ctx.get("workflow_states") or []
    if not states:
        return []
    state_set = set(states)
    out = []
    for i, raw in enumerate(lines, start=1):
        for m in STATE_TOKEN_RE.finditer(raw):
            tok = m.group(1)
            if tok in STATE_IGNORE or tok in state_set:
                continue
            # Heuristic: state-like = has underscore OR ends with common suffixes
            looks_state = ("_" in tok) or tok.endswith(
                ("ED", "ING", "AL", "ANT", "OUS"))
            if not looks_state:
                continue
            out.append(_vio(
                "domain-unknown-state", "error", file_path, i, raw,
                f"State literal '{tok}' is not in workflow.md "
                f"(allowed: {', '.join(states)})."
            ))
    return out


HARDCODED_CONFIG = re.compile(
    r"\b(batch[_]?[Ss]ize|timeout(?:Ms)?|concurrency|maxPoll(?:Records)?|"
    r"poll[_]?[Tt]imeout|retr(?:y|ies)|backoff(?:Ms)?)\s*=\s*(\d+)\b"
)


def rule_no_hardcoded_config(file_path: Path, lines: List[str],
                             ctx: Dict) -> List[Dict]:
    """Flag literal numeric assignments to config-like identifiers."""
    out = []
    for i, raw in enumerate(lines, start=1):
        line = _strip_line_comment(raw)
        for m in HARDCODED_CONFIG.finditer(line):
            name, value = m.group(1), m.group(2)
            out.append(_vio(
                "no-hardcoded-config", "warn", file_path, i, raw,
                f"Hardcoded value '{value}' assigned to '{name}'. "
                "Read from configuration."
            ))
    return out


AUTOWIRED_FIELD = re.compile(
    r"^\s*@Autowired\b(?!\s*\(?\s*\n*\s*[A-Za-z_]\w*\s*\()"
)


def rule_constructor_injection(file_path: Path, lines: List[str],
                               ctx: Dict) -> List[Dict]:
    """Flag @Autowired on a field declaration. Heuristic: @Autowired on a line
    where the NEXT non-blank line declares a field (no `(` on it) and is not a
    constructor signature."""
    out = []
    n = len(lines)
    for i, raw in enumerate(lines):
        if "@Autowired" not in raw:
            continue
        # find next non-blank line
        j = i + 1
        while j < n and not lines[j].strip():
            j += 1
        if j >= n:
            continue
        nxt = lines[j].strip()
        # constructor / method has '(' on its declaration line.
        # field declaration: ends with ';' and has no '('
        is_method_or_ctor = "(" in nxt
        if not is_method_or_ctor and nxt.endswith(";"):
            out.append(_vio(
                "constructor-injection", "error", file_path, i + 1, raw,
                "@Autowired on a field — use constructor injection instead."
            ))
    return out


# ---------------------------------------------------------------------------
# Driver
# ---------------------------------------------------------------------------

ALL_RULES = [
    rule_kafka_hardcoded_topics,
    rule_kafka_offset_commit_position,
    rule_kafka_dlq_present,
    rule_kafka_batch_processing,
    rule_mqtt_no_inline_construction,
    rule_mqtt_only_registered_types,
    rule_domain_no_new_states,
    rule_no_hardcoded_config,
    rule_constructor_injection,
]

# File extensions we attempt to lint. Other files: emit no violations
# (script does not crash — caller may pass any path, including .md).
LINTABLE_EXT = {".java", ".kt", ".kts", ".scala", ".groovy",
                ".js", ".ts", ".jsx", ".tsx",
                ".py", ".go", ".rb", ".cs"}


def run(file_path: Path, ws_root: Optional[Path], domain: Optional[str],
        ws_name: Optional[str]) -> Tuple[List[Dict], Dict]:
    violations: List[Dict] = []

    # Skip non-code files silently — design choice from spec
    if file_path.suffix.lower() not in LINTABLE_EXT:
        return violations, {
            "skipped": True,
            "reason": f"unsupported extension '{file_path.suffix}'",
            "workspace": ws_name,
        }

    text = file_path.read_text(encoding="utf-8", errors="replace")
    lines = text.splitlines()

    ctx: Dict = {
        "workspace": ws_name,
        "domain": domain,
        "mqtt_types": [],
        "workflow_states": [],
        "workspace_rules_file": [],
    }
    if ws_root and ws_root.is_dir() and ws_name:
        ws_dir = ws_root / "workspaces" / ws_name
        if ws_dir.is_dir():
            ctx["mqtt_types"] = load_mqtt_registered_types(ws_dir)
            ctx["workflow_states"] = load_workflow_states(ws_dir, domain)
            ctx["workspace_rules_file"] = load_workspace_rules(ws_dir)

    for rule_fn in ALL_RULES:
        try:
            violations.extend(rule_fn(file_path, lines, ctx))
        except Exception as e:  # never let a buggy rule kill the run
            violations.append(_vio(
                f"internal-{rule_fn.__name__}", "warn", file_path, 0, "",
                f"Rule crashed: {e!r}"
            ))

    # TODO(workspace-rules): if ctx["workspace_rules_file"] is non-empty,
    # load and execute additive `ws-` rules from that markdown file.
    # Currently we only surface the file path so users know rules exist.

    return violations, ctx


def main(argv: List[str]) -> int:
    parser = argparse.ArgumentParser(
        prog="validate.py",
        description="Layer 1 rule-based validator — see "
                    "agents/pipeline/validator-rules.md.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("--file", required=True, help="Code file to validate.")
    parser.add_argument("--workspace", default=None,
                        help="Override workspace name (else from .claude/wiki.json).")
    parser.add_argument("--wiki-root", default=None,
                        help="Override wiki root path (else from .claude/wiki.json).")
    parser.add_argument("--pretty", action="store_true",
                        help="Pretty-print JSON output.")
    args = parser.parse_args(argv)

    file_path = Path(args.file).resolve()
    if not file_path.exists():
        print(json.dumps({
            "violations": [],
            "summary": {"errors": 1, "warnings": 0},
            "error": f"file not found: {file_path}",
        }), file=sys.stdout)
        return 2

    ws_name, wiki_root, domain = resolve_workspace_context(
        file_path, args.workspace, args.wiki_root
    )

    violations, ctx = run(file_path, wiki_root, domain, ws_name)

    errors = sum(1 for v in violations if v["severity"] == "error")
    warnings = sum(1 for v in violations if v["severity"] == "warn")

    out = {
        "violations": violations,
        "summary": {"errors": errors, "warnings": warnings},
        "context": {
            "workspace": ws_name,
            "wiki_root": wiki_root.as_posix() if wiki_root else None,
            "domain": domain,
            "mqtt_registered_types": ctx.get("mqtt_types", []),
            "workflow_states": ctx.get("workflow_states", []),
            "workspace_rules_file": ctx.get("workspace_rules_file", []),
            "skipped": ctx.get("skipped", False),
        },
    }
    if args.pretty:
        print(json.dumps(out, indent=2, ensure_ascii=False))
    else:
        print(json.dumps(out, ensure_ascii=False))

    return 1 if errors > 0 else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
