//! gateway.rs — the real tool/resource registry behind [`ToolRegistry`].
//! W2.T1 ported `internal/mcp/resources.go` (claim + evidence read
//! resources) and the evidence_capture tool; W2.T2 ports the remaining
//! non-campaign tools of `internal/mcp/tools.go` (workspace_current,
//! memory_ask, memory_status, memory_reindex, claim_draft). Campaign tools
//! (campaign_begin/next/submit_draft) and claim_lifecycle slot in with the
//! m6 approval/campaign runtime (ChallengeStore/CampaignStore) and are
//! intentionally absent.

use std::path::Path;
use std::time::Instant;

use serde_json::Value;

use crate::claims::{
    new_claim_id, Claim, ClaimSource, ClaimStore, ClaimTransition, ClaimTransitionAuthorization,
    Contradiction, EvidenceSpan, OKF_CLAIM_TYPE,
};
use crate::clock::{rfc3339, Clock};
use crate::evidence::{Evidence, EvidenceStore};
use crate::embedder::{rebuild_with_options, EmbeddingStore, RebuildOptions};
use crate::index::{approved_catalog, IndexStore, IndexSummary, InvalidClaimSerde};
use crate::mcp::protocol::{
    CallToolResult, ContentBlock, McpError, OrderedJson, ReadResourceResult, ToolEntry,
};
use crate::mcp::server::{ToolRegistry, MCP_MAX_INPUT_BYTES};
use crate::mcp::transport::SafeStderr;
use crate::paths::Paths;
use crate::query::{
    trusted_query, QueryClaim, QueryClaimSource, QueryContradiction, QueryEvidenceSpan,
    TrustedQueryOptions, TrustedQueryResponse,
};

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

/// One property's schema kind, as jsonschema-go renders it.
#[derive(Clone, Copy)]
enum PropertyKind {
    String,
    Boolean,
    /// `{"type": ["null", "array"], "items": {"type": "string"}}`.
    StringArray,
}

