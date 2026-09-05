//! yaml.rs — gopkg.in/yaml.v3-compatible YAML layer for zbrain frontmatter.
//!
//! The trust contract requires byte-identical canonical files across the
//! Go→Rust cutover, so marshaling is a hand-rolled emitter reproducing
//! yaml.v3's exact output for the claim/evidence schemas (4-space root indent,
//! 2-space steps inside block sequence items, yaml.v3 scalar quoting). The
//! libyaml-based serde_yml emitter cannot reproduce that style, so it is used
//! only for unmarshal, where output style is irrelevant.

use std::fmt::Write as _;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum YamlStyle {
    Plain,
    Single,
    Double,
    Literal,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Yaml {
    Scalar { value: String, style: YamlStyle },
    Seq(Vec<Yaml>),
    Map(Vec<(String, Yaml)>),
}

impl Yaml {
    pub fn scalar(value: &str) -> Yaml {
        let style = choose_style(value);
        Yaml::Scalar {
            value: value.to_string(),
            style,
        }
    }

    pub fn seq(items: Vec<Yaml>) -> Yaml {
        Yaml::Seq(items)
    }

    pub fn map(entries: Vec<(&str, Yaml)>) -> Yaml {
        Yaml::Map(
            entries
                .into_iter()
                .map(|(key, value)| (key.to_string(), value))
                .collect(),
        )
    }

    pub fn string_list(values: &[String]) -> Yaml {
        Yaml::Seq(values.iter().map(|v| Yaml::scalar(v)).collect())
    }
}

pub fn emit(root: &Yaml) -> Vec<u8> {
    let mut out = String::new();
    match root {
        Yaml::Map(entries) => write_map(&mut out, entries, 0, false),
        Yaml::Seq(items) => write_seq(&mut out, items, 0),
        Yaml::Scalar { value, style } => {
            match style {
                YamlStyle::Literal => write_literal(&mut out, value, 4),
                _ => {
                    out.push_str(&render_inline(value, *style));
                    out.push('\n');
                }
            }
        }
    }
    out.into_bytes()
}

/// Render a claim/evidence markdown document: `---\n<frontmatter>---\n<body>`.
pub fn emit_markdown_document(frontmatter: &Yaml, body: &str) -> Vec<u8> {
    let mut out = String::from("---\n");
    out.push_str(&String::from_utf8_lossy(&emit(frontmatter)));
    out.push_str("---\n");
    out.push_str(body);
    out.into_bytes()
}

fn write_map(out: &mut String, entries: &[(String, Yaml)], indent: usize, in_seq: bool) {
    let step = if in_seq { 2 } else { 4 };
    for (key, value) in entries {
        match value {
            Yaml::Scalar { value, style } => {
                out.push_str(&" ".repeat(indent));
                out.push_str(key);
                out.push_str(": ");
                match style {
                    YamlStyle::Literal => write_literal(out, value, indent + step),
                    _ => {
                        out.push_str(&render_inline(value, *style));
                        out.push('\n');
                    }
                }
            }
            Yaml::Seq(items) => {
                out.push_str(&" ".repeat(indent));
                out.push_str(key);
                out.push(':');
                if items.is_empty() {
                    out.push_str(" []\n");
                    continue;
                }
                out.push('\n');
                write_seq(out, items, indent + step);
            }
            Yaml::Map(sub) => {
                out.push_str(&" ".repeat(indent));
                out.push_str(key);
                out.push(':');
                if sub.is_empty() {
                    out.push_str(" {}\n");
                    continue;
                }
                out.push('\n');
                write_map(out, sub, indent + step, in_seq);
            }
        }
    }
}

fn write_seq(out: &mut String, items: &[Yaml], indent: usize) {
    for item in items {
        match item {
            Yaml::Scalar { value, style } => {
                out.push_str(&" ".repeat(indent));
                out.push_str("- ");
                match style {
                    YamlStyle::Literal => {
                        // Literal scalars inside scalar sequence items do not
                        // occur in the claim/evidence schema; fall back to the
                        // inline form the way yaml.v3 does for flow context.
                        out.push_str(&render_double_quoted(value));
                        out.push('\n');
                    }
                    _ => {
                        out.push_str(&render_inline(value, *style));
                        out.push('\n');
                    }
                }
            }
            Yaml::Seq(nested) => {
                out.push_str(&" ".repeat(indent));
                if nested.is_empty() {
                    out.push_str("- []\n");
                    continue;
                }
                out.push_str("-\n");
                write_seq(out, nested, indent + 2);
            }
            Yaml::Map(entries) => {
                if entries.is_empty() {
                    out.push_str(&" ".repeat(indent));
                    out.push_str("- {}\n");
                    continue;
                }
                write_map_item(out, entries, indent);
            }
        }
    }
}

/// A mapping that is the value of a block sequence item: the first field is
/// compacted onto the `- ` line, the remaining fields align at dash+2, and
/// everything below steps by 2 (yaml.v3 behavior for sequences).
fn write_map_item(out: &mut String, entries: &[(String, Yaml)], dash_indent: usize) {
    let field_indent = dash_indent + 2;
    for (index, (key, value)) in entries.iter().enumerate() {
        let compact = index == 0;
        let prefix = if compact {
            format!("{}- ", " ".repeat(dash_indent))
        } else {
            " ".repeat(field_indent)
        };
        match value {
            Yaml::Scalar { value, style } => {
                out.push_str(&prefix);
                out.push_str(key);
                out.push_str(": ");
                match style {
                    YamlStyle::Literal => write_literal(out, value, field_indent),
                    _ => {
                        out.push_str(&render_inline(value, *style));
                        out.push('\n');
                    }
                }
            }
            Yaml::Seq(items) => {
                out.push_str(&prefix);
                out.push_str(key);
                out.push(':');
                if items.is_empty() {
                    out.push_str(" []\n");
                    continue;
                }
                out.push('\n');
                write_seq(out, items, field_indent + 2);
            }
            Yaml::Map(sub) => {
                out.push_str(&prefix);
                out.push_str(key);
                out.push(':');
                if sub.is_empty() {
                    out.push_str(" {}\n");
                    continue;
                }
                out.push('\n');
                write_map(out, sub, field_indent + 2, true);
            }
        }
    }
}

fn render_inline(value: &str, style: YamlStyle) -> String {
    match style {
        YamlStyle::Plain => value.to_string(),
        YamlStyle::Single => format!("'{}'", value.replace('\'', "''")),
        YamlStyle::Double => render_double_quoted(value),
        YamlStyle::Literal => String::new(),
    }
}

fn write_literal(out: &mut String, value: &str, content_indent: usize) {
    let trailing_newlines = value.len() - value.trim_end_matches('\n').len();
    let content = &value[..value.len() - trailing_newlines];
    let chomp = match trailing_newlines {
        0 => "-",
        1 => "",
        _ => "+",
    };
    let lines: Vec<&str> = content.split('\n').collect();
    let mut header = String::from("|");
    if lines.first().is_some_and(|line| line.is_empty() || line.starts_with(' ')) {
        let _ = write!(header, "{}", content_indent);
    }
    header.push_str(chomp);
    out.push_str(&header);
    out.push('\n');
    for line in &lines {
        if line.is_empty() {
            out.push('\n');
        } else {
            out.push_str(&" ".repeat(content_indent));
            out.push_str(line);
            out.push('\n');
        }
    }
    for _ in 1..trailing_newlines {
        out.push('\n');
    }
}

fn render_double_quoted(value: &str) -> String {
    let mut out = String::from("\"");
    for c in value.chars() {
        match c {
            '\\' => out.push_str("\\\\"),
            '"' => out.push_str("\\\""),
            '\x00' => out.push_str("\\0"),
            '\x07' => out.push_str("\\a"),
            '\x08' => out.push_str("\\b"),
            '\t' => out.push_str("\\t"),
            '\n' => out.push_str("\\n"),
            '\x0b' => out.push_str("\\v"),
            '\x0c' => out.push_str("\\f"),
            '\r' => out.push_str("\\r"),
            '\x1b' => out.push_str("\\e"),
            '\u{85}' => out.push_str("\\N"),
            '\u{a0}' => out.push_str("\\_"),
            '\u{2028}' => out.push_str("\\L"),
            '\u{2029}' => out.push_str("\\P"),
            c if (c as u32) < 0x20 || (c as u32) == 0x7f => {
                let _ = write!(out, "\\x{:02x}", c as u32);
            }
            c if (0x80..=0x9f).contains(&(c as u32)) => {
                let _ = write!(out, "\\u{:04x}", c as u32);
            }
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

/// Scalar style selection mirroring gopkg.in/yaml.v3's encoder + emitter
/// analysis, verified against the committed scalar corpus fixture.
pub fn choose_style(value: &str) -> YamlStyle {
    if value.is_empty() {
        return YamlStyle::Double;
    }
    if value.contains('\n') {
        if literal_allowed(value) {
            return YamlStyle::Literal;
        }
        return YamlStyle::Double;
    }
    if resolves_as_non_string(value)
        || value.contains('\t')
        || value.contains('\r')
        || value
            .chars()
            .any(|c| (c as u32) < 0x20 || (c as u32) == 0x7f || (0x80..=0x9f).contains(&(c as u32)))
    {
        return YamlStyle::Double;
    }
    if plain_allowed(value) {
        return YamlStyle::Plain;
    }
    YamlStyle::Single
}

fn literal_allowed(value: &str) -> bool {
    // yaml.v3 falls back to double-quoted when any line carries trailing
    // spaces or contains characters a block scalar cannot represent.
    !value.contains('\r')
        && !value
            .chars()
            .any(|c| (c as u32) < 0x20 && c != '\n' && c != '\t' || (c as u32) == 0x7f)
        && !value.lines().any(|line| line.ends_with(' '))
}

const NULL_WORDS: [&str; 5] = ["", "~", "null", "Null", "NULL"];
const BOOL_WORDS: [&str; 6] = ["true", "True", "TRUE", "false", "False", "FALSE"];
const OLD_BOOL_WORDS: [&str; 16] = [
    "y", "Y", "yes", "Yes", "YES", "n", "N", "no", "No", "NO", "on", "On", "ON", "off", "Off",
    "OFF",
];
const INF_WORDS: [&str; 3] = [".inf", ".Inf", ".INF"];
const NAN_WORDS: [&str; 3] = [".nan", ".NaN", ".NAN"];

/// Whether the plain form of `value` would resolve to a non-string tag under
/// yaml.v3's resolver (null, bool, old bool, number, sexagesimal, timestamp).
fn resolves_as_non_string(value: &str) -> bool {
    if NULL_WORDS.contains(&value) || BOOL_WORDS.contains(&value) {
        return true;
    }
    if OLD_BOOL_WORDS.contains(&value) {
        return true;
    }
    if value == "<<" {
        return false;
    }
    let unsigned_inf = value.trim_start_matches(['+', '-']);
    if INF_WORDS.contains(&unsigned_inf) || NAN_WORDS.contains(&value) {
        return true;
    }
    match value.as_bytes().first() {
        Some(b) if b.is_ascii_digit() || *b == b'-' || *b == b'+' || *b == b'.' => {}
        _ => return false,
    }
    let plain: String = value.chars().filter(|c| *c != '_').collect();
    let body = plain.trim_start_matches(['+', '-']);
    let negative = plain.starts_with('-');
    let _ = negative;
    // Integer forms accepted by Go's strconv.ParseInt(plain, 0, 64).
    if parse_go_int(body).is_some() {
        return true;
    }
    if parse_go_float(&plain).is_some() {
        return true;
    }
    if is_base60(&plain) {
        return true;
    }
    is_timestamp(value)
}

fn parse_go_int(body: &str) -> Option<i64> {
    let (digits, radix) = if let Some(rest) = body.strip_prefix("0x").or_else(|| body.strip_prefix("0X")) {
        (rest, 16)
    } else if let Some(rest) = body.strip_prefix("0b").or_else(|| body.strip_prefix("0B")) {
        (rest, 2)
    } else if let Some(rest) = body.strip_prefix("0o").or_else(|| body.strip_prefix("0O")) {
        (rest, 8)
    } else if body.len() > 1 && body.starts_with('0') {
        (&body[1..], 8)
    } else {
        (body, 10)
    };
    if digits.is_empty() || digits.starts_with(['+', '-']) {
        return None;
    }
    i64::from_str_radix(digits, radix).ok()
}

fn parse_go_float(plain: &str) -> Option<f64> {
    let body = plain.trim_start_matches(['+', '-']);
    if body.is_empty() {
        return None;
    }
    if !body.chars().any(|c| c.is_ascii_digit()) {
        return None;
    }
    if body.chars().filter(|c| *c == '.').count() > 1 {
        return None;
    }
    if !body
        .chars()
        .all(|c| c.is_ascii_digit() || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-')
    {
        return None;
    }
    plain.parse::<f64>().ok()
}

fn is_base60(plain: &str) -> bool {
    let body = plain.trim_start_matches(['+', '-']);
    let Some((head, tail)) = body.split_once(':') else {
        return false;
    };
    if head.is_empty() || !head.chars().all(|c| c.is_ascii_digit()) {
        return false;
    }
    let mut rest = tail;
    let mut groups = 0;
    loop {
        let group = match rest.split_once(':') {
            Some((group, remainder)) => {
                rest = remainder;
                group
            }
            None => {
                let last = rest.split_once('.').map(|(g, _)| g).unwrap_or(rest);
                rest = "";
                last
            }
        };
        if group.is_empty() || group.len() > 2 || !group.chars().all(|c| c.is_ascii_digit()) {
            return false;
        }
        if groups < 2 && group.parse::<u32>().map(|v| v > 59).unwrap_or(true) {
            return false;
        }
        groups += 1;
        if rest.is_empty() {
            break;
        }
    }
    if let Some((_, frac)) = body.split_once('.') {
        if frac.is_empty() || !frac.chars().all(|c| c.is_ascii_digit()) {
            return false;
        }
    }
    groups >= 1
}

/// yaml.v3 allowedTimestampFormats: RFC3339 with short date fields (upper or
/// lower-case "t"), space-separated without zone, and date only.
fn is_timestamp(value: &str) -> bool {
    if let Some((date, rest)) = value.split_once(['T', 't']) {
        if !is_ymd(date) {
            return false;
        }
        let (time, zone) = strip_rfc3339_zone(rest);
        return zone && is_hms(time);
    }
    if let Some((date, time)) = value.split_once(' ') {
        return is_ymd(date) && is_hms(time);
    }
    is_ymd(value)
}

/// RFC3339-style zone suffix: `Z`/`z` or `±hh:mm`.
fn strip_rfc3339_zone(rest: &str) -> (&str, bool) {
    if rest.ends_with('Z') || rest.ends_with('z') {
        return (&rest[..rest.len() - 1], true);
    }
    if rest.len() >= 6 {
        let (time, zone) = rest.split_at(rest.len() - 6);
        let digits = |s: &str| !s.is_empty() && s.chars().all(|c| c.is_ascii_digit());
        if (zone.starts_with('+') || zone.starts_with('-'))
            && digits(&zone[1..3])
            && zone.as_bytes()[3] == b':'
            && digits(&zone[4..])
        {
            return (time, true);
        }
    }
    (rest, false)
}

fn is_ymd(date: &str) -> bool {
    let parts: Vec<&str> = date.split('-').collect();
    if parts.len() != 3 {
        return false;
    }
    let year = parts[0];
    let month = parts[1];
    let day = parts[2];
    if year.len() != 4 || !year.chars().all(|c| c.is_ascii_digit()) {
        return false;
    }
    if month.is_empty() || month.len() > 2 || day.is_empty() || day.len() > 2 {
        return false;
    }
    if !month.chars().all(|c| c.is_ascii_digit()) || !day.chars().all(|c| c.is_ascii_digit()) {
        return false;
    }
    let year: i32 = year.parse().unwrap_or(-1);
    let month: u32 = month.parse().unwrap_or(0);
    let day: u32 = day.parse().unwrap_or(0);
    chrono::NaiveDate::from_ymd_opt(year, month, day).is_some()
}

fn is_hms(time: &str) -> bool {
    let (time, _) = time.split_once('.').unwrap_or((time, ""));
    let parts: Vec<&str> = time.split(':').collect();
    if parts.len() != 3 {
        return false;
    }
    for (index, part) in parts.iter().enumerate() {
        if part.is_empty() || part.len() > 2 || !part.chars().all(|c| c.is_ascii_digit()) {
            return false;
        }
        let value: u32 = part.parse().unwrap_or(u32::MAX);
        let limit = match index {
            0 => 23,
            _ => 59,
        };
        if value > limit {
            return false;
        }
    }
    true
}

fn plain_allowed(value: &str) -> bool {
    let mut chars = value.chars();
    let Some(first) = chars.next() else {
        return false;
    };
    if first == ' ' || value.ends_with(' ') {
        return false;
    }
    if matches!(
        first,
        '#' | ',' | '[' | ']' | '{' | '}' | '&' | '*' | '!' | '|' | '>' | '\'' | '"' | '%' | '@' | '`'
    ) {
        return false;
    }
    if matches!(first, '-' | '?' | ':') {
        let second = value[1..].chars().next();
        if second.is_none() || second == Some(' ') {
            return false;
        }
    }
    if value.starts_with("---") || value.starts_with("...") {
        return false;
    }
    if value.contains(": ") || value.contains(" #") || value.ends_with(':') {
        return false;
    }
    true
}

#[cfg(test)]
mod tests {
    use super::*;

    fn render(value: &str) -> String {
        String::from_utf8(emit(&Yaml::map(vec![("v", Yaml::scalar(value))]))).unwrap()
    }

    #[test]
    fn scalar_styles_match_yaml_v3() {
        let corpus = include_str!(concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/tests/fixtures/yaml_scalar_corpus.txt"
        ));
        for case in corpus.lines() {
            let Some((input, expected)) = case.split_once(' ') else {
                panic!("corpus line malformed");
            };
            let input = decode_hex(input);
            let expected = decode_hex(expected);
            assert_eq!(render(&input), expected, "input {input:?}");
        }
    }

    fn decode_hex(value: &str) -> String {
        String::from_utf8(
            (0..value.len())
                .step_by(2)
                .map(|i| u8::from_str_radix(&value[i..i + 2], 16).unwrap())
                .collect(),
        )
        .unwrap()
    }

    #[test]
    fn indents_like_yaml_v3() {
        let claim = Yaml::map(vec![
            ("type", Yaml::scalar("zbrain.claim")),
            ("tags", Yaml::string_list(&["memory".into(), "trust".into()])),
            (
                "sources",
                Yaml::seq(vec![Yaml::map(vec![
                    ("id", Yaml::scalar("evd_0123456789abcdef0123456789abcdef")),
                    (
                        "spans",
                        Yaml::seq(vec![Yaml::map(vec![
                            ("evidence_id", Yaml::scalar("evd_0123456789abcdef0123456789abcdef")),
                            ("start_line", Yaml::Scalar { value: "2".into(), style: YamlStyle::Plain }),
                            ("digest", Yaml::scalar("sha256:span-v1:abc")),
                        ])]),
                    ),
                ])]),
            ),
            (
                "zbrain",
                Yaml::map(vec![
                    ("profile", Yaml::scalar("zbrain.trusted-memory/v1")),
                    (
                        "transitions",
                        Yaml::seq(vec![Yaml::map(vec![
                            ("kind", Yaml::scalar("approve")),
                            (
                                "related_claim_ids",
                                Yaml::string_list(&["clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb".into()]),
                            ),
                        ])]),
                    ),
                ]),
            ),
        ]);
        let out = String::from_utf8(emit(&claim)).unwrap();
        assert_eq!(
            out,
            "type: zbrain.claim\n\
             tags:\n    - memory\n    - trust\n\
             sources:\n    - id: evd_0123456789abcdef0123456789abcdef\n      spans:\n        - evidence_id: evd_0123456789abcdef0123456789abcdef\n          start_line: 2\n          digest: sha256:span-v1:abc\n\
             zbrain:\n    profile: zbrain.trusted-memory/v1\n    transitions:\n        - kind: approve\n          related_claim_ids:\n            - clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n"
        );
    }
}
