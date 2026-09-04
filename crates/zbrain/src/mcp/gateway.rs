//! gateway.rs — W2.T1 of phase m5-mcp-gateway: the real tool/resource
//! registry behind [`ToolRegistry`]. Ports `internal/mcp/tools.go`
//! (evidence_capture) and `internal/mcp/resources.go` (claim + evidence read
//! resources) onto the m2 stores, mirroring `newServer`'s registration
//! structure so W2.T2 extends by adding registrations. The remaining tools
//! (memory_ask, memory_status, evidence_check, claim_lifecycle, campaign_*,
//! doctor) are gated on m4 and intentionally absent.

use std::path::Path;
use std::time::Instant;

use serde_json::Value;

use crate::claims::{
    Claim, ClaimSource, ClaimStore, ClaimTransition, ClaimTransitionAuthorization, Contradiction,
    EvidenceSpan,
};
use crate::clock::Clock;
use crate::evidence::{Evidence, EvidenceStore};
use crate::mcp::protocol::{
    CallToolResult, ContentBlock, McpError, OrderedJson, ReadResourceResult, ToolEntry,
};
use crate::mcp::server::{ToolRegistry, MCP_MAX_INPUT_BYTES};
use crate::mcp::transport::SafeStderr;
use crate::paths::Paths;

/// Sized clock reference so a `Box<dyn Clock>` satisfies the store's
/// `&impl Clock` bound without touching the store signatures.
struct SharedClock<'a>(&'a dyn Clock);

impl Clock for SharedClock<'_> {
    fn now(&self) -> chrono::DateTime<chrono::Utc> {
        self.0.now()
    }
}

/// Real gateway registry: tool/resource handlers wired to the canonical
/// claims and evidence stores. Mirrors `internal/mcp.Options` (paths, clock)
/// with the registration done in the [`ToolRegistry`] methods.
pub struct ZbrainRegistry {
    pub paths: Paths,
    pub clock: Box<dyn Clock>,
    pub stderr: SafeStderr,
}

impl ZbrainRegistry {
    pub fn new(paths: Paths, clock: Box<dyn Clock>) -> Self {
        Self { paths, clock, stderr: SafeStderr::default() }
    }

    /// Ports `resolveWorkspace`: explicit name or the current workspace,
    /// always validated through the boundary.
    fn resolve_workspace(&self, name: &str) -> Result<String, String> {
        let workspace = if name.is_empty() {
            crate::workspace::resolve_current_workspace(&self.paths)
                .map_err(|error| error.to_string())?
                .workspace
        } else {
            name.to_string()
        };
        crate::boundary::validate_workspace(&self.paths, &workspace)
            .map_err(|error| error.to_string())?;
        Ok(workspace)
    }