/// A tool's inferred input schema as `jsonschema.ForType` renders the Go
/// struct: required list in field order and properties in field order.
struct ToolInput {
    required: &'static [&'static str],
    properties: &'static [(&'static str, PropertyKind)],
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

    /// Ports `runMCPTool`: schema validation fails closed as isError,
    /// the marshaled-input bounds check maps to -32602, then the handler
    /// body runs. (Go's 5s handler timeout has no deterministic test and is
    /// not reproduced.)
    fn run_tool(
        &self,
        arguments: Option<&Value>,
        input: &ToolInput,
        body: impl FnOnce(&Value) -> Result<CallToolResult, McpError>,
    ) -> Result<CallToolResult, McpError> {
        if let Err(message) = validate_arguments(arguments, input) {
            return Ok(is_error_result(&format!("validating \"arguments\": {message}")));
        }
        if let Some(arguments) = arguments {
            let encoded = arguments.to_string();
            if encoded.len() > MCP_MAX_INPUT_BYTES {
                return Err(McpError::invalid_params("input exceeds 1MB limit"));
            }
        }
        body(arguments.unwrap_or(&Value::Object(Default::default())))
    }

    /// Ports the evidence_capture handler body: guards, workspace
    /// resolution, then `EvidenceStore.AddFile`.
    fn run_evidence_capture(
        &self,
        arguments: Option<&Value>,
    ) -> Result<CallToolResult, McpError> {
        self.run_tool(arguments, &EVIDENCE_CAPTURE_INPUT, |arguments| {
            let file = arguments.get("file").and_then(Value::as_str).unwrap_or_default();
            let origin =
                arguments.get("origin").and_then(Value::as_str).unwrap_or_default();
            let media_type = arguments
                .get("media_type")
                .and_then(Value::as_str)
                .unwrap_or_default();
            if file.trim().is_empty() {
                return Ok(is_error_result("file is required"));
            }
            if origin.trim().is_empty() {
                return Ok(is_error_result("origin is required"));
            }
            let workspace = match self.resolve_workspace(
                arguments.get("workspace").and_then(Value::as_str).unwrap_or_default(),
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
        })
    }

    /// Ports the workspace_current handler body: `ResolveCurrentWorkspace`.
    fn run_workspace_current(
        &self,
        arguments: Option<&Value>,
    ) -> Result<CallToolResult, McpError> {
        self.run_tool(arguments, &WORKSPACE_CURRENT_INPUT, |_arguments| {
            let current = match crate::workspace::resolve_current_workspace(&self.paths) {
                Ok(current) => current,
                Err(error) => return Ok(is_error_result(&error.to_string())),
            };
            let out = OrderedJson::object(vec![
                ("schema_version", OrderedJson::Int(1)),
                (
                    "project_root",
                    OrderedJson::string(current.project_root.to_string_lossy().into_owned()),
                ),
                ("workspace", OrderedJson::string(&current.workspace)),
                (
                    "secondary_workspaces",
                    OrderedJson::Array(
                        current
                            .secondary_workspaces
                            .iter()
                            .map(OrderedJson::string)
                            .collect(),
                    ),
                ),
            ]);
            Ok(text_result(out))
        })
    }

    /// Ports the memory_ask handler body: trusted query with a hard limit
    /// of 10, rendered as the Go `TrustedQueryResponse` JSON.
    fn run_memory_ask(&self, arguments: Option<&Value>) -> Result<CallToolResult, McpError> {
        self.run_tool(arguments, &MEMORY_ASK_INPUT, |arguments| {
            let query = arguments.get("query").and_then(Value::as_str).unwrap_or_default();
            if query.trim().is_empty() {
                return Ok(is_error_result("query is required"));
            }
            let workspace = match self.resolve_workspace(
                arguments.get("workspace").and_then(Value::as_str).unwrap_or_default(),
            ) {
                Ok(workspace) => workspace,
                Err(message) => return Ok(is_error_result(&message)),
            };
            let includes: Vec<String> = arguments
                .get("include")
                .and_then(Value::as_array)
                .map(|items| {
                    items
                        .iter()
                        .filter_map(Value::as_str)
                        .map(str::to_string)
                        .collect()
                })
                .unwrap_or_default();
            let response = match trusted_query(
                &self.paths,
                TrustedQueryOptions {
                    workspace,
                    includes,
                    query: query.to_string(),
                    limit: 10,
                    embedding: arguments
                        .get("embedding")
                        .and_then(Value::as_bool)
                        .unwrap_or(false),
                    after: arguments
                        .get("after")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                    before: arguments
                        .get("before")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                    as_of: arguments
                        .get("as_of")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                },
            ) {
                Ok(response) => response,
                Err(error) => return Ok(is_error_result(&error.to_string())),
            };
            Ok(text_result(ask_response_json(&response)))
        })
    }

    /// Ports the memory_status handler body: trust scan, embedding summary,
    /// and index freshness collapsed into one summary object.
    fn run_memory_status(&self, arguments: Option<&Value>) -> Result<CallToolResult, McpError> {
        self.run_tool(arguments, &MEMORY_STATUS_INPUT, |arguments| {
            let workspace = match self.resolve_workspace(
                arguments.get("workspace").and_then(Value::as_str).unwrap_or_default(),
            ) {
                Ok(workspace) => workspace,
                Err(message) => return Ok(is_error_result(&message)),
            };
            let mut summary = IndexSummary { workspace: workspace.clone(), ..Default::default() };
            if let Ok(scan) = ClaimStore::new(self.paths.clone())
                .scan_workspace_for_trust(&workspace)
            {
                summary.approved = scan.claims.len() as i64;
                summary.invalid = scan.invalid.len() as i64;
                summary.invalid_count = scan.invalid.len() as i64;
                summary.invalid_claims =
                    scan.invalid.iter().map(InvalidClaimSerde::from).collect();
                summary.catalog = Some(approved_catalog(&scan.claims));
            }
            summary.embedding = EmbeddingStore::new(self.paths.clone())
                .summary(&workspace, summary.approved);
            if let Err(error) = IndexStore::new(self.paths.clone()).check_fresh(&workspace) {
                summary.rebuild_state = crate::index::REBUILD_STATUS_REJECTED.to_string();
                if summary.invalid == 0 {
                    summary.invalid_claims = vec![InvalidClaimSerde {
                        path: String::new(),
                        error: error.to_string(),
                    }];
                }
            }
            Ok(text_result(summary_output_json(2, &summary)))
        })
    }

    /// Ports the memory_reindex handler body: `RebuildWithOptions`.
    fn run_memory_reindex(&self, arguments: Option<&Value>) -> Result<CallToolResult, McpError> {
        self.run_tool(arguments, &MEMORY_REINDEX_INPUT, |arguments| {
            let workspace = match self.resolve_workspace(
                arguments.get("workspace").and_then(Value::as_str).unwrap_or_default(),
            ) {
                Ok(workspace) => workspace,
                Err(message) => return Ok(is_error_result(&message)),
            };
            let summary = match rebuild_with_options(
                &self.paths,
                &workspace,
                RebuildOptions {
                    embedding: arguments
                        .get("embedding")
                        .and_then(Value::as_bool)
                        .unwrap_or(false),
                },
            ) {
                Ok(summary) => summary,
                Err(error) => return Ok(is_error_result(&error.to_string())),
            };
            Ok(text_result(summary_output_json(1, &summary)))
        })
    }

    /// Ports the claim_draft handler body: draft construction, MarkDirty,
    /// then `WriteDraft` (drafts are promotion candidates, never trusted).
    fn run_claim_draft(&self, arguments: Option<&Value>) -> Result<CallToolResult, McpError> {
        self.run_tool(arguments, &CLAIM_DRAFT_INPUT, |arguments| {
            let workspace = match self.resolve_workspace(
                arguments.get("workspace").and_then(Value::as_str).unwrap_or_default(),
            ) {
                Ok(workspace) => workspace,
                Err(message) => return Ok(is_error_result(&message)),
            };
            let id = match new_claim_id() {
                Ok(id) => id,
                Err(error) => return Ok(is_error_result(&error.to_string())),
            };
            let string_list = |key: &str| -> Vec<String> {
                arguments
                    .get(key)
                    .and_then(Value::as_array)
                    .map(|items| {
                        items
                            .iter()
                            .filter_map(Value::as_str)
                            .map(str::to_string)
                            .collect()
                    })
                    .unwrap_or_default()
            };
            let claim = Claim {
                claim_type: OKF_CLAIM_TYPE.to_string(),
                id,
                tier: arguments.get("tier").and_then(Value::as_str).unwrap_or_default().to_string(),
                title: arguments
                    .get("title")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_string(),
                basis: arguments.get("basis").and_then(Value::as_str).unwrap_or_default().to_string(),
                created_at: rfc3339(self.clock.now()),
                created_by: "owner:mcp".to_string(),
                evidence_ids: string_list("evidence"),
                supporting_claim_ids: string_list("support"),
                conflicts_with: string_list("conflicts_with"),
                body: arguments.get("body").and_then(Value::as_str).unwrap_or_default().to_string(),
                ..Default::default()
            };
            let index = IndexStore::new(self.paths.clone());
            if let Err(error) = index.mark_dirty(&workspace) {
                return Ok(is_error_result(&error.to_string()));
            }
            let created = match ClaimStore::new(self.paths.clone())
                .write_draft(&workspace, claim)
            {
                Ok(created) => created,
                Err(error) => return Ok(is_error_result(&error.to_string())),
            };
            let out = OrderedJson::object(vec![
                ("schema_version", OrderedJson::Int(1)),
                ("workspace", OrderedJson::string(&workspace)),
                ("id", OrderedJson::string(&created.id)),
                ("status", OrderedJson::string(&created.status)),
                ("path", OrderedJson::string(&created.path)),
            ]);
            Ok(text_result(out))
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
        // The go-sdk lists tools sorted by name.
        let mut tools = vec![
            ToolEntry {
                description: Some("Create a draft claim as a promotion candidate (drafts are never trusted answer material).".to_string()),
                input_schema: claim_draft_input_schema(),
                name: "claim_draft".to_string(),
            },
            ToolEntry {
                description: Some("Snapshot a local source file into an immutable evidence record.".to_string()),
                input_schema: evidence_capture_input_schema(),
                name: "evidence_capture".to_string(),
            },
            ToolEntry {
                description: Some("Query trusted memory and return trusted context JSON without calling an LLM.".to_string()),
                input_schema: memory_ask_input_schema(),
                name: "memory_ask".to_string(),
            },
            ToolEntry {
                description: Some("Rebuild the disposable derived index for a workspace.".to_string()),
                input_schema: memory_reindex_input_schema(),
                name: "memory_reindex".to_string(),
            },
            ToolEntry {
                description: Some("Report machine-readable trust, claim, and index health for a workspace.".to_string()),
                input_schema: memory_status_input_schema(),
                name: "memory_status".to_string(),
            },
            ToolEntry {
                description: Some("Resolve the current workspace boundary.".to_string()),
                input_schema: workspace_current_input_schema(),
                name: "workspace_current".to_string(),
            },
        ];
        tools.sort_by(|left, right| left.name.cmp(&right.name));
        tools
    }

    fn call_tool(&self, name: &str, arguments: Option<&Value>) -> Result<CallToolResult, McpError> {
        let started = Instant::now();
        let outcome = match name {
            "claim_draft" => self.run_claim_draft(arguments),
            "evidence_capture" => self.run_evidence_capture(arguments),
            "memory_ask" => self.run_memory_ask(arguments),
            "memory_reindex" => self.run_memory_reindex(arguments),
            "memory_status" => self.run_memory_status(arguments),
            "workspace_current" => self.run_workspace_current(arguments),
            // claim_lifecycle and campaign_* land with the m6
            // approval/campaign runtime; unknown until then.
            _ => return Err(McpError::unknown_tool(name)),
        };
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

/// Success result carrying the pretty JSON as text content and the same
/// payload as structured content (Go's `jsonResult`).
fn text_result(out: OrderedJson) -> CallToolResult {
    CallToolResult {
        content: vec![ContentBlock { r#type: "text", text: out.pretty() }],
        structured_content: Some(out),
        ..Default::default()
    }
}

// ---------------------------------------------------------------------------
// Tool input schemas, byte-exact against `jsonschema.ForType` of the Go
// input structs (captured from `go run ./cmd/zbrain mcp serve` tools/list).
// ---------------------------------------------------------------------------

fn string_property(description: &str) -> OrderedJson {
    OrderedJson::object(vec![
        ("type", OrderedJson::string("string")),
        ("description", OrderedJson::string(description)),
    ])
}

fn boolean_property(description: &str) -> OrderedJson {
    OrderedJson::object(vec![
        ("type", OrderedJson::string("boolean")),
        ("description", OrderedJson::string(description)),
    ])
}

fn string_array_property(description: &str) -> OrderedJson {
    OrderedJson::object(vec![
        ("type", OrderedJson::array(vec![OrderedJson::string("null"), OrderedJson::string("array")])),
        (
            "items",
            OrderedJson::object(vec![("type", OrderedJson::string("string"))]),
        ),
        ("description", OrderedJson::string(description)),
    ])
}

fn input_schema(properties: Vec<(&'static str, OrderedJson)>, required: &[&str]) -> OrderedJson {
    let mut entries = vec![("type", OrderedJson::string("object"))];
    if !properties.is_empty() {
        entries.push(("properties", OrderedJson::object(properties)));
    }
    if !required.is_empty() {
        entries.push((
            "required",
            OrderedJson::Array(required.iter().map(|name| OrderedJson::string(*name)).collect()),
        ));
    }
    entries.push(("additionalProperties", OrderedJson::Bool(false)));
    OrderedJson::object(entries)
}

/// `evidence_capture` (Go `evidenceCaptureIn`).
const EVIDENCE_CAPTURE_INPUT: ToolInput = ToolInput {
    required: &["file", "origin"],
    properties: &[
        ("workspace", PropertyKind::String),
        ("file", PropertyKind::String),
        ("origin", PropertyKind::String),
        ("media_type", PropertyKind::String),
    ],
};

fn evidence_capture_input_schema() -> OrderedJson {
    input_schema(
        vec![
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
        ],
        &["file", "origin"],
    )
}

/// `workspace_current` (Go `currentIn`, an empty struct).
const WORKSPACE_CURRENT_INPUT: ToolInput = ToolInput { required: &[], properties: &[] };

fn workspace_current_input_schema() -> OrderedJson {
    input_schema(vec![], &[])
}

/// `memory_ask` (Go `askIn`).
const MEMORY_ASK_INPUT: ToolInput = ToolInput {
    required: &["query"],
    properties: &[
        ("query", PropertyKind::String),
        ("workspace", PropertyKind::String),
        ("include", PropertyKind::StringArray),
        ("embedding", PropertyKind::Boolean),
        ("after", PropertyKind::String),
        ("before", PropertyKind::String),
        ("as_of", PropertyKind::String),
    ],
};

fn memory_ask_input_schema() -> OrderedJson {
    input_schema(
        vec![
            ("query", string_property("the trusted-memory query")),
            (
                "workspace",
                string_property("workspace to query; defaults to the current workspace"),
            ),
            (
                "include",
                string_array_property("read-only secondary workspace to include"),
            ),
            (
                "embedding",
                boolean_property("enable local hybrid embedding retrieval; defaults to false"),
            ),
            (
                "after",
                string_property("filter claims verified/created at or after RFC3339 timestamp"),
            ),
            (
                "before",
                string_property("filter claims verified/created at or before RFC3339 timestamp"),
            ),
            (
                "as_of",
                string_property("reconstruct active memory state as of RFC3339 timestamp"),
            ),
        ],
        &["query"],
    )
}

/// `memory_status` (Go `statusIn`).
const MEMORY_STATUS_INPUT: ToolInput = ToolInput {
    required: &[],
    properties: &[(
        "workspace",
        PropertyKind::String,
    )],
};

fn memory_status_input_schema() -> OrderedJson {
    input_schema(
        vec![(
            "workspace",
            string_property("workspace to inspect; defaults to the current workspace"),
        )],
        &[],
    )
}

/// `memory_reindex` (Go `reindexIn`).
const MEMORY_REINDEX_INPUT: ToolInput = ToolInput {
    required: &[],
    properties: &[
        ("workspace", PropertyKind::String),
        ("embedding", PropertyKind::Boolean),
    ],
};

fn memory_reindex_input_schema() -> OrderedJson {
    input_schema(
        vec![
            (
                "workspace",
                string_property("workspace whose derived index to rebuild; defaults to the current workspace"),
            ),
            (
                "embedding",
                boolean_property("enable local loopback embedding; defaults to false"),
            ),
        ],
        &[],
    )
}

/// `claim_draft` (Go `claimDraftIn`).
const CLAIM_DRAFT_INPUT: ToolInput = ToolInput {
    required: &["tier", "title", "basis", "body"],
    properties: &[
        ("workspace", PropertyKind::String),
        ("tier", PropertyKind::String),
        ("title", PropertyKind::String),
        ("basis", PropertyKind::String),
        ("evidence", PropertyKind::StringArray),
        ("support", PropertyKind::StringArray),
        ("conflicts_with", PropertyKind::StringArray),
        ("body", PropertyKind::String),
    ],
};

fn claim_draft_input_schema() -> OrderedJson {
    input_schema(
        vec![
            (
                "workspace",
                string_property("target workspace; defaults to the current workspace"),
            ),
            ("tier", string_property("claim tier")),
            ("title", string_property("claim title")),
            ("basis", string_property("owner, evidence, or derived")),
            ("evidence", string_array_property("evidence IDs to bind")),
            ("support", string_array_property("supporting claim IDs")),
            (
                "conflicts_with",
                string_array_property("conflicting claim IDs"),
            ),
            ("body", string_property("claim body")),
        ],
        &["tier", "title", "basis", "body"],
    )
}

/// Schema-validation gate shared by every tool, mirroring the SDK's
/// `applySchema` against the inferred schema. Message shapes ported from
/// `jsonschema-go`'s validate errors against the live Go gateway
/// (`validating root: ` prefixes, integer/number kind names).
fn validate_arguments(arguments: Option<&Value>, input: &ToolInput) -> Result<(), String> {
    let Some(arguments) = arguments else {
        // Absent (or JSON-null) arguments decode to the zero struct; with
        // no required fields that validation passes.
        if input.required.is_empty() {
            return Ok(());
        }
        return Err(missing_properties(input.required));
    };
    let Value::Object(map) = arguments else {
        let raw = serde_json::to_string(arguments).unwrap_or_default();
        return Err(format!(
            "unmarshaling arguments: json: cannot unmarshal {raw:?} into Go value of type map[string]interface {{}}"
        ));
    };
    let mut missing: Vec<&str> = Vec::new();
    for required in input.required {
        if !map.contains_key(*required) {
            missing.push(required);
        }
    }
    if !missing.is_empty() {
        return Err(missing_properties(&missing));
    }
    let mut unknown: Vec<&String> = map
        .keys()
        .filter(|key| !input.properties.iter().any(|(name, _)| name == *key))
        .collect();
    unknown.sort();
    if !unknown.is_empty() {
        let quoted: Vec<String> = unknown.iter().map(|key| format!("\"{key}\"")).collect();
        return Err(format!(
            "validating root: unexpected additional properties [{}]",
            quoted.join(" ")
        ));
    }
    for (property, kind) in input.properties {
        let Some(value) = map.get(*property) else {
            continue;
        };
        if value.is_null() {
            // jsonschema-go treats nil instances as missing and accepts.
            continue;
        }
        match kind {
            PropertyKind::String => {
                if !value.is_string() {
                    return Err(type_error(property, value, "string"));
                }
            }
            PropertyKind::Boolean => {
                if !value.is_boolean() {
                    return Err(type_error(property, value, "boolean"));
                }
            }
            PropertyKind::StringArray => {
                let Value::Array(items) = value else {
                    return Err(format!(
                        "validating root: validating /properties/{property}: type: {} has type {:?}, want one of \"null, array\"",
                        go_render(value),
                        json_kind(value)
                    ));
                };
                for item in items {
                    if !item.is_string() && !item.is_null() {
                        return Err(format!(
                            "validating root: validating /properties/{property}/items: type: {} has type {:?}, want \"string\"",
                            go_render(item),
                            json_kind(item)
                        ));
                    }
                }
            }
        }
    }
    Ok(())
}

fn type_error(property: &str, value: &Value, want: &str) -> String {
    format!(
        "validating root: validating /properties/{property}: type: {} has type {:?}, want {want:?}",
        go_render(value),
        json_kind(value)
    )
}

/// Entry point of the missing-properties error: the root instance reports
/// the failure with a `validating root: ` prefix.
fn missing_properties(properties: &[&str]) -> String {
    let quoted: Vec<String> = properties
        .iter()
        .map(|property| format!("\"{property}\""))
        .collect();
    format!(
        "validating root: required: missing properties: [{}]",
        quoted.join(" ")
    )
}

/// jsonschema-go's instance kind names: integral JSON numbers are
/// "integer", fractional ones "number".
fn json_kind(value: &Value) -> &'static str {
    match value {
        Value::Null => "null",
        Value::Bool(_) => "bool",
        Value::Number(number) => {
            if number.as_f64().is_some_and(|f| f.fract() == 0.0) {
                "integer"
            } else {
                "number"
            }
        }
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

/// Go `InvalidClaim` json tags with `invalid_claims,omitempty` on the
/// summary field: the key vanishes entirely when the list is empty.
fn invalid_claim_entry(invalid: &InvalidClaimSerde) -> OrderedJson {
    OrderedJson::object(vec![
        ("path", OrderedJson::string(&invalid.path)),
        ("error", OrderedJson::string(&invalid.error)),
    ])
}

/// `{schema_version, ...IndexSummary}` as the Go anonymous struct renders
/// it: schema_version first, then the IndexSummary fields in struct order.
/// `catalog` is `null` when the summary has none (Go nil slice) and `[]`
/// when empty-but-present.
fn summary_output_json(schema_version: i64, summary: &IndexSummary) -> OrderedJson {
    let mut entries = vec![
        ("schema_version", OrderedJson::Int(schema_version)),
        ("workspace", OrderedJson::string(&summary.workspace)),
        ("approved", OrderedJson::Int(summary.approved)),
        ("draft", OrderedJson::Int(summary.draft)),
        ("invalid", OrderedJson::Int(summary.invalid)),
        ("invalid_count", OrderedJson::Int(summary.invalid_count)),
    ];
    // Go `invalid_claims,omitempty`: the key vanishes when empty.
    if !summary.invalid_claims.is_empty() {
        entries.push((
            "invalid_claims",
            OrderedJson::Array(
                summary
                    .invalid_claims
                    .iter()
                    .map(invalid_claim_entry)
                    .collect(),
            ),
        ));
    }
    entries.push(("legacy", OrderedJson::Int(summary.legacy)));
    entries.push(("rebuild_state", OrderedJson::string(&summary.rebuild_state)));
    entries.push(("manifest_digest", OrderedJson::string(&summary.manifest_digest)));
    entries.push(("rebuilt_at", OrderedJson::string(&summary.rebuilt_at)));
    entries.push(("embedding", embedding_summary_json(&summary.embedding)));
    entries.push((
        "catalog",
        match &summary.catalog {
            None => OrderedJson::Null,
            Some(catalog) => {
                OrderedJson::Array(catalog.iter().map(catalog_claim_json).collect())
            }
        },
    ));
    OrderedJson::object(entries)
}

/// Go `EmbeddingSummary` json tags: `model` and `degraded_reason` omitempty,
/// `strategy`/`indexed`/`eligible` always present.
fn embedding_summary_json(summary: &crate::index::EmbeddingSummary) -> OrderedJson {
    let mut entries = vec![
        ("strategy", OrderedJson::string(&summary.strategy)),
    ];
    if !summary.model.is_empty() {
        entries.push(("model", OrderedJson::string(&summary.model)));
    }
    entries.push(("indexed", OrderedJson::Int(summary.indexed)));
    entries.push(("eligible", OrderedJson::Int(summary.eligible)));
    if !summary.degraded.is_empty() {
        entries.push(("degraded_reason", OrderedJson::string(&summary.degraded)));
    }
    OrderedJson::object(entries)
}

/// Go `CatalogClaim` json tags: `stale_after` omitempty.
fn catalog_claim_json(claim: &crate::index::CatalogClaim) -> OrderedJson {
    let mut entries = vec![
        ("id", OrderedJson::string(&claim.id)),
        ("title", OrderedJson::string(&claim.title)),
        ("tier", OrderedJson::string(&claim.tier)),
    ];
    if !claim.stale_after.is_empty() {
        entries.push(("stale_after", OrderedJson::string(&claim.stale_after)));
    }
    OrderedJson::object(entries)
}

/// Go `TrustedQueryResponse` wire JSON in struct field order: nil slices as
/// null (`claims`, `gaps`, `promotion_candidates`), empty non-nil as `[]`
/// (`conflicts`, `index`, `scopes.includes`), omitempty strings dropped.
fn ask_response_json(response: &TrustedQueryResponse) -> OrderedJson {
    OrderedJson::object(vec![
        ("schema_version", OrderedJson::Int(response.schema_version)),
        ("status", OrderedJson::string(&response.status)),
        ("query", OrderedJson::string(&response.query)),
        (
            "scopes",
            OrderedJson::object(vec![
                ("primary", OrderedJson::string(&response.scopes.primary)),
                (
                    "includes",
                    OrderedJson::Array(
                        response
                            .scopes
                            .includes
                            .iter()
                            .map(OrderedJson::string)
                            .collect(),
                    ),
                ),
            ]),
        ),
        // Go renders nil slices as null and empty non-nil slices as [];
        // the Option encodes exactly that distinction.
        (
            "claims",
            match &response.claims {
                None => OrderedJson::Null,
                Some(claims) => OrderedJson::Array(claims.iter().map(query_claim_json).collect()),
            },
        ),
        (
            "conflicts",
            OrderedJson::Array(
                response
                    .conflicts
                    .iter()
                    .map(|conflict| {
                        OrderedJson::object(vec![
                            ("workspace", OrderedJson::string(&conflict.workspace)),
                            (
                                "claim_ids",
                                OrderedJson::Array(
                                    conflict
                                        .claim_ids
                                        .iter()
                                        .map(OrderedJson::string)
                                        .collect(),
                                ),
                            ),
                        ])
                    })
                    .collect(),
            ),
        ),
        (
            "gaps",
            match &response.gaps {
                None => OrderedJson::Null,
                Some(gaps) => OrderedJson::Array(
                    gaps.iter()
                        .map(|gap| {
                            OrderedJson::object(vec![
                                ("workspace", OrderedJson::string(&gap.workspace)),
                                ("reason", OrderedJson::string(&gap.reason)),
                            ])
                        })
                        .collect(),
                ),
            },
        ),
        (
            "promotion_candidates",
            match &response.promotion_candidates {
                None => OrderedJson::Null,
                Some(candidates) => {
                    OrderedJson::Array(candidates.iter().map(query_claim_json).collect())
                }
            },
        ),
        (
            "index",
            OrderedJson::Array(
                response
                    .index
                    .iter()
                    .map(|metadata| {
                        OrderedJson::object(vec![
                            ("workspace", OrderedJson::string(&metadata.workspace)),
                            ("fresh", OrderedJson::Bool(metadata.fresh)),
                        ])
                    })
                    .collect(),
            ),
        ),
    ])
}

/// Go `QueryClaim` json tags: `description`/`stale_after` omitempty strings,
/// `sources`/`contradicts` omitempty slices.
fn query_claim_json(claim: &QueryClaim) -> OrderedJson {
    let mut entries = vec![
        ("workspace", OrderedJson::string(&claim.workspace)),
        ("id", OrderedJson::string(&claim.id)),
        ("path", OrderedJson::string(&claim.path)),
        ("tier", OrderedJson::string(&claim.tier)),
        ("type", OrderedJson::string(&claim.claim_type)),
        ("status", OrderedJson::string(&claim.status)),
        ("title", OrderedJson::string(&claim.title)),
    ];
    if !claim.description.is_empty() {
        entries.push(("description", OrderedJson::string(&claim.description)));
    }
    if !claim.stale_after.is_empty() {
        entries.push(("stale_after", OrderedJson::string(&claim.stale_after)));
    }
    entries.push(("score", OrderedJson::Float(claim.score)));
    if let Some(sources) = &claim.sources {
        entries.push((
            "sources",
            OrderedJson::Array(sources.iter().map(query_claim_source_json).collect()),
        ));
    }
    if let Some(contradicts) = &claim.contradicts {
        entries.push((
            "contradicts",
            OrderedJson::Array(contradicts.iter().map(query_contradiction_json).collect()),
        ));
    }
    OrderedJson::object(entries)
}

/// Go `ClaimSource` json tags on the query projection: `title`/`spans`
/// omitempty.
fn query_claim_source_json(source: &QueryClaimSource) -> OrderedJson {
    let mut entries = vec![
        ("id", OrderedJson::string(&source.id)),
        ("resource", OrderedJson::string(&source.resource)),
    ];
    if !source.title.is_empty() {
        entries.push(("title", OrderedJson::string(&source.title)));
    }
    entries.push(("digest", OrderedJson::string(&source.digest)));
    if let Some(spans) = &source.spans {
        entries.push((
            "spans",
            OrderedJson::Array(spans.iter().map(query_evidence_span_json).collect()),
        ));
    }
    OrderedJson::object(entries)
}

fn query_evidence_span_json(span: &QueryEvidenceSpan) -> OrderedJson {
    OrderedJson::object(vec![
        ("evidence_id", OrderedJson::string(&span.evidence_id)),
        ("start_line", OrderedJson::Int(span.start_line)),
        ("end_line", OrderedJson::Int(span.end_line)),
        ("digest", OrderedJson::string(&span.digest)),
    ])
}

fn query_contradiction_json(contradiction: &QueryContradiction) -> OrderedJson {
    OrderedJson::object(vec![
        ("claim_id", OrderedJson::string(&contradiction.claim_id)),
        ("heuristic", OrderedJson::string(&contradiction.heuristic)),
    ])
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
        tools_call_named_frame(id, "evidence_capture", arguments)
    }

    fn tools_call_named_frame(id: i64, name: &str, arguments: &str) -> String {
        format!(
            r#"{{"jsonrpc":"2.0","id":{id},"method":"tools/call","params":{{"name":"{name}","arguments":{arguments}}}}}"#
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
        assert_eq!(tools.len(), 6);
        assert_eq!(
            tools.iter().map(|tool| tool.name.as_str()).collect::<Vec<_>>(),
            vec![
                "claim_draft",
                "evidence_capture",
                "memory_ask",
                "memory_reindex",
                "memory_status",
                "workspace_current",
            ]
        );
        let capture = tools
            .iter()
            .find(|tool| tool.name == "evidence_capture")
            .expect("evidence_capture tool");
        assert_eq!(
            capture.description.as_deref(),
            Some("Snapshot a local source file into an immutable evidence record.")
        );
        // Byte-exact jsonschema-go output: struct field-order properties,
        // required in field order, additionalProperties false.
        assert_eq!(
            capture.input_schema.compact(),
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
            "validating \"arguments\": validating root: required: missing properties: [\"file\" \"origin\"]"
        );
        let result = reg
            .call_tool("evidence_capture", Some(&args(r#"{"file":"x"}"#)))
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: required: missing properties: [\"origin\"]"
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
            "validating \"arguments\": validating root: unexpected additional properties [\"extra\"]"
        );

        // Wrong-typed property fails closed as isError (integral JSON
        // numbers are "integer" in jsonschema-go).
        let result = reg
            .call_tool(
                "evidence_capture",
                Some(&args(r#"{"file":3,"origin":"o"}"#)),
            )
            .unwrap();
        assert!(result.is_error);
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: validating /properties/file: type: 3 has type \"integer\", want \"string\""
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

    // --- W2.T2: memory tools (ports TestToolSurface, TestBounds,
    // TestMemoryAskTemporalFiltering, and
    // TestMemoryEmbeddingOptInAndLexicalFallback) ---

    /// Ports `testOptions`: the fixture draft is approved and the derived
    /// index rebuilt so the memory tools exercise real trusted content.
    fn memory_fixture(name: &str) -> Fixture {
        let fix = fixture(name);
        ClaimStore::new(fix.paths.clone())
            .approve("research", &fix.claim_id)
            .unwrap();
        IndexStore::new(fix.paths.clone())
            .rebuild("research")
            .unwrap();
        fix
    }

    /// Writer that funnels stderr diagnostics into a shared buffer so tests
    /// can assert the audit log without leaking content.
    #[derive(Clone, Default)]
    struct SharedBuffer(std::sync::Arc<std::sync::Mutex<Vec<u8>>>);

    impl std::io::Write for SharedBuffer {
        fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
            self.0.lock().unwrap().extend_from_slice(buf);
            Ok(buf.len())
        }

        fn flush(&mut self) -> std::io::Result<()> {
            Ok(())
        }
    }

    fn buffered_registry(fixture: &Fixture) -> (ZbrainRegistry, SharedBuffer) {
        let buffer = SharedBuffer::default();
        let stderr = SafeStderr::new(Box::new(buffer.clone()));
        let reg = ZbrainRegistry {
            paths: fixture.paths.clone(),
            clock: Box::new(FixedClock::new(fixture.clock.now())),
            stderr,
        };
        (reg, buffer)
    }

    #[test]
    fn w2t2_tool_schemas_match_go_wire_capture() {
        let fix = memory_fixture("w2t2-schemas");
        let tools = registry(&fix).tools();
        let schema = |name: &str| -> String {
            tools
                .iter()
                .find(|tool| tool.name == name)
                .unwrap_or_else(|| panic!("{name} listed"))
                .input_schema
                .compact()
        };
        // Byte-identical to `tools/list` captures from
        // `go run ./cmd/zbrain mcp serve`.
        assert_eq!(
            schema("workspace_current"),
            r#"{"type":"object","additionalProperties":false}"#
        );
        assert_eq!(
            schema("memory_ask"),
            r#"{"type":"object","properties":{"query":{"type":"string","description":"the trusted-memory query"},"workspace":{"type":"string","description":"workspace to query; defaults to the current workspace"},"include":{"type":["null","array"],"items":{"type":"string"},"description":"read-only secondary workspace to include"},"embedding":{"type":"boolean","description":"enable local hybrid embedding retrieval; defaults to false"},"after":{"type":"string","description":"filter claims verified/created at or after RFC3339 timestamp"},"before":{"type":"string","description":"filter claims verified/created at or before RFC3339 timestamp"},"as_of":{"type":"string","description":"reconstruct active memory state as of RFC3339 timestamp"}},"required":["query"],"additionalProperties":false}"#
        );
        assert_eq!(
            schema("memory_status"),
            r#"{"type":"object","properties":{"workspace":{"type":"string","description":"workspace to inspect; defaults to the current workspace"}},"additionalProperties":false}"#
        );
        assert_eq!(
            schema("memory_reindex"),
            r#"{"type":"object","properties":{"workspace":{"type":"string","description":"workspace whose derived index to rebuild; defaults to the current workspace"},"embedding":{"type":"boolean","description":"enable local loopback embedding; defaults to false"}},"additionalProperties":false}"#
        );
        assert_eq!(
            schema("claim_draft"),
            r#"{"type":"object","properties":{"workspace":{"type":"string","description":"target workspace; defaults to the current workspace"},"tier":{"type":"string","description":"claim tier"},"title":{"type":"string","description":"claim title"},"basis":{"type":"string","description":"owner, evidence, or derived"},"evidence":{"type":["null","array"],"items":{"type":"string"},"description":"evidence IDs to bind"},"support":{"type":["null","array"],"items":{"type":"string"},"description":"supporting claim IDs"},"conflicts_with":{"type":["null","array"],"items":{"type":"string"},"description":"conflicting claim IDs"},"body":{"type":"string","description":"claim body"}},"required":["tier","title","basis","body"],"additionalProperties":false}"#
        );
    }

    #[test]
    fn workspace_current_resolves_default_workspace() {
        let fix = memory_fixture("workspace-current");
        let reg = registry(&fix);
        let result = reg.call_tool("workspace_current", None).unwrap();
        assert!(!result.is_error);
        let text = result.content[0].text.clone();
        assert_key_order(
            &text,
            &[
                "schema_version",
                "project_root",
                "workspace",
                "secondary_workspaces",
            ],
        );
        let parsed: Value = serde_json::from_str(&text).unwrap();
        assert_eq!(parsed["schema_version"], 1);
        assert_eq!(parsed["workspace"], "research");
        assert_eq!(parsed["secondary_workspaces"], serde_json::json!([]));
        // structuredContent mirrors the text payload.
        let structured = serde_json::to_value(result.structured_content.unwrap()).unwrap();
        assert_eq!(structured, parsed);
    }

    #[test]
    fn memory_ask_ready_and_gap_wire_bytes() {
        let fix = memory_fixture("ask-ready");
        let responses = run_session_raw(
            registry(&fix),
            &format!(
                "{}\n{}\n{}\n{}\n",
                initialize_frame(1),
                tools_call_named_frame(2, "memory_ask", r#"{"query":"Resource Claim"}"#),
                tools_call_named_frame(3, "memory_ask", r#"{"query":"zzz-unmatchable"}"#),
                tools_call_named_frame(4, "memory_ask", r#"{"query":"Resource Claim","workspace":"nonexistent"}"#),
            ),
        );
        let ready = &responses[1];
        let ready_value: Value = serde_json::from_str(ready).unwrap();
        let ready_text = ready_value["result"]["content"][0]["text"].as_str().unwrap();
        assert_key_order(
            ready_text,
            &[
                "schema_version",
                "status",
                "query",
                "scopes",
                "primary",
                "includes",
                "claims",
                "workspace",
                "id",
                "path",
                "tier",
                "type",
                "status",
                "title",
                "score",
                "conflicts",
                "gaps",
                "promotion_candidates",
                "index",
            ],
        );
        let parsed: Value = serde_json::from_str(ready_text).unwrap();
        assert_eq!(parsed["claims"][0]["id"], fix.claim_id);
        assert_eq!(parsed["claims"][0]["status"], "approved");
        assert_eq!(parsed["claims"][0]["type"], "zbrain.claim");
        assert!(parsed["claims"][0]["score"].is_number());
        assert_eq!(parsed["conflicts"], serde_json::json!([]));
        assert_eq!(parsed["gaps"], Value::Null);
        assert_eq!(parsed["promotion_candidates"], Value::Null);
        assert_eq!(
            parsed["index"],
            serde_json::json!([{ "workspace": "research", "fresh": true }])
        );
        // structuredContent is the compact rendering of the same payload.
        let structured = &ready_value["result"]["structuredContent"];
        let text_value: Value = serde_json::from_str(ready_text).unwrap();
        assert_eq!(&text_value, structured);

        // Gap response: byte-identical to the Go gateway capture
        // (`memory_ask {"query":"zzz-unmatchable"}` on an equivalent
        // workspace).
        let gap = &responses[2];
        let gap_value: Value = serde_json::from_str(gap).unwrap();
        let gap_text = gap_value["result"]["content"][0]["text"].as_str().unwrap();
        assert_eq!(
            gap_text,
            concat!(
                "{\n",
                "  \"schema_version\": 1,\n",
                "  \"status\": \"gap\",\n",
                "  \"query\": \"zzz-unmatchable\",\n",
                "  \"scopes\": {\n",
                "    \"primary\": \"research\",\n",
                "    \"includes\": []\n",
                "  },\n",
                "  \"claims\": null,\n",
                "  \"conflicts\": [],\n",
                "  \"gaps\": [\n",
                "    {\n",
                "      \"workspace\": \"research\",\n",
                "      \"reason\": \"no approved claims matched the query in resolved scopes\"\n",
                "    }\n",
                "  ],\n",
                "  \"promotion_candidates\": null,\n",
                "  \"index\": [\n",
                "    {\n",
                "      \"workspace\": \"research\",\n",
                "      \"fresh\": true\n",
                "    }\n",
                "  ]\n",
                "}"
            )
        );

        // Nonexistent workspace fails closed as isError.
        let missing: Value = serde_json::from_str(&responses[3]).unwrap();
        assert_eq!(missing["result"]["isError"], true);
        assert_eq!(
            missing["result"]["content"][0]["text"],
            "workspace \"nonexistent\" does not exist"
        );
    }

    #[test]
    fn memory_ask_schema_and_guard_errors_match_go_wire_capture() {
        let fix = memory_fixture("ask-errors");
        let reg = registry(&fix);
        let args = |json_text: &str| serde_json::from_str::<Value>(json_text).unwrap();
        let result = reg.call_tool("memory_ask", Some(&args("{}"))).unwrap();
        assert!(result.is_error);
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: required: missing properties: [\"query\"]"
        );
        let result = reg
            .call_tool("memory_ask", Some(&args(r#"{"query":""}"#)))
            .unwrap();
        assert_eq!(result.content[0].text, "query is required");
        let result = reg
            .call_tool("memory_ask", Some(&args(r#"{"query":"   "}"#)))
            .unwrap();
        assert_eq!(result.content[0].text, "query is required");
        let result = reg
            .call_tool("memory_ask", Some(&args(r#"{"query":"x","extra":1}"#)))
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: unexpected additional properties [\"extra\"]"
        );
        let result = reg
            .call_tool("memory_ask", Some(&args(r#"{"query":3}"#)))
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: validating /properties/query: type: 3 has type \"integer\", want \"string\""
        );
        let result = reg
            .call_tool("memory_ask", Some(&args(r#"{"query":3.5}"#)))
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: validating /properties/query: type: 3.5 has type \"number\", want \"string\""
        );
        let result = reg
            .call_tool("memory_ask", Some(&args(r#"{"query":"x","embedding":"notabool"}"#)))
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: validating /properties/embedding: type: notabool has type \"string\", want \"boolean\""
        );
        let result = reg
            .call_tool("memory_ask", Some(&args(r#"{"query":"x","include":"notanarray"}"#)))
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: validating /properties/include: type: notanarray has type \"string\", want one of \"null, array\""
        );
        let result = reg
            .call_tool("memory_ask", Some(&args(r#"{"query":"x","include":[1]}"#)))
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: validating /properties/include/items: type: 1 has type \"integer\", want \"string\""
        );
        let result = reg
            .call_tool("memory_ask", Some(&args(r#"{"query":"x","include":null}"#)))
            .unwrap();
        assert!(!result.is_error, "null include passes schema");
        let result = reg
            .call_tool(
                "memory_ask",
                Some(&args(r#"{"query":"x","after":"bad-timestamp"}"#)),
            )
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "invalid after timestamp \"bad-timestamp\": parsing time \"bad-timestamp\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"bad-timestamp\" as \"2006\""
        );
        let result = reg
            .call_tool(
                "memory_ask",
                Some(&args(r#"{"query":"x","as_of":"2026-13-01T00:00:00Z"}"#)),
            )
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "invalid as_of timestamp \"2026-13-01T00:00:00Z\": parsing time \"2026-13-01T00:00:00Z\": month out of range"
        );
        let result = reg
            .call_tool(
                "memory_ask",
                Some(&args(r#"{"query":"x","before":"2026-08-10T00:00:00"}"#)),
            )
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "invalid before timestamp \"2026-08-10T00:00:00\": parsing time \"2026-08-10T00:00:00\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"\" as \"Z07:00\""
        );
        let result = reg
            .call_tool(
                "memory_ask",
                Some(&args(r#"{"query":"x","include":["elsewhere"]}"#)),
            )
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "workspace \"elsewhere\" does not exist"
        );
        // Non-object arguments fail closed as isError with the Go unmarshal
        // error shape.
        let result = reg
            .call_tool("memory_ask", Some(&args(r#""bogus""#)))
            .unwrap();
        assert!(result.is_error);
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": unmarshaling arguments: json: cannot unmarshal \"\\\"bogus\\\"\" into Go value of type map[string]interface {}"
        );
    }

    #[test]
    fn memory_ask_temporal_filtering() {
        let fix = memory_fixture("temporal");
        let store = ClaimStore::new(fix.paths.clone());
        let early = store
            .write_draft(
                "research",
                Claim {
                    claim_type: OKF_CLAIM_TYPE.to_string(),
                    id: "clm_11111111111111111111111111111111".to_string(),
                    tier: "projects".to_string(),
                    title: "MCP Temporal Early".to_string(),
                    basis: crate::claims::CLAIM_BASIS_OWNER.to_string(),
                    created_at: "2026-08-01T09:00:00Z".to_string(),
                    created_by: "owner".to_string(),
                    body: "mcp temporal query marker\n".to_string(),
                    ..Default::default()
                },
            )
            .unwrap();
        let early_clock = FixedClock::new(Utc.with_ymd_and_hms(2026, 8, 1, 10, 0, 0).unwrap());
        ClaimStore::with_clock(fix.paths.clone(), std::sync::Arc::new(early_clock))
            .approve("research", &early.id)
            .unwrap();
        let late = store
            .write_draft(
                "research",
                Claim {
                    claim_type: OKF_CLAIM_TYPE.to_string(),
                    id: "clm_22222222222222222222222222222222".to_string(),
                    tier: "projects".to_string(),
                    title: "MCP Temporal Late".to_string(),
                    basis: crate::claims::CLAIM_BASIS_OWNER.to_string(),
                    created_at: "2026-08-28T09:00:00Z".to_string(),
                    created_by: "owner".to_string(),
                    body: "mcp temporal query marker\n".to_string(),
                    ..Default::default()
                },
            )
            .unwrap();
        let late_clock = FixedClock::new(Utc.with_ymd_and_hms(2026, 8, 28, 10, 0, 0).unwrap());
        ClaimStore::with_clock(fix.paths.clone(), std::sync::Arc::new(late_clock))
            .approve("research", &late.id)
            .unwrap();
        IndexStore::new(fix.paths.clone())
            .rebuild("research")
            .unwrap();

        let reg = registry(&fix);
        let args = |json_text: &str| serde_json::from_str::<Value>(json_text).unwrap();
        let decode_claims = |result: &CallToolResult| -> (String, Vec<String>) {
            let parsed: Value = serde_json::from_str(&result.content[0].text).unwrap();
            (
                parsed["status"].as_str().unwrap().to_string(),
                parsed["claims"]
                    .as_array()
                    .map(|claims| {
                        claims
                            .iter()
                            .map(|claim| claim["id"].as_str().unwrap().to_string())
                            .collect()
                    })
                    .unwrap_or_default(),
            )
        };
        let after = reg
            .call_tool(
                "memory_ask",
                Some(&args(
                    r#"{"query":"mcp temporal","after":"2026-08-10T00:00:00Z"}"#,
                )),
            )
            .unwrap();
        assert!(!after.is_error);
        let (status, ids) = decode_claims(&after);
        assert_eq!(status, "ready");
        assert_eq!(ids, vec![late.id]);

        let before = reg
            .call_tool(
                "memory_ask",
                Some(&args(
                    r#"{"query":"mcp temporal","before":"2026-08-10T00:00:00Z"}"#,
                )),
            )
            .unwrap();
        assert!(!before.is_error);
        let (status, ids) = decode_claims(&before);
        assert_eq!(status, "ready");
        assert_eq!(ids, vec![early.id]);
    }

    #[test]
    fn memory_status_shapes_match_go_wire_capture() {
        let fix = memory_fixture("status-clean");
        let reg = registry(&fix);
        let result = reg.call_tool("memory_status", None).unwrap();
        assert!(!result.is_error);
        let text = result.content[0].text.clone();
        assert_key_order(
            &text,
            &[
                "schema_version",
                "workspace",
                "approved",
                "draft",
                "invalid",
                "invalid_count",
                // invalid_claims is omitempty: the clean state omits it.
                "legacy",
                "rebuild_state",
                "manifest_digest",
                "rebuilt_at",
                "embedding",
                "strategy",
                "indexed",
                "eligible",
                "degraded_reason",
                "catalog",
            ],
        );
        let parsed: Value = serde_json::from_str(&text).unwrap();
        assert_eq!(parsed["schema_version"], 2);
        assert_eq!(parsed["workspace"], "research");
        // ScanWorkspaceForTrust counts every valid canonical claim; the
        // catalog only carries approved ones.
        assert_eq!(parsed["approved"], 1);
        assert_eq!(parsed["rebuild_state"], "");
        assert_eq!(parsed["manifest_digest"], "");
        assert_eq!(parsed["rebuilt_at"], "");
        assert_eq!(
            parsed["embedding"],
            serde_json::json!({
                "strategy": "lexical",
                "indexed": 0,
                "eligible": 0,
                "degraded_reason": "embeddings not configured",
            })
        );
        assert_eq!(
            parsed["catalog"],
            serde_json::json!([
                { "id": fix.claim_id, "title": "Resource Claim", "tier": "projects" }
            ])
        );
        // Byte-identity: dirty state with the fresh-scan invalid claim
        // (Go capture `memory_status` right after `claim_draft`).
        let _dirty = ClaimStore::new(fix.paths.clone())
            .write_draft(
                "research",
                Claim {
                    claim_type: OKF_CLAIM_TYPE.to_string(),
                    id: new_claim_id().unwrap(),
                    tier: "projects".to_string(),
                    title: "Dirtying Draft".to_string(),
                    basis: crate::claims::CLAIM_BASIS_OWNER.to_string(),
                    created_at: rfc3339(fix.clock.now()),
                    created_by: "test".to_string(),
                    body: "dirty body".to_string(),
                    ..Default::default()
                },
            )
            .unwrap();
        let result = reg.call_tool("memory_status", None).unwrap();
        let text = result.content[0].text.clone();
        assert_eq!(
            text,
            format!(
                concat!(
                    "{{\n",
                    "  \"schema_version\": 2,\n",
                    "  \"workspace\": \"research\",\n",
                    "  \"approved\": 2,\n",
                    "  \"draft\": 0,\n",
                    "  \"invalid\": 0,\n",
                    "  \"invalid_count\": 0,\n",
                    "  \"invalid_claims\": [\n",
                    "    {{\n",
                    "      \"path\": \"\",\n",
                    "      \"error\": \"workspace \\\"research\\\" index is dirty; run zbrain reindex\"\n",
                    "    }}\n",
                    "  ],\n",
                    "  \"legacy\": 0,\n",
                    "  \"rebuild_state\": \"rejected\",\n",
                    "  \"manifest_digest\": \"\",\n",
                    "  \"rebuilt_at\": \"\",\n",
                    "  \"embedding\": {{\n",
                    "    \"strategy\": \"lexical\",\n",
                    "    \"indexed\": 0,\n",
                    "    \"eligible\": 0,\n",
                    "    \"degraded_reason\": \"embeddings not configured\"\n",
                    "  }},\n",
                    "  \"catalog\": [\n",
                    "    {{\n",
                    "      \"id\": \"{claim_id}\",\n",
                    "      \"title\": \"Resource Claim\",\n",
                    "      \"tier\": \"projects\"\n",
                    "    }}\n",
                    "  ]\n",
                    "}}"
                ),
                claim_id = fix.claim_id
            )
        );
    }

    #[test]
    fn memory_reindex_rebuilds_and_reports_summary() {
        let fix = memory_fixture("reindex");
        ClaimStore::new(fix.paths.clone())
            .write_draft(
                "research",
                Claim {
                    claim_type: OKF_CLAIM_TYPE.to_string(),
                    id: new_claim_id().unwrap(),
                    tier: "projects".to_string(),
                    title: "Second Claim".to_string(),
                    basis: crate::claims::CLAIM_BASIS_OWNER.to_string(),
                    created_at: rfc3339(fix.clock.now()),
                    created_by: "test".to_string(),
                    body: "second body".to_string(),
                    ..Default::default()
                },
            )
            .unwrap();
        let reg = registry(&fix);
        let result = reg.call_tool("memory_reindex", None).unwrap();
        assert!(!result.is_error);
        let text = result.content[0].text.clone();
        assert_key_order(
            &text,
            &[
                "schema_version",
                "workspace",
                "approved",
                "draft",
                "invalid",
                "invalid_count",
                "legacy",
                "rebuild_state",
                "manifest_digest",
                "rebuilt_at",
                "embedding",
                "catalog",
            ],
        );
        let parsed: Value = serde_json::from_str(&text).unwrap();
        assert_eq!(parsed["schema_version"], 1);
        assert_eq!(parsed["rebuild_state"], "clean");
        assert!(!parsed["manifest_digest"].as_str().unwrap().is_empty());
        // Only the approved claim counts as approved; the draft is counted
        // separately (matches the Go capture).
        assert_eq!(parsed["approved"], 1);
        assert_eq!(parsed["draft"], 1);
        // A non-embedding rebuild leaves no embedding summary behind
        // (strategy stays the zero value, like the Go capture).
        assert_eq!(
            parsed["embedding"],
            serde_json::json!({ "strategy": "", "indexed": 0, "eligible": 0 })
        );
    }

    #[test]
    fn memory_embedding_opt_in_and_lexical_fallback() {
        let fix = memory_fixture("embedding");
        let reg = registry(&fix);
        let args = |json_text: &str| serde_json::from_str::<Value>(json_text).unwrap();

        let lexical = reg
            .call_tool("memory_ask", Some(&args(r#"{"query":"embedding-only-term"}"#)))
            .unwrap();
        let parsed: Value = serde_json::from_str(&lexical.content[0].text).unwrap();
        assert_eq!(parsed["status"], "gap");
        assert!(
            !fix.paths.indexes_dir.join("research.embeddings.sqlite").exists(),
            "embedding sidecar exists before opt-in"
        );

        let reindexed = reg
            .call_tool("memory_reindex", Some(&args(r#"{"embedding":true}"#)))
            .unwrap();
        assert!(!reindexed.is_error);
        let summary: Value = serde_json::from_str(&reindexed.content[0].text).unwrap();
        assert_eq!(
            summary["embedding"],
            serde_json::json!({
                "strategy": "loopback",
                "model": "zbrain/loopback-v1",
                "indexed": 1,
                "eligible": 1,
            })
        );

        let status = reg.call_tool("memory_status", None).unwrap();
        let status_value: Value = serde_json::from_str(&status.content[0].text).unwrap();
        assert_eq!(status_value["embedding"]["strategy"], "loopback");
        assert_eq!(status_value["embedding"]["indexed"], 1);
        assert_eq!(status_value["embedding"]["eligible"], 1);

        let hybrid = reg
            .call_tool(
                "memory_ask",
                Some(&args(r#"{"query":"embedding-only-term","embedding":true}"#)),
            )
            .unwrap();
        let hybrid_value: Value = serde_json::from_str(&hybrid.content[0].text).unwrap();
        assert_eq!(
            hybrid_value["claims"].as_array().map(Vec::len),
            Some(1),
            "want one vector-retrieved claim"
        );

        let lexical_again = reg
            .call_tool("memory_ask", Some(&args(r#"{"query":"embedding-only-term"}"#)))
            .unwrap();
        let again_value: Value = serde_json::from_str(&lexical_again.content[0].text).unwrap();
        assert_eq!(again_value["status"], "gap");
    }

    #[test]
    fn claim_draft_tool_creates_promotion_candidate() {
        let fix = memory_fixture("claim-draft");
        let reg = registry(&fix);
        let args = |json_text: &str| serde_json::from_str::<Value>(json_text).unwrap();
        let result = reg
            .call_tool(
                "claim_draft",
                Some(&args(
                    r#"{"tier":"projects","title":"Draft via MCP","basis":"evidence","body":"draft body"}"#,
                )),
            )
            .unwrap();
        assert!(!result.is_error);
        let text = result.content[0].text.clone();
        assert_key_order(
            &text,
            &["schema_version", "workspace", "id", "status", "path"],
        );
        let parsed: Value = serde_json::from_str(&text).unwrap();
        assert_eq!(parsed["schema_version"], 1);
        assert_eq!(parsed["workspace"], "research");
        assert!(parsed["id"].as_str().unwrap().starts_with("clm_"));
        assert_eq!(parsed["status"], "draft");
        assert_eq!(
            parsed["path"],
            format!("projects/{}.md", parsed["id"].as_str().unwrap())
        );
        let claim = ClaimStore::new(fix.paths.clone())
            .read("research", parsed["id"].as_str().unwrap())
            .unwrap();
        assert_eq!(claim.created_by, "owner:mcp");
        assert_eq!(claim.created_at, rfc3339(fix.clock.now()));

        // Draft creation dirties the index.
        let status = reg.call_tool("memory_status", None).unwrap();
        let status_value: Value = serde_json::from_str(&status.content[0].text).unwrap();
        assert_eq!(status_value["rebuild_state"], "rejected");
        assert_eq!(
            status_value["invalid_claims"][0]["error"],
            "workspace \"research\" index is dirty; run zbrain reindex"
        );

        // Guard and schema failures match the Go capture.
        let result = reg
            .call_tool(
                "claim_draft",
                Some(&args(r#"{"tier":"projects","title":"t","basis":"weird","body":"b"}"#)),
            )
            .unwrap();
        assert_eq!(result.content[0].text, "claim basis \"weird\" is not supported");
        let result = reg.call_tool("claim_draft", Some(&args("{}"))).unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: required: missing properties: [\"tier\" \"title\" \"basis\" \"body\"]"
        );
        let result = reg
            .call_tool(
                "claim_draft",
                Some(&args(
                    r#"{"tier":"projects","title":"t","basis":"owner","body":"b","extra":1}"#,
                )),
            )
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: unexpected additional properties [\"extra\"]"
        );
        let result = reg
            .call_tool(
                "claim_draft",
                Some(&args(r#"{"tier":3,"title":"t","basis":"owner","body":"b"}"#)),
            )
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: validating /properties/tier: type: 3 has type \"integer\", want \"string\""
        );
        let result = reg
            .call_tool(
                "claim_draft",
                Some(&args(r#"{"tier":"projects","title":"t","basis":"owner","body":"b","evidence":"notarray"}"#)),
            )
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "validating \"arguments\": validating root: validating /properties/evidence: type: notarray has type \"string\", want one of \"null, array\""
        );
        let result = reg
            .call_tool(
                "claim_draft",
                Some(&args(
                    r#"{"tier":"projects","title":"t","basis":"owner","body":"b","workspace":"nonexistent"}"#,
                )),
            )
            .unwrap();
        assert_eq!(
            result.content[0].text,
            "workspace \"nonexistent\" does not exist"
        );
    }

    #[test]
    fn oversized_inputs_map_to_32602() {
        let fix = memory_fixture("oversized-w2t2");
        let reg = registry(&fix);
        let oversized = "a".repeat((1 << 20) + 1024);
        let arguments = serde_json::json!({ "query": oversized });
        let error = reg.call_tool("memory_ask", Some(&arguments)).unwrap_err();
        assert_eq!(error.to_wire("tools/call").code, -32602);
        assert_eq!(error.to_wire("tools/call").message, "input exceeds 1MB limit");
        let arguments = serde_json::json!({
            "tier": "projects",
            "title": "huge",
            "basis": "owner",
            "body": "b".repeat((1 << 20) + 10),
        });
        let error = reg.call_tool("claim_draft", Some(&arguments)).unwrap_err();
        assert_eq!(error.to_wire("tools/call").code, -32602);
    }

    #[test]
    fn audit_log_carries_tool_workspace_and_duration_without_body() {
        let fix = memory_fixture("audit");
        let (reg, buffer) = buffered_registry(&fix);
        let result = reg
            .call_tool(
                "memory_ask",
                Some(&serde_json::json!({ "query": "Resource Claim" })),
            )
            .unwrap();
        assert!(!result.is_error);
        let logged = String::from_utf8(buffer.0.lock().unwrap().clone()).unwrap();
        assert!(logged.contains("tool=memory_ask"), "log: {logged}");
        assert!(logged.contains("workspace=current"), "log: {logged}");
        assert!(logged.contains("duration="), "log: {logged}");
        assert!(!logged.contains("Resource Claim"), "audit log leaked query");
    }
}