    /// Ports the evidence_capture handler body: guards, workspace
    /// resolution, then `EvidenceStore.AddFile`.
    fn run_evidence_capture(
        &self,
        arguments: Option<&Value>,
    ) -> Result<CallToolResult, McpError> {
        if let Err(message) = validate_capture_arguments(arguments) {
            return Ok(is_error_result(&format!("validating \"arguments\": {message}")));
        }
        // runMCPTool's bounds check over the marshaled input.
        if let Some(arguments) = arguments {
            let encoded = arguments.to_string();
            if encoded.len() > MCP_MAX_INPUT_BYTES {
                return Err(McpError::invalid_params("input exceeds 1MB limit"));
            }
        }
        let file = arguments
            .and_then(|arguments| arguments.get("file"))
            .and_then(Value::as_str)
            .unwrap_or_default();
        let origin = arguments
            .and_then(|arguments| arguments.get("origin"))
            .and_then(Value::as_str)
            .unwrap_or_default();
        let media_type = arguments
            .and_then(|arguments| arguments.get("media_type"))
            .and_then(Value::as_str)
            .unwrap_or_default();
        if file.trim().is_empty() {
            return Ok(is_error_result("file is required"));
        }
        if origin.trim().is_empty() {
            return Ok(is_error_result("origin is required"));
        }
        let workspace = match self.resolve_workspace(
            arguments
                .and_then(|arguments| arguments.get("workspace"))
                .and_then(Value::as_str)
                .unwrap_or_default(),
        ) {
            Ok(workspace) => workspace,
            Err(message) => return Ok(is_error_result(&message)),
        };
        let evidence = match EvidenceStore::new(self.paths.clone()).add_file(
            &workspace,
            Path::new(file),
            origin,
            media_type,
            &SharedClock(&*self.clock),
        ) {
            Ok(evidence) => evidence,
            Err(error) => return Ok(is_error_result(&error.to_string())),
        };
        let out = capture_output_json(&workspace, &evidence);
        Ok(CallToolResult {
            content: vec![ContentBlock { r#type: "text", text: out.pretty() }],
            structured_content: Some(out),
            ..Default::default()
        })
    }

    /// Ports `readClaimResource`: canonical claim JSON text content.
    fn read_claim_resource(
        &self,
        uri: &str,
        workspace: &str,
        id: &str,
    ) -> Option<ReadResourceResult> {
        let claim = ClaimStore::new(self.paths.clone()).read(workspace, id).ok()?;
        Some(ReadResourceResult::text(
            uri,
            "application/json",
            claim_resource_json(&claim).pretty(),
        ))
    }

    /// Ports `readEvidenceResource`: fenced envelope with the raw snapshot
    /// bytes nested under `untrusted_evidence` only.
    fn read_evidence_resource(
        &self,
        uri: &str,
        workspace: &str,
        id: &str,
    ) -> Option<ReadResourceResult> {
        let store = EvidenceStore::new(self.paths.clone());
        let evidence = store.read(workspace, id).ok()?;
        let raw = store.read_raw(workspace, id).ok()?;
        Some(ReadResourceResult::text(
            uri,
            "application/json",
            evidence_fence_json(&evidence, &raw).pretty(),
        ))
    }
}

impl ToolRegistry for ZbrainRegistry {
    fn tools(&self) -> Vec<ToolEntry> {
        vec![ToolEntry {
            description: Some("Snapshot a local source file into an immutable evidence record.".to_string()),
            input_schema: evidence_capture_input_schema(),
            name: "evidence_capture".to_string(),
        }]
    }

    fn call_tool(&self, name: &str, arguments: Option<&Value>) -> Result<CallToolResult, McpError> {
        if name != "evidence_capture" {
            return Err(McpError::unknown_tool(name));
        }
        let started = Instant::now();
        let outcome = self.run_evidence_capture(arguments);
        let workspace = arguments
            .and_then(|arguments| arguments.get("workspace"))
            .and_then(Value::as_str)
            .filter(|workspace| !workspace.is_empty())
            .unwrap_or("current");
        self.stderr.log(&format!(
            "mcp tool={name} workspace={workspace} duration={:?}",
            started.elapsed()
        ));
        outcome
    }

    fn resource_templates(&self) -> Vec<Value> {
        use serde_json::json;
        vec![
            json!({
                "description": "Read a canonical claim by workspace and ID.",
                "mimeType": "application/json",
                "name": "Claim",
                "uriTemplate": "zbrain://workspace/{workspace}/claim/{id}",
            }),
            json!({
                "description": "Read an evidence record by workspace and ID.",
                "mimeType": "application/json",
                "name": "Evidence",
                "uriTemplate": "zbrain://workspace/{workspace}/evidence/{id}",
            }),
        ]
    }

    fn read_resource(&self, uri: &str) -> Option<ReadResourceResult> {
        let (workspace, resource_type, id) = parse_workspace_uri(uri)?;
        crate::boundary::validate_workspace(&self.paths, workspace).ok()?;
        match resource_type {
            "claim" => self.read_claim_resource(uri, workspace, id),
            "evidence" => self.read_evidence_resource(uri, workspace, id),
            _ => None,
        }
    }
}

/// Ports `handleResource`'s URI routing: `zbrain://workspace/{workspace}/{claim|evidence}/{id}`,
/// mirroring `url.Parse` + `strings.Trim(u.Path, "/")` (leading/trailing
/// slashes dropped, extra path segments rejected).
fn parse_workspace_uri(uri: &str) -> Option<(&str, &str, &str)> {
    let rest = uri.strip_prefix("zbrain://workspace/")?;
    let rest = rest.trim_matches('/');
    let mut parts = rest.split('/');
    let workspace = parts.next()?;
    let resource_type = parts.next()?;
    let id = parts.next()?;
    if parts.next().is_some() {
        return None;
    }
    Some((workspace, resource_type, id))
}

/// Tool-level `isError` result, mirroring the SDK's `SetError`: the error
/// text becomes a single text content block.
fn is_error_result(message: &str) -> CallToolResult {
    CallToolResult {
        content: vec![ContentBlock { r#type: "text", text: message.to_string() }],
        is_error: true,
        ..Default::default()
    }
}

/// The `evidence_capture` input schema as `jsonschema.ForType` renders the
/// Go struct: field-order properties, `required` in field order, and
/// `additionalProperties: false`.
fn evidence_capture_input_schema() -> OrderedJson {
    fn string_property(description: &str) -> OrderedJson {
        OrderedJson::object(vec![
            ("type", OrderedJson::string("string")),
            ("description", OrderedJson::string(description)),
        ])
    }
    OrderedJson::object(vec![
        ("type", OrderedJson::string("object")),
        (
            "properties",
            OrderedJson::object(vec![
                (
                    "workspace",
                    string_property("target workspace; defaults to the current workspace"),
                ),
                ("file", string_property("local source file to snapshot")),
                (
                    "origin",
                    string_property("origin URI or path recorded in the evidence metadata"),
                ),
                ("media_type", string_property("optional media type")),
            ]),
        ),
        (
            "required",
            OrderedJson::array(vec![
                OrderedJson::string("file"),
                OrderedJson::string("origin"),
            ]),
        ),
        ("additionalProperties", OrderedJson::Bool(false)),
    ])
}

/// Schema-validation gate for `evidence_capture` arguments, mirroring the
/// SDK's `applySchema` against the inferred schema (message shapes ported
/// from `jsonschema-go`'s validate errors).
fn validate_capture_arguments(arguments: Option<&Value>) -> Result<(), String> {
    let Some(arguments) = arguments else {
        // Absent (or JSON-null) arguments decode to an empty object.
        return missing_properties(&["file", "origin"]);
    };
    let Value::Object(map) = arguments else {
        return Err(format!(
            "unmarshaling arguments: json: cannot unmarshal {} into Go value of type map[string]interface {{}}",
            json_kind(arguments)
        ));
    };
    let mut missing = Vec::new();
    for required in ["file", "origin"] {
        if !map.contains_key(required) {
            missing.push(required);
        }
    }
    if !missing.is_empty() {
        return missing_properties(&missing);
    }
    let mut unknown: Vec<&String> = map
        .keys()
        .filter(|key| !matches!(key.as_str(), "workspace" | "file" | "origin" | "media_type"))
        .collect();
    unknown.sort();
    if !unknown.is_empty() {
        let quoted: Vec<String> = unknown.iter().map(|key| format!("\"{key}\"")).collect();
        return Err(format!("unexpected additional properties [{}]", quoted.join(" ")));
    }
    for property in ["workspace", "file", "origin", "media_type"] {
        if let Some(value) = map.get(property) {
            if value.is_null() {
                // jsonschema-go treats nil instances as missing and accepts.
                continue;
            }
            if !value.is_string() {
                return Err(format!(
                    "validating /properties/{property}: type: {} has type {:?}, want \"string\"",
                    go_render(value),
                    json_kind(value)
                ));
            }
        }
    }
    Ok(())
}

fn missing_properties(properties: &[&str]) -> Result<(), String> {
    let quoted: Vec<String> = properties.iter().map(|property| format!("\"{property}\"")).collect();
    Err(format!("required: missing properties: [{}]", quoted.join(" ")))
}

fn json_kind(value: &Value) -> &'static str {
    match value {
        Value::Null => "null",
        Value::Bool(_) => "bool",
        Value::Number(_) => "number",
        Value::String(_) => "string",
        Value::Array(_) => "array",
        Value::Object(_) => "object",
    }
}

/// Go `%v` rendering of a decoded JSON value, for type-error messages.
fn go_render(value: &Value) -> String {
    match value {
        Value::Null => "<nil>".to_string(),
        Value::Bool(true) => "true".to_string(),
        Value::Bool(false) => "false".to_string(),
        Value::Number(number) => number.to_string(),
        Value::String(text) => text.clone(),
        Value::Array(items) => {
            let rendered: Vec<String> = items.iter().map(go_render).collect();
            format!("[{}]", rendered.join(" "))
        }
        Value::Object(map) => {
            let mut keys: Vec<&String> = map.keys().collect();
            keys.sort();
            let rendered: Vec<String> = keys
                .iter()
                .map(|key| format!("{}:{}", key, go_render(&map[key.as_str()])))
                .collect();
            format!("map[{}]", rendered.join(" "))
        }
    }
}

/// `evidence_capture` output: `{schema_version, workspace, ...Evidence}` with
/// Go's `Evidence` json field order (including `deduped`).
fn capture_output_json(workspace: &str, evidence: &Evidence) -> OrderedJson {
    let mut entries = vec![
        ("schema_version", OrderedJson::Int(1)),
        ("workspace", OrderedJson::string(workspace)),
    ];
    entries.extend(evidence_json_entries(evidence));
    OrderedJson::object(entries)
}

/// Go `Evidence` wire fields in struct order; `deduped` is json-only
/// (`yaml:"-"`) and always present.
fn evidence_json_entries(evidence: &Evidence) -> Vec<(&'static str, OrderedJson)> {
    vec![
        ("id", OrderedJson::string(&evidence.id)),
        ("origin", OrderedJson::string(&evidence.origin)),
        ("captured_at", OrderedJson::string(&evidence.captured_at)),
        ("media_type", OrderedJson::string(&evidence.media_type)),
        ("byte_length", OrderedJson::Int(evidence.byte_length)),
        ("sha256", OrderedJson::string(&evidence.sha256)),
        ("deduped", OrderedJson::Bool(evidence.deduped)),
    ]
}

/// Ports the fenced `untrusted_evidence` envelope: trust marker at top
/// level, raw snapshot bytes only under the fence, never as top-level
/// `raw_content`.
fn evidence_fence_json(evidence: &Evidence, raw: &[u8]) -> OrderedJson {
    OrderedJson::object(vec![
        ("schema_version", OrderedJson::Int(1)),
        ("trust", OrderedJson::string("untrusted_evidence")),
        (
            "evidence",
            OrderedJson::object(evidence_json_entries(evidence)),
        ),
        (
            "untrusted_evidence",
            OrderedJson::object(vec![(
                "raw_content",
                OrderedJson::string(String::from_utf8_lossy(raw).into_owned()),
            )]),
        ),
    ])
}

/// Canonical claim JSON as `json.MarshalIndent(claim)` renders the Go
/// `Claim` struct: PascalCase field names (the struct has no json tags),
/// every field present, nil slices as `null`, nested structs using their
/// snake_case json tags.
fn claim_resource_json(claim: &Claim) -> OrderedJson {
    OrderedJson::object(vec![
        ("Schema", OrderedJson::string(&claim.schema)),
        ("Type", OrderedJson::string(&claim.claim_type)),
        ("ID", OrderedJson::string(&claim.id)),
        ("Tier", OrderedJson::string(&claim.tier)),
        ("Path", OrderedJson::string(&claim.path)),
        ("Status", OrderedJson::string(&claim.status)),
        ("Title", OrderedJson::string(&claim.title)),
        ("Description", OrderedJson::string(&claim.description)),
        ("Resource", OrderedJson::string(&claim.resource)),
        ("Basis", OrderedJson::string(&claim.basis)),
        ("CreatedAt", OrderedJson::string(&claim.created_at)),
        ("CreatedBy", OrderedJson::string(&claim.created_by)),
        ("VerifiedAt", OrderedJson::string(&claim.verified_at)),
        ("VerifiedBy", OrderedJson::string(&claim.verified_by)),
        ("VerifiedDigest", OrderedJson::string(&claim.verified_digest)),
        ("StaleAfter", OrderedJson::string(&claim.stale_after)),
        (
            "Sources",
            optional_slice(
                &claim.sources,
                claim_source_json,
            ),
        ),
        ("EvidenceIDs", OrderedJson::strings_or_null(&claim.evidence_ids)),
        (
            "SupportingClaimIDs",
            OrderedJson::strings_or_null(&claim.supporting_claim_ids),
        ),
        ("Supersedes", OrderedJson::strings_or_null(&claim.supersedes)),
        (
            "ConflictsWith",
            OrderedJson::strings_or_null(&claim.conflicts_with),
        ),
        (
            "Contradicts",
            optional_slice(&claim.contradicts, |contradiction| {
                contradiction_json(contradiction)
            }),
        ),
        ("Tags", OrderedJson::strings_or_null(&claim.tags)),
        (
            "Transitions",
            optional_slice(&claim.transitions, |transition| {
                claim_transition_json(transition)
            }),
        ),
        ("Body", OrderedJson::string(&claim.body)),
    ])
}

fn optional_slice<T, F: Fn(&T) -> OrderedJson>(values: &[T], render: F) -> OrderedJson {
    if values.is_empty() {
        return OrderedJson::Null;
    }
    OrderedJson::Array(values.iter().map(render).collect())
}

/// Go `ClaimSource` json tags: `title` and `spans` are omitempty.
fn claim_source_json(source: &ClaimSource) -> OrderedJson {
    let mut entries = vec![
        ("id", OrderedJson::string(&source.id)),
        ("resource", OrderedJson::string(&source.resource)),
    ];
    if !source.title.is_empty() {
        entries.push(("title", OrderedJson::string(&source.title)));
    }
    entries.push(("digest", OrderedJson::string(&source.digest)));
    if !source.spans.is_empty() {
        entries.push((
            "spans",
            OrderedJson::Array(source.spans.iter().map(evidence_span_json).collect()),
        ));
    }
    OrderedJson::object(entries)
}

fn evidence_span_json(span: &EvidenceSpan) -> OrderedJson {
    OrderedJson::object(vec![
        ("evidence_id", OrderedJson::string(&span.evidence_id)),
        ("start_line", OrderedJson::Int(span.start_line)),
        ("end_line", OrderedJson::Int(span.end_line)),
        ("digest", OrderedJson::string(&span.digest)),
    ])
}

fn contradiction_json(contradiction: &Contradiction) -> OrderedJson {
    OrderedJson::object(vec![
        ("claim_id", OrderedJson::string(&contradiction.claim_id)),
        ("heuristic", OrderedJson::string(&contradiction.heuristic)),
    ])
}

/// Go `ClaimTransition` json tags: `reason`, `related_claim_ids`,
/// `prior_verification_digest`, and `authorization` are omitempty pointers.
fn claim_transition_json(transition: &ClaimTransition) -> OrderedJson {
    let mut entries = vec![
        ("kind", OrderedJson::string(&transition.kind)),
        ("at", OrderedJson::string(&transition.at)),
        ("by", OrderedJson::string(&transition.by)),
    ];
    if !transition.reason.is_empty() {
        entries.push(("reason", OrderedJson::string(&transition.reason)));
    }
    if !transition.related_claim_ids.is_empty() {
        entries.push((
            "related_claim_ids",
            OrderedJson::strings_or_null(&transition.related_claim_ids),
        ));
    }
    if !transition.prior_verification_digest.is_empty() {
        entries.push((
            "prior_verification_digest",
            OrderedJson::string(&transition.prior_verification_digest),
        ));
    }
    if let Some(authorization) = &transition.authorization {
        entries.push((
            "authorization",
            claim_transition_authorization_json(authorization),
        ));
    }
    OrderedJson::object(entries)
}

/// All `ClaimTransitionAuthorization` fields are omitempty; a non-nil
/// pointer still renders as an object (possibly `{}`).
fn claim_transition_authorization_json(authorization: &ClaimTransitionAuthorization) -> OrderedJson {
    let mut entries = Vec::new();
    if !authorization.challenge_id.is_empty() {
        entries.push(("challenge_id", OrderedJson::string(&authorization.challenge_id)));
    }
    if !authorization.method.is_empty() {
        entries.push(("method", OrderedJson::string(&authorization.method)));
    }
    if !authorization.mcp_client.is_empty() {
        entries.push(("mcp_client", OrderedJson::string(&authorization.mcp_client)));
    }
    OrderedJson::object(entries)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::claims::{
        new_claim_id, CLAIM_BASIS_EVIDENCE, OKF_CLAIM_TYPE,
    };
    use crate::clock::{rfc3339, FixedClock};
    use crate::config::ensure_config;
    use crate::mcp::protocol::SERVER_NAME;
    use crate::mcp::server::{McpOptions, Server};
    use crate::mcp::transport::MemoryTransport;
    use crate::paths::Options;
    use crate::workspace::create_workspace;
    use chrono::{TimeZone, Utc};
    use serde_json::{json, Value};

    const CLAIM_URI_TEMPLATE: &str = "zbrain://workspace/{workspace}/claim/{id}";
    const EVIDENCE_URI_TEMPLATE: &str = "zbrain://workspace/{workspace}/evidence/{id}";

    /// Ports `resourceFixture`: isolated runtime with one workspace, one
    /// evidence record, and one draft claim (approval is owner-pinned and
    /// lands with m3; resource reads do not filter by status).
    struct Fixture {
        _dir: std::path::PathBuf,
        paths: Paths,
        clock: FixedClock,
        claim_id: String,
        evidence_id: String,
    }

    fn fixture(name: &str) -> Fixture {
        let dir = std::env::temp_dir().join(format!(
            "zbrain-mcp-gateway-{}-{name}",
            std::process::id()
        ));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let paths = Paths::resolve(Options {
            cwd: Some(dir.clone()),
            home_dir: Some(dir.clone()),
            runtime_dir: Some(dir.join(".zbrain")),
        })
        .unwrap();
        ensure_config(&paths.config_file).unwrap();
        let clock = FixedClock::new(Utc.with_ymd_and_hms(2026, 8, 20, 0, 0, 0).unwrap());
        create_workspace(&paths, "research", &clock).unwrap();

        let source = dir.join("source.txt");
        std::fs::write(&source, b"raw snapshot bytes").unwrap();
        let evidence = EvidenceStore::new(paths.clone())
            .add_file("research", &source, "file://source.txt", "text/plain", &clock)
            .unwrap();

        let claim_id = new_claim_id().unwrap();
        let draft = ClaimStore::new(paths.clone())
            .write_draft(
                "research",
                Claim {
                    claim_type: OKF_CLAIM_TYPE.to_string(),
                    id: claim_id.clone(),
                    tier: "projects".to_string(),
                    title: "Resource Claim".to_string(),
                    basis: CLAIM_BASIS_EVIDENCE.to_string(),
                    created_at: rfc3339(clock.now()),
                    created_by: "test".to_string(),
                    evidence_ids: vec![evidence.id.clone()],
                    body: "Resource claim body".to_string(),
                    ..Default::default()
                },
            )
            .unwrap();
        assert_eq!(draft.status, "draft");
        Fixture { _dir: dir, paths, clock, claim_id, evidence_id: evidence.id }
    }

    fn registry(fixture: &Fixture) -> ZbrainRegistry {
        ZbrainRegistry::new(fixture.paths.clone(), Box::new(FixedClock::new(fixture.clock.now())))
    }

    /// Drives requests through the full server over the memory transport,
    /// returning the raw response frames (for byte-shape assertions).
    fn run_session_raw(registry: ZbrainRegistry, requests: &str) -> Vec<String> {
        let options = McpOptions { version: "test".to_string(), ..Default::default() };
        let mut server = Server::new(registry, options);
        let mut transport = MemoryTransport::with_requests(requests.as_bytes().to_vec());
        server.run(&mut transport).unwrap();
        String::from_utf8(transport.into_writer())
            .expect("responses utf-8")
            .lines()
            .map(str::to_string)
            .collect()
    }

    fn run_session(registry: ZbrainRegistry, requests: &str) -> Vec<Value> {
        run_session_raw(registry, requests)
            .iter()
            .map(|line| serde_json::from_str(line).expect("response frame"))
            .collect()
    }

    fn initialize_frame(id: i64) -> String {
        format!(
            r#"{{"jsonrpc":"2.0","id":{id},"method":"initialize","params":{{"protocolVersion":"2025-06-18","capabilities":{{}},"clientInfo":{{"name":"zbrain-test","version":"0.0.0"}}}}}}"#
        )
    }

    fn resources_read_frame(id: i64, uri: &str) -> String {
        format!(
            r#"{{"jsonrpc":"2.0","id":{id},"method":"resources/read","params":{{"uri":"{uri}"}}}}"#
        )
    }

    fn tools_call_frame(id: i64, arguments: &str) -> String {
        format!(
            r#"{{"jsonrpc":"2.0","id":{id},"method":"tools/call","params":{{"name":"evidence_capture","arguments":{arguments}}}}}"#
        )
    }

    fn result_field(response: &Value, key: &str) -> Value {
        response["result"][key].clone()
    }

    /// Byte-shape guard: the JSON keys appear in exactly this order.
    fn assert_key_order(text: &str, keys: &[&str]) {
        let mut cursor = 0;
        for key in keys {
            let needle = format!("\"{key}\"");
            let position = text[cursor..]
                .find(&needle)
                .unwrap_or_else(|| panic!("key {key} missing from {text}"))
                + cursor;
            assert!(
                position >= cursor,
                "key {key} out of order in {text}"
            );
            cursor = position + needle.len();
        }
    }

    // --- Resource surface (ports TestResourceSurface) ---

    #[test]
    fn advertises_both_resource_templates() {
        let fix = fixture("templates");
        let templates = registry(&fix).resource_templates();
        assert_eq!(templates.len(), 2);
        assert_eq!(templates[0]["uriTemplate"], CLAIM_URI_TEMPLATE);
        assert_eq!(templates[0]["name"], "Claim");
        assert_eq!(templates[0]["mimeType"], "application/json");
        assert_eq!(templates[1]["uriTemplate"], EVIDENCE_URI_TEMPLATE);
        assert_eq!(templates[1]["name"], "Evidence");
        // Wire key order mirrors the go-sdk ResourceTemplate struct.
        let rendered = templates[0].to_string();
        assert_key_order(&rendered, &["description", "mimeType", "name", "uriTemplate"]);
    }

    #[test]
    fn claim_resource_read_returns_canonical_claim() {
        let fix = fixture("claim-read");
        let reg = registry(&fix);
        let claim = ClaimStore::new(fix.paths.clone()).read("research", &fix.claim_id).unwrap();
        let uri = format!("zbrain://workspace/research/claim/{}", fix.claim_id);
        let read = reg.read_resource(&uri).expect("claim resource");
        assert_eq!(read.ttl_ms, 0);
        assert_eq!(read.cache_scope, "public");
        assert_eq!(read.contents.len(), 1);
        assert_eq!(read.contents[0].uri, uri);
        assert_eq!(read.contents[0].mime_type.as_deref(), Some("application/json"));
        let text = read.contents[0].text.clone().expect("text");

        // Byte-exact Go field order for the untagged Claim struct.
        assert_key_order(
            &text,
            &[
                "Schema", "Type", "ID", "Tier", "Path", "Status", "Title", "Description",
                "Resource", "Basis", "CreatedAt", "CreatedBy", "VerifiedAt", "VerifiedBy",
                "VerifiedDigest", "StaleAfter", "Sources", "EvidenceIDs", "SupportingClaimIDs",
                "Supersedes", "ConflictsWith", "Contradicts", "Tags", "Transitions", "Body",
            ],
        );
        let parsed: serde_json::Map<String, Value> =
            serde_json::from_str(&text).expect("claim json");
        // The OKF parse path leaves Schema empty (Go does too: only the
        // legacy frontmatter carries a schema).
        assert_eq!(parsed["Schema"], "");
        assert_eq!(parsed["Type"], "zbrain.claim");
        assert_eq!(parsed["ID"], fix.claim_id);
        assert_eq!(parsed["Tier"], "projects");
        assert_eq!(parsed["Path"], claim.path);
        assert_eq!(parsed["Status"], "draft");
        assert_eq!(parsed["Title"], "Resource Claim");
        assert_eq!(parsed["Basis"], "evidence");
        assert_eq!(parsed["CreatedBy"], "test");
        assert_eq!(parsed["EvidenceIDs"], json!([fix.evidence_id]));
        assert_eq!(parsed["Body"], "Resource claim body");
        for nil_field in [
            "Sources",
            "SupportingClaimIDs",
            "Supersedes",
            "ConflictsWith",
            "Contradicts",
            "Tags",
            "Transitions",
        ] {
            assert!(parsed[nil_field].is_null(), "{nil_field} must marshal as Go nil");
        }
        assert!(text.contains("Resource claim body"));
        assert!(text.contains(&fix.claim_id));
    }

    #[test]
    fn evidence_resource_fenced_envelope() {
        // Ports TestEvidenceResourceFenced: trust marker, nested raw bytes,
        // no top-level raw_content, byte-exact envelope field order.
        let fix = fixture("fenced");
        let uri = format!("zbrain://workspace/research/evidence/{}", fix.evidence_id);
        let read = registry(&fix).read_resource(&uri).expect("evidence resource");
        let text = read.contents[0].text.clone().expect("text");
        assert_key_order(
            &text,
            &["schema_version", "trust", "evidence", "untrusted_evidence"],
        );
        assert_key_order(
            &text,
            &[
                "id", "origin", "captured_at", "media_type", "byte_length", "sha256", "deduped",
            ],
        );
        let parsed: Value = serde_json::from_str(&text).unwrap();
        assert_eq!(parsed["schema_version"], 1);
        assert_eq!(parsed["trust"], "untrusted_evidence");
        assert_eq!(parsed["evidence"]["id"], fix.evidence_id);
        assert_eq!(parsed["evidence"]["origin"], "file://source.txt");
        assert_eq!(parsed["evidence"]["media_type"], "text/plain");
        assert_eq!(parsed["evidence"]["byte_length"], 18);
        assert_eq!(parsed["evidence"]["deduped"], false);
        assert!(parsed.get("raw_content").is_none(), "top-level raw_content present");
        assert_eq!(
            parsed["untrusted_evidence"]["raw_content"],
            "raw snapshot bytes"
        );
    }

    #[test]
    fn resource_read_maps_domain_failures_to_not_found() {
        let fix = fixture("not-found");
        let reg = registry(&fix);
        for uri in [
            format!("zbrain://workspace/research/claim/clm_{}", "f".repeat(32)),
            format!("zbrain://workspace/nonexistent/claim/{}", fix.claim_id),
            format!("zbrain://workspace/research/delete/{}", fix.claim_id),
            "claims://current".to_string(),
            format!("zbrain://workspace/research/claim/{}/extra", fix.claim_id),
        ] {
            assert!(
                reg.read_resource(&uri).is_none(),
                "uri {uri} must map to ResourceNotFound"
            );
        }
    }

    #[test]
    fn resource_read_wire_frames() {
        // Server-level: success frames carry ttlMs/cacheScope (and
        // resultType/_meta only per protocol), failures map to -32602
        // Resource not found with the URI in data. Byte-shape: the success
        // frame is the go-sdk ReadResourceResult wire form exactly.
        let fix = fixture("wire");
        let claim_uri = format!("zbrain://workspace/research/claim/{}", fix.claim_id);
        let responses = run_session_raw(
            registry(&fix),
            &format!(
                "{}\n{}\n{}\n",
                initialize_frame(1),
                resources_read_frame(2, &claim_uri),
                resources_read_frame(3, &format!(
                    "zbrain://workspace/research/claim/{}",
                    "f".repeat(32)
                )),
            ),
        );
        let expected_result_prefix = "{\"ttlMs\":0,\"cacheScope\":\"public\",\"contents\":[{\"uri\":\""
            .to_string()
            + &claim_uri
            + "\",\"mimeType\":\"application/json\",\"text\":\"{";
        let expected_frame_prefix =
            format!("{{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{expected_result_prefix}");
        assert!(
            responses[1].starts_with(&expected_frame_prefix),
            "unexpected read frame: {}",
            responses[1]
        );
        let success_frame: Value = serde_json::from_str(&responses[1]).unwrap();
        assert!(success_frame["result"].get("resultType").is_none());
        assert_eq!(success_frame["result"]["contents"][0]["uri"], claim_uri);
        let error_frame: Value = serde_json::from_str(&responses[2]).unwrap();
        assert_eq!(error_frame["error"]["code"], -32602);
        assert_eq!(error_frame["error"]["message"], "Resource not found");
        assert_eq!(
            error_frame["error"]["data"]["uri"],
            format!("zbrain://workspace/research/claim/{}", "f".repeat(32))
        );
    }

    #[test]
    fn modern_read_carries_result_type_and_server_meta() {
        let fix = fixture("modern-read");
        let claim_uri = format!("zbrain://workspace/research/claim/{}", fix.claim_id);
        let responses = run_session(
            registry(&fix),
            &format!(
                r#"{{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{{"uri":"{claim_uri}","_meta":{{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{{}}}}}}}}"#
            ),
        );
        assert_eq!(result_field(&responses[0], "resultType"), "complete");
        assert_eq!(
            result_field(&responses[0], "_meta")["io.modelcontextprotocol/serverInfo"]["name"],
            SERVER_NAME
        );
    }

    #[test]
    fn resource_reads_are_side_effect_free() {
        let fix = fixture("side-effect-free");
        let reg = registry(&fix);
        let claim_uri = format!("zbrain://workspace/research/claim/{}", fix.claim_id);
        let evidence_uri = format!("zbrain://workspace/research/evidence/{}", fix.evidence_id);
        reg.read_resource(&claim_uri).unwrap();
        reg.read_resource(&evidence_uri).unwrap();
        let claim = ClaimStore::new(fix.paths.clone()).read("research", &fix.claim_id).unwrap();
        assert_eq!(claim.status, "draft");
        let evidence = EvidenceStore::new(fix.paths.clone())
            .read("research", &fix.evidence_id)
            .unwrap();
        assert!(!evidence.sha256.is_empty());
        let raw = EvidenceStore::new(fix.paths.clone())
            .read_raw("research", &fix.evidence_id)
            .unwrap();
        assert_eq!(raw, b"raw snapshot bytes");
    }

    // --- Tools surface (ports the evidence_capture cases of TestToolSurface
    // and TestEvidenceCaptureDedupesIdenticalFile) ---

    #[test]
    fn evidence_capture_is_the_registered_tool_with_exact_schema() {
        let fix = fixture("tool-schema");
        let tools = registry(&fix).tools();
        assert_eq!(tools.len(), 1);
        assert_eq!(tools[0].name, "evidence_capture");
        assert_eq!(
            tools[0].description.as_deref(),
            Some("Snapshot a local source file into an immutable evidence record.")
        );
        // Byte-exact jsonschema-go output: struct field-order properties,
        // required in field order, additionalProperties false.
        assert_eq!(
            tools[0].input_schema.compact(),
            r#"{"type":"object","properties":{"workspace":{"type":"string","description":"target workspace; defaults to the current workspace"},"file":{"type":"string","description":"local source file to snapshot"},"origin":{"type":"string","description":"origin URI or path recorded in the evidence metadata"},"media_type":{"type":"string","description":"optional media type"}},"required":["file","origin"],"additionalProperties":false}"#
        );
    }

    #[test]
    fn evidence_capture_snapshots_and_dedupes() {
        let fix = fixture("capture");
        let source = fix._dir.join("capture.txt");
        std::fs::write(&source, b"capture bytes").unwrap();
        let source_json = source.to_string_lossy().replace('\\', "\\\\");
        let responses = run_session(
            registry(&fix),
            &format!(
                "{}\n{}\n{}\n",
                initialize_frame(1),
                tools_call_frame(2, &format!(r#"{{"file":"{source_json}","origin":"file://capture.txt"}}"#)),
                tools_call_frame(3, &format!(r#"{{"file":"{source_json}","origin":"file://capture.txt"}}"#)),
            ),
        );
        let first = &responses[1];
        assert!(first["result"].get("isError").is_none(), "first capture errored");
        let text = first["result"]["content"][0]["text"].as_str().expect("text");
        assert!(text.contains("\"id\": \"evd_"), "{text}");
        // Wire field order mirrors the Go output struct.
        assert_key_order(
            text,
            &[
                "schema_version",
                "workspace",
                "id",
                "origin",
                "captured_at",
                "media_type",
                "byte_length",
                "sha256",
                "deduped",
            ],
        );
        let first_structured = &first["result"]["structuredContent"];
        assert_eq!(first_structured["workspace"], "research");
        assert_eq!(first_structured["deduped"], false);
        assert!(first_structured["id"]
            .as_str()
            .expect("id")
            .starts_with("evd_"));
        // structuredContent is the compact rendering of the same payload:
        // pretty text re-parses to exactly the structured object.
        let text_value: Value = serde_json::from_str(text).unwrap();
        assert_eq!(text_value, *first_structured);
        let second = &responses[2];
        let second_structured = &second["result"]["structuredContent"];
        assert_eq!(second_structured["id"], first_structured["id"]);
        assert_eq!(second_structured["deduped"], true);
    }

    #[test]
    fn evidence_capture_dedupes_across_origins() {
        let fix = fixture("dedupe-origin");
        let source = fix._dir.join("capture.txt");
        std::fs::write(&source, b"capture bytes").unwrap();
        let source_json = source.to_string_lossy().replace('\\', "\\\\");
        let reg = registry(&fix);
        let first = reg
            .call_tool(
                "evidence_capture",
                Some(&serde_json::from_str::<Value>(&format!(
                    r#"{{"file":"{source_json}","origin":"file://one.txt"}}"#
                ))
                .unwrap()),
            )
            .unwrap();
        let second = reg
            .call_tool(
                "evidence_capture",
                Some(&serde_json::from_str::<Value>(&format!(
                    r#"{{"file":"{source_json}","origin":"file://two.txt"}}"#
                ))
                .unwrap()),
            )
            .unwrap();
        assert!(!first.is_error && !second.is_error);
        let out = |result: &CallToolResult| {
            serde_json::to_value(result.structured_content.clone().expect("structured")).unwrap()
        };
        let first_value = out(&first);
        let second_value = out(&second);
        assert_eq!(second_value["id"], first_value["id"]);
        assert_eq!(second_value["deduped"], true);
        // Dedup returns the existing record: the original origin is kept.
        assert_eq!(second_value["origin"], "file://one.txt");
    }

    #[test]
    fn evidence_capture_error_paths() {
        let fix = fixture("capture-errors");
        let reg = registry(&fix);
        let args = |json_text: &str| serde_json::from_str::<Value>(json_text).unwrap();

        // Schema-invalid: missing required fields fail closed as isError.
        let result = reg.call_tool("evidence_capture", Some(&args("{}"))).unwrap();
        assert!(result.is_error);
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": required: missing properties: [\"file\" \"origin\"]"
        );
        let result = reg
            .call_tool("evidence_capture", Some(&args(r#"{"file":"x"}"#)))
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": required: missing properties: [\"origin\"]"
        );

        // Unknown property fails closed as isError.
        let result = reg
            .call_tool(
                "evidence_capture",
                Some(&args(r#"{"file":"f","origin":"o","extra":1}"#)),
            )
            .unwrap();
        assert!(result.is_error);
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": unexpected additional properties [\"extra\"]"
        );

        // Wrong-typed property fails closed as isError.
        let result = reg
            .call_tool(
                "evidence_capture",
                Some(&args(r#"{"file":3,"origin":"o"}"#)),
            )
            .unwrap();
        assert!(result.is_error);
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating /properties/file: type: 3 has type \"number\", want \"string\""
        );

        // Empty strings pass schema validation and hit the handler guards.
        let result = reg
            .call_tool("evidence_capture", Some(&args(r#"{"file":"","origin":"o"}"#)))
            .unwrap();
        assert!(result.is_error);
        assert_eq!(result.content[0].text, "file is required");
        let result = reg
            .call_tool(
                "evidence_capture",
                Some(&args(r#"{"file":"x","origin":"  "}"#)),
            )
            .unwrap();
        assert!(result.is_error);
        assert_eq!(result.content[0].text, "origin is required");

        // Domain failures map to isError.
        let missing = fix._dir.join("no-such-file.txt");
        let missing_json = missing.to_string_lossy().replace('\\', "\\\\");
        let result = reg
            .call_tool(
                "evidence_capture",
                Some(&args(&format!(
                    r#"{{"file":"{missing_json}","origin":"file://x"}}"#
                ))),
            )
            .unwrap();
        assert!(result.is_error);
        assert!(!result.content[0].text.is_empty());
        let result = reg
            .call_tool(
                "evidence_capture",
                Some(&args(r#"{"file":"x","origin":"o","workspace":"nonexistent"}"#)),
            )
            .unwrap();
        assert!(result.is_error);
        assert!(!result.content[0].text.is_empty());
    }

    #[test]
    fn oversized_evidence_capture_origin_maps_to_32602() {
        let fix = fixture("oversized");
        let reg = registry(&fix);
        let oversized = "c".repeat((1 << 20) + 10);
        let arguments = serde_json::json!({ "file": "f", "origin": oversized });
        let error = reg.call_tool("evidence_capture", Some(&arguments)).unwrap_err();
        assert_eq!(error.to_wire("tools/call").code, -32602);
        assert_eq!(error.to_wire("tools/call").message, "input exceeds 1MB limit");
    }

    #[test]
    fn unknown_tool_maps_to_32602() {
        let fix = fixture("unknown-tool");
        let error = registry(&fix)
            .call_tool("no_such_tool", None)
            .unwrap_err();
        assert_eq!(error.to_wire("tools/call").code, -32602);
    }

    #[test]
    fn uri_routing_mirrors_go_url_parse() {
        assert_eq!(
            parse_workspace_uri("zbrain://workspace/research/claim/clm_x"),
            Some(("research", "claim", "clm_x"))
        );
        assert_eq!(
            parse_workspace_uri("zbrain://workspace/research/evidence/evd_x/"),
            Some(("research", "evidence", "evd_x"))
        );
        assert_eq!(parse_workspace_uri("claims://current"), None);
        assert_eq!(parse_workspace_uri("zbrain://workspace"), None);
        assert_eq!(parse_workspace_uri("zbrain://workspace/research/claim"), None);
        assert_eq!(
            parse_workspace_uri("zbrain://workspace/research/claim/a/b"),
            None
        );
        assert_eq!(parse_workspace_uri("zbrain://workspace//claim/id"), None);
    }

    /// Golden byte-identity: the fence, capture output, and claim resource
    /// renders must equal the Go oracle's `json.MarshalIndent` bytes
    /// (captured from the Go structs, including nil slices as null).
    #[test]
    fn rendered_payloads_match_go_bytes() {
        let evidence = Evidence {
            id: "evd_1".to_string(),
            origin: "file://source.txt".to_string(),
            captured_at: "2026-08-20T00:00:00Z".to_string(),
            media_type: "text/plain".to_string(),
            byte_length: 17,
            sha256: "abc".to_string(),
            deduped: false,
        };
        assert_eq!(
            evidence_fence_json(&evidence, b"raw snapshot bytes").pretty(),
            concat!(
                "{\n",
                "  \"schema_version\": 1,\n",
                "  \"trust\": \"untrusted_evidence\",\n",
                "  \"evidence\": {\n",
                "    \"id\": \"evd_1\",\n",
                "    \"origin\": \"file://source.txt\",\n",
                "    \"captured_at\": \"2026-08-20T00:00:00Z\",\n",
                "    \"media_type\": \"text/plain\",\n",
                "    \"byte_length\": 17,\n",
                "    \"sha256\": \"abc\",\n",
                "    \"deduped\": false\n",
                "  },\n",
                "  \"untrusted_evidence\": {\n",
                "    \"raw_content\": \"raw snapshot bytes\"\n",
                "  }\n",
                "}"
            )
        );

        let captured = Evidence {
            id: "evd_2".to_string(),
            origin: "file://capture.txt".to_string(),
            captured_at: "2026-08-20T00:00:00Z".to_string(),
            media_type: "application/octet-stream".to_string(),
            byte_length: 13,
            sha256: "def".to_string(),
            deduped: true,
        };
        assert_eq!(
            capture_output_json("research", &captured).pretty(),
            concat!(
                "{\n",
                "  \"schema_version\": 1,\n",
                "  \"workspace\": \"research\",\n",
                "  \"id\": \"evd_2\",\n",
                "  \"origin\": \"file://capture.txt\",\n",
                "  \"captured_at\": \"2026-08-20T00:00:00Z\",\n",
                "  \"media_type\": \"application/octet-stream\",\n",
                "  \"byte_length\": 13,\n",
                "  \"sha256\": \"def\",\n",
                "  \"deduped\": true\n",
                "}"
            )
        );

        let claim = Claim {
            schema: "zbrain.claim/v1".to_string(),
            claim_type: OKF_CLAIM_TYPE.to_string(),
            id: "clm_abc".to_string(),
            tier: "projects".to_string(),
            path: "projects/clm_abc.md".to_string(),
            status: "approved".to_string(),
            title: "Resource Claim".to_string(),
            basis: CLAIM_BASIS_EVIDENCE.to_string(),
            created_at: "2026-08-20T00:00:00Z".to_string(),
            created_by: "test".to_string(),
            evidence_ids: vec!["evd_1".to_string()],
            body: "Resource claim body".to_string(),
            ..Default::default()
        };
        assert_eq!(
            claim_resource_json(&claim).pretty(),
            concat!(
                "{\n",
                "  \"Schema\": \"zbrain.claim/v1\",\n",
                "  \"Type\": \"zbrain.claim\",\n",
                "  \"ID\": \"clm_abc\",\n",
                "  \"Tier\": \"projects\",\n",
                "  \"Path\": \"projects/clm_abc.md\",\n",
                "  \"Status\": \"approved\",\n",
                "  \"Title\": \"Resource Claim\",\n",
                "  \"Description\": \"\",\n",
                "  \"Resource\": \"\",\n",
                "  \"Basis\": \"evidence\",\n",
                "  \"CreatedAt\": \"2026-08-20T00:00:00Z\",\n",
                "  \"CreatedBy\": \"test\",\n",
                "  \"VerifiedAt\": \"\",\n",
                "  \"VerifiedBy\": \"\",\n",
                "  \"VerifiedDigest\": \"\",\n",
                "  \"StaleAfter\": \"\",\n",
                "  \"Sources\": null,\n",
                "  \"EvidenceIDs\": [\n",
                "    \"evd_1\"\n",
                "  ],\n",
                "  \"SupportingClaimIDs\": null,\n",
                "  \"Supersedes\": null,\n",
                "  \"ConflictsWith\": null,\n",
                "  \"Contradicts\": null,\n",
                "  \"Tags\": null,\n",
                "  \"Transitions\": null,\n",
                "  \"Body\": \"Resource claim body\"\n",
                "}"
            )
        );

        // Go's default HTML escaping applies to string values.
        let mut html_claim = claim.clone();
        html_claim.title = "<a> & \"b\"".to_string();
        let text = claim_resource_json(&html_claim).compact();
        assert!(text.contains(r#""Title":"\u003ca\u003e \u0026 \"b\"""#), "{text}");
    }

    #[test]
    fn pretty_rendering_matches_go_indentation() {
        let nested = OrderedJson::object(vec![
            ("a", OrderedJson::Int(1)),
            (
                "list",
                OrderedJson::Array(vec![
                    OrderedJson::string("x"),
                    OrderedJson::object(vec![("b", OrderedJson::Bool(true))]),
                ]),
            ),
            ("empty", OrderedJson::Array(vec![])),
            ("obj", OrderedJson::object(vec![])),
        ]);
        assert_eq!(
            nested.pretty(),
            concat!(
                "{\n",
                "  \"a\": 1,\n",
                "  \"list\": [\n",
                "    \"x\",\n",
                "    {\n",
                "      \"b\": true\n",
                "    }\n",
                "  ],\n",
                "  \"empty\": [],\n",
                "  \"obj\": {}\n",
                "}"
            )
        );
        assert_eq!(
            nested.compact(),
            r#"{"a":1,"list":["x",{"b":true}],"empty":[],"obj":{}}"#
        );
    }
}
