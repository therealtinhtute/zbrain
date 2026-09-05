//! MCP server dispatch: session state, protocol gates, and the method
//! registry. Ports the Go SDK's server request handling
//! (`ServerSession.handle` and `checkRequest`) and zbrain's tool/resource
//! gateway skeleton; tool and resource implementations land in W2 behind
//! [`ToolRegistry`].

use serde_json::{json, Value};

use crate::mcp::protocol::{
    error_response, negotiated_version, success_response, success_response_raw,
    validate_request_meta, CallToolResult, DiscoverResult, InitializeParams, InitializeResult,
    Implementation, ListPromptsPayload, ListResult, ListResourcesPayload,
    ListResourceTemplatesPayload, ListToolsPayload, McpError, OrderedJson, ReadResourceResult,
    ResultMeta, RpcRequest, ServerCapabilities, SuccessPayload, ToolEntry, ValidatedMeta,
    PROTOCOL_VERSION_20260728, SERVER_NAME, SUPPORTED_PROTOCOL_VERSIONS,
};
use crate::mcp::transport::{SafeStderr, Transport};

/// zbrain's own tool-input bound: marshaled arguments above this limit map to
/// -32602 (`input exceeds 1MB limit`); enforced by the W2 tool wrappers.
pub const MCP_MAX_INPUT_BYTES: usize = 1 << 20;

const RESULT_TYPE_COMPLETE: &str = "complete";
const CACHE_SCOPE_PUBLIC: &str = "public";
const TTL_MS_ZERO: i64 = 0;

/// Methods the Go SDK's server switch treats as legacy-only: under the
/// stateless per-request `_meta` protocol they answer -32601.
const LEGACY_ONLY_METHODS: [&str; 7] = [
    "initialize",
    "ping",
    "notifications/initialized",
    "notifications/roots/list_changed",
    "logging/setLevel",
    "resources/subscribe",
    "resources/unsubscribe",
];

/// Per-method receive flags mirroring go-sdk `serverMethodInfos`.
#[derive(Debug, Clone, Copy)]
struct MethodInfo {
    notification: bool,
    missing_params_ok: bool,
}

fn method_info(method: &str) -> Option<MethodInfo> {
    let (notification, missing_params_ok) = match method {
        "completion/complete" => (false, false),
        "server/discover" => (false, true),
        "initialize" => (false, false),
        "ping" => (false, true),
        "logging/setLevel" => (false, false),
        "notifications/cancelled" => (true, true),
        "notifications/initialized" => (true, true),
        "notifications/progress" => (true, false),
        "notifications/roots/list_changed" => (true, true),
        "prompts/get" => (false, false),
        "prompts/list" => (false, true),
        "resources/list" => (false, true),
        "resources/read" => (false, false),
        "resources/subscribe" | "resources/unsubscribe" => (false, false),
        "resources/templates/list" => (false, true),
        "tools/call" => (false, false),
        "tools/list" => (false, true),
        _ => return None,
    };
    Some(MethodInfo { notification, missing_params_ok })
}

/// Tool/resource surface the W2 implementations register into. The server
/// derives its advertised capabilities and list/call responses from it.
pub trait ToolRegistry {
    fn tools(&self) -> Vec<ToolEntry> {
        Vec::new()
    }

    /// Runs a tool; `None` arguments means JSON-null/absent arguments.
    fn call_tool(&self, _name: &str, _arguments: Option<&Value>) -> Result<CallToolResult, McpError> {
        Err(McpError::unknown_tool(_name))
    }

    fn resources(&self) -> Vec<Value> {
        Vec::new()
    }

    fn resource_templates(&self) -> Vec<Value> {
        Vec::new()
    }

    /// Reads a resource by URI; `None` maps to `Resource not found`.
    fn read_resource(&self, _uri: &str) -> Option<ReadResourceResult> {
        None
    }

    fn prompts(&self) -> Vec<Value> {
        Vec::new()
    }

    /// Gets a prompt by name; `None` maps to `unknown prompt`.
    fn get_prompt(&self, _name: &str) -> Option<Value> {
        None
    }
}

/// W1 stub: an empty registry. W2 registers the seven zbrain gateway tools
/// and the claim resource here.
pub struct StubRegistry;

impl ToolRegistry for StubRegistry {}

/// Runtime services the MCP layer reuses from the CLI/runtime boundary.
#[derive(Default)]
pub struct McpOptions {
    /// Server version reported in `serverInfo`.
    pub version: String,
    /// Diagnostics sink (never protocol output); defaults to discard like
    /// Go's `io.Discard` fallback.
    pub stderr: SafeStderr,
    /// Runtime paths backing the gateway registry; resolved from the
    /// environment when absent.
    pub paths: Option<crate::paths::Paths>,
    /// Injected clock for the gateway stores; system clock when absent.
    pub clock: Option<Box<dyn crate::clock::Clock>>,
}

/// Session state, mirroring go-sdk `ServerSessionState`.
#[derive(Default)]
struct SessionState {
    initialize_params: Option<InitializeParams>,
    initialized_notification: bool,
}

/// Runs the stdio MCP server until the transport reaches EOF or a fatal
/// framing error occurs. Diagnostics go to the stderr sink; protocol frames
/// go through the transport only.
pub struct Server<R: ToolRegistry> {
    registry: R,
    options: McpOptions,
}

impl<R: ToolRegistry> Server<R> {
    pub fn new(registry: R, options: McpOptions) -> Self {
        Self { registry, options }
    }

    fn server_info(&self) -> Implementation {
        Implementation { name: SERVER_NAME.to_string(), version: self.options.version.clone() }
    }

    /// Advertised capabilities: logging always, plus tools/resources when
    /// registered (go-sdk `Server.capabilities`).
    fn capabilities(&self) -> ServerCapabilities {
        let mut capabilities = ServerCapabilities {
            logging: Some(crate::mcp::protocol::LoggingCapabilities {}),
            ..Default::default()
        };
        if !self.registry.tools().is_empty() {
            capabilities.tools = Some(crate::mcp::protocol::ToolCapabilities { list_changed: true });
        }
        if !self.registry.resources().is_empty() || !self.registry.resource_templates().is_empty() {
            capabilities.resources =
                Some(crate::mcp::protocol::ResourceCapabilities { list_changed: true, subscribe: false });
        }
        capabilities
    }

    /// SEP-2322: `resultType: "complete"` on call/read results when the
    /// session is uninitialized (latest default) or negotiated at
    /// 2026-07-28 or later (go-sdk `clientSupportsMultiRoundTrip`).
    fn client_supports_multi_round_trip(state: &SessionState) -> bool {
        match &state.initialize_params {
            None => true,
            Some(params) => params.protocol_version.as_str() >= PROTOCOL_VERSION_20260728,
        }
    }

    /// Runs the dispatch loop until EOF (Ok) or a fatal framing error (Err).
    pub fn run(&mut self, transport: &mut impl Transport) -> Result<(), McpError> {
        self.options.stderr.log("server run start");
        let mut state = SessionState::default();
        loop {
            match transport.read_request() {
                Ok(Some(request)) => {
                    if let Some(frame) = self.handle(&mut state, &request) {
                        transport
                            .write_frame(&frame)
                            .map_err(|error| McpError::Plain(format!("write: {error}")))?;
                    }
                }
                Ok(None) => return Ok(()),
                Err(error) => {
                    self.options
                        .stderr
                        .log(&format!("server session ended with error: {error}"));
                    return Err(McpError::Plain(error.0));
                }
            }
        }
    }

    /// Handles one request; returns the wire frame to write, if any.
    fn handle(&mut self, state: &mut SessionState, request: &RpcRequest) -> Option<String> {
        let is_call = request.id.is_some();
        let method = request.method.as_str();
        let outcome = self.handle_inner(state, request);
        match outcome {
            Ok(SuccessPayload::Json(result)) => {
                if is_call {
                    let id = request.id.as_ref().expect("is_call");
                    Some(success_response(id, result))
                } else {
                    None
                }
            }
            Ok(SuccessPayload::Raw(result_json)) => {
                if is_call {
                    let id = request.id.as_ref().expect("is_call");
                    Some(success_response_raw(id, &result_json))
                } else {
                    None
                }
            }
            Err(error) => {
                if is_call {
                    let id = request.id.as_ref().expect("is_call");
                    Some(error_response(id, error.to_wire(method)))
                } else {
                    // Notification errors are logged, never answered.
                    self.options.stderr.log(&format!(
                        "notification {method} failed: {error}"
                    ));
                    None
                }
            }
        }
    }

    fn handle_inner(
        &mut self,
        state: &mut SessionState,
        request: &RpcRequest,
    ) -> Result<SuccessPayload, McpError> {
        let method = request.method.as_str();

        // Per-request protocol detection (SEP-2575), then the -32022 gate.
        let meta = validate_request_meta(request.params.as_ref())?;
        if meta.uses_new_protocol {
            let version = meta.protocol_version.as_deref().unwrap_or_default();
            if !SUPPORTED_PROTOCOL_VERSIONS.contains(&version) {
                return Err(McpError::unsupported_protocol_version(version));
            }
        }

        // Era gates: legacy methods vanish in the new protocol, and
        // `server/discover` exists only there.
        if LEGACY_ONLY_METHODS.contains(&method) && meta.uses_new_protocol {
            return Err(McpError::method_not_found());
        }
        if method == "server/discover" && !meta.uses_new_protocol {
            return Err(McpError::method_not_found());
        }

        let initialized = state.initialize_params.is_some();
        if !LEGACY_ONLY_METHODS.contains(&method) && method != "server/discover" {
            if !initialized && !meta.uses_new_protocol {
                return Err(McpError::Plain(format!(
                    "method {method:?} is invalid during session initialization"
                )));
            }
            if !initialized && meta.uses_new_protocol {
                state.initialize_params = Some(InitializeParams {
                    protocol_version: meta.protocol_version.clone().unwrap_or_default(),
                    client_info: meta.client_info.clone(),
                });
            }
        }

        // checkRequest: unknown methods and id/params shape violations.
        let Some(info) = method_info(method) else {
            return Err(McpError::method_not_found());
        };
        let is_call = request.id.is_some();
        if info.notification && is_call {
            return Err(McpError::invalid_request(format!(
                "unexpected id for {method:?}"
            )));
        }
        if !info.notification && !is_call {
            // Requests sent as notifications cannot be answered; the Go SDK
            // fails them silently.
            return Err(McpError::invalid_request(format!(
                "missing id for {method:?}"
            )));
        }
        if !info.missing_params_ok && !request.params_present {
            return Err(McpError::invalid_request(
                "missing required \"params\"".to_string(),
            ));
        }

        self.dispatch(state, request, &meta, method)
    }

    fn dispatch(
        &mut self,
        state: &mut SessionState,
        request: &RpcRequest,
        meta: &ValidatedMeta,
        method: &str,
    ) -> Result<SuccessPayload, McpError> {
        match method {
            "initialize" => self.handle_initialize(state, request).map(SuccessPayload::Json),
            "ping" => Ok(json!({}).into()),
            "logging/setLevel" => Ok(json!({}).into()),
            "notifications/initialized" => {
                self.handle_initialized_notification(state).map(SuccessPayload::Json)
            }
            "notifications/cancelled" | "notifications/progress"
            | "notifications/roots/list_changed" => Ok(json!({}).into()),
            "server/discover" => self.handle_discover().map(SuccessPayload::Json),
            "tools/list" => Ok(serde_json::to_value(ListResult {
                result_type: meta.uses_new_protocol.then_some(RESULT_TYPE_COMPLETE),
                meta: meta
                    .uses_new_protocol
                    .then(|| ResultMeta { server_info: Some(self.server_info()) }),
                ttl_ms: TTL_MS_ZERO,
                cache_scope: CACHE_SCOPE_PUBLIC,
                payload: ListToolsPayload { tools: self.registry.tools() },
            })
            .expect("list tools result")
            .into()),
            "tools/call" => self.handle_call_tool(state, request, meta),
            "resources/list" => Ok(serde_json::to_value(ListResult {
                result_type: meta.uses_new_protocol.then_some(RESULT_TYPE_COMPLETE),
                meta: meta
                    .uses_new_protocol
                    .then(|| ResultMeta { server_info: Some(self.server_info()) }),
                ttl_ms: TTL_MS_ZERO,
                cache_scope: CACHE_SCOPE_PUBLIC,
                payload: ListResourcesPayload { resources: self.registry.resources() },
            })
            .expect("list resources result")
            .into()),
            "resources/templates/list" => Ok(serde_json::to_value(ListResult {
                result_type: meta.uses_new_protocol.then_some(RESULT_TYPE_COMPLETE),
                meta: meta
                    .uses_new_protocol
                    .then(|| ResultMeta { server_info: Some(self.server_info()) }),
                ttl_ms: TTL_MS_ZERO,
                cache_scope: CACHE_SCOPE_PUBLIC,
                payload: ListResourceTemplatesPayload {
                    resource_templates: self.registry.resource_templates(),
                },
            })
            .expect("list resource templates result")
            .into()),
            "resources/read" => self.handle_read_resource(state, request, meta),
            "prompts/list" => Ok(serde_json::to_value(ListResult {
                result_type: meta.uses_new_protocol.then_some(RESULT_TYPE_COMPLETE),
                meta: meta
                    .uses_new_protocol
                    .then(|| ResultMeta { server_info: Some(self.server_info()) }),
                ttl_ms: TTL_MS_ZERO,
                cache_scope: CACHE_SCOPE_PUBLIC,
                payload: ListPromptsPayload { prompts: self.registry.prompts() },
            })
            .expect("list prompts result")
            .into()),
            "prompts/get" => {
                let name = request
                    .params
                    .as_ref()
                    .and_then(|p| p.get("name"))
                    .and_then(Value::as_str)
                    .unwrap_or_default();
                self.registry
                    .get_prompt(name)
                    .map(SuccessPayload::Json)
                    .ok_or_else(|| McpError::unknown_prompt(name))
            }
            "resources/subscribe" | "resources/unsubscribe" | "completion/complete" => {
                Err(McpError::method_not_found())
            }
            _ => Err(McpError::method_not_found()),
        }
    }

    fn handle_initialize(
        &mut self,
        state: &mut SessionState,
        request: &RpcRequest,
    ) -> Result<Value, McpError> {
        if state.initialize_params.is_some() {
            return Err(McpError::Plain("duplicate \"initialize\" received".to_string()));
        }
        // go-sdk initializeMethodInfo: JSON-null params decode to nil and
        // surface as a plain (code 0) error; absent params already failed
        // checkRequest with -32600.
        let Some(params) = &request.params else {
            return Err(McpError::Plain("missing required \"params\"".to_string()));
        };
        let params: InitializeParams = match params {
            Value::Object(object) => InitializeParams {
                protocol_version: object
                    .get("protocolVersion")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_string(),
                client_info: object
                    .get("clientInfo")
                    .and_then(|value| serde_json::from_value(value.clone()).ok()),
            },
            _ => {
                return Err(McpError::Plain(
                    "unmarshaling \"params\": invalid initialize params".to_string(),
                ));
            }
        };
        let protocol_version = negotiated_version(&params.protocol_version);
        state.initialize_params = Some(params);
        let result = InitializeResult {
            capabilities: self.capabilities(),
            instructions: None,
            protocol_version,
            server_info: self.server_info(),
        };
        Ok(serde_json::to_value(result).expect("initialize result"))
    }

    fn handle_initialized_notification(&self, state: &mut SessionState) -> Result<Value, McpError> {
        if state.initialize_params.is_none() {
            return Err(McpError::Plain(
                "\"notifications/initialized\" before \"initialize\"".to_string(),
            ));
        }
        if state.initialized_notification {
            return Err(McpError::Plain(
                "duplicate \"notifications/initialized\" received".to_string(),
            ));
        }
        state.initialized_notification = true;
        Ok(json!({}))
    }

    fn handle_discover(&self) -> Result<Value, McpError> {
        let result = DiscoverResult {
            result_type: RESULT_TYPE_COMPLETE,
            meta: ResultMeta { server_info: Some(self.server_info()) },
            ttl_ms: TTL_MS_ZERO,
            cache_scope: CACHE_SCOPE_PUBLIC,
            supported_versions: SUPPORTED_PROTOCOL_VERSIONS.iter().map(|v| v.to_string()).collect(),
            capabilities: self.capabilities(),
            instructions: None,
        };
        Ok(serde_json::to_value(result).expect("discover result"))
    }

    fn handle_call_tool(
        &self,
        state: &SessionState,
        request: &RpcRequest,
        meta: &ValidatedMeta,
    ) -> Result<SuccessPayload, McpError> {
        let params = request.params.as_ref().ok_or_else(|| {
            // JSON-null params fail closed as -32602 (go-sdk wraps
            // ErrInvalidParams when typed params decode to nil).
            McpError::Wire {
                code: crate::mcp::protocol::CODE_INVALID_PARAMS,
                message: "invalid params: missing required \"params\"".to_string(),
                data: None,
            }
        })?;
        let name = params
            .get("name")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_string();
        let arguments = params.get("arguments").filter(|a| !a.is_null());
        let mut result = self.registry.call_tool(&name, arguments)?;
        if Self::client_supports_multi_round_trip(state) {
            result.result_type = Some(RESULT_TYPE_COMPLETE);
        }
        let result_type = result.result_type;
        // Rendered by hand (OrderedJson) so the go-sdk CallToolResult field
        // order and Go's JSON string escaping survive the wire.
        let mut entries: Vec<(&'static str, OrderedJson)> = Vec::new();
        if meta.uses_new_protocol {
            entries.push(("_meta", server_info_meta(&self.server_info())));
        }
        if !result.content.is_empty() {
            entries.push((
                "content",
                OrderedJson::Array(
                    result
                        .content
                        .iter()
                        .map(|block| {
                            OrderedJson::object(vec![
                                ("type", OrderedJson::string(block.r#type)),
                                ("text", OrderedJson::string(&block.text)),
                            ])
                        })
                        .collect(),
                ),
            ));
        }
        if let Some(structured) = &result.structured_content {
            entries.push(("structuredContent", structured.clone()));
        }
        if result.is_error {
            entries.push(("isError", OrderedJson::Bool(true)));
        }
        if let Some(result_type) = result_type {
            entries.push(("resultType", OrderedJson::string(result_type)));
        }
        Ok(SuccessPayload::Raw(OrderedJson::object(entries).compact()))
    }

    /// Wraps the registry's read result the way the go-sdk does for every
    /// read: `resultType: "complete"` when the client supports
    /// multi-round-trip, the SEP-2575 `_meta` server annotation on the
    /// new-protocol path, and the go-sdk ReadResourceResult field order.
    fn handle_read_resource(
        &self,
        state: &SessionState,
        request: &RpcRequest,
        meta: &ValidatedMeta,
    ) -> Result<SuccessPayload, McpError> {
        let params = request.params.as_ref().ok_or_else(|| {
            McpError::invalid_params("missing required \"params\"")
        })?;
        let uri = params.get("uri").and_then(Value::as_str).unwrap_or_default();
        let result = self
            .registry
            .read_resource(uri)
            .ok_or_else(|| McpError::resource_not_found(uri))?;
        let result_type = if Self::client_supports_multi_round_trip(state) {
            Some(RESULT_TYPE_COMPLETE)
        } else {
            None
        };
        let mut entries: Vec<(&'static str, OrderedJson)> = Vec::new();
        if meta.uses_new_protocol {
            entries.push(("_meta", server_info_meta(&self.server_info())));
        }
        entries.push(("ttlMs", OrderedJson::Int(result.ttl_ms)));
        entries.push(("cacheScope", OrderedJson::string(result.cache_scope)));
        entries.push((
            "contents",
            OrderedJson::Array(
                result
                    .contents
                    .iter()
                    .map(|content| {
                        let mut content_entries = vec![("uri", OrderedJson::string(&content.uri))];
                        if let Some(mime_type) = &content.mime_type {
                            content_entries.push(("mimeType", OrderedJson::string(mime_type)));
                        }
                        if let Some(text) = &content.text {
                            content_entries.push(("text", OrderedJson::string(text)));
                        }
                        OrderedJson::object(content_entries)
                    })
                    .collect(),
            ),
        ));
        if let Some(result_type) = result_type {
            entries.push(("resultType", OrderedJson::string(result_type)));
        }
        Ok(SuccessPayload::Raw(OrderedJson::object(entries).compact()))
    }
}

/// SEP-2575 `_meta` server annotation rendered in go-sdk key order.
fn server_info_meta(server_info: &Implementation) -> OrderedJson {
    OrderedJson::object(vec![(
        "io.modelcontextprotocol/serverInfo",
        OrderedJson::object(vec![
            ("name", OrderedJson::string(&server_info.name)),
            ("version", OrderedJson::string(&server_info.version)),
        ]),
    )])
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::mcp::protocol::CODE_UNSUPPORTED_PROTOCOL_VERSION;
    use crate::mcp::transport::MemoryTransport;
    use serde_json::Value;

    fn run_session(requests: &str) -> (Vec<Value>, Result<(), McpError>) {
        let mut server = Server::new(StubRegistry, McpOptions::default());
        let mut transport = MemoryTransport::with_requests(requests.as_bytes().to_vec());
        let outcome = server.run(&mut transport);
        let responses = String::from_utf8(transport.into_writer())
            .expect("responses utf-8")
            .lines()
            .map(|line| serde_json::from_str(line).expect("response frame"))
            .collect();
        (responses, outcome)
    }

    fn initialize_frame(id: i64, version: &str) -> String {
        format!(
            r#"{{"jsonrpc":"2.0","id":{id},"method":"initialize","params":{{"protocolVersion":"{version}","capabilities":{{}},"clientInfo":{{"name":"zbrain-test","version":"0.0.0"}}}}}}"#
        )
    }

    fn result_field(response: &Value, key: &str) -> Value {
        response["result"][key].clone()
    }

    fn error_field(response: &Value, key: &str) -> Value {
        response["error"][key].clone()
    }

    // --- Revision matrix (ports TestProtocolRevisionMatrix) ---

    #[test]
    fn legacy_negotiates_20250618() {
        let (responses, outcome) =
            run_session(&format!("{}\n", initialize_frame(1, "2025-06-18")));
        outcome.unwrap();
        assert_eq!(responses.len(), 1);
        assert_eq!(result_field(&responses[0], "protocolVersion"), "2025-06-18");
        assert_eq!(
            result_field(&responses[0], "serverInfo")["name"],
            SERVER_NAME
        );
    }

    #[test]
    fn legacy_caps_at_20251125() {
        let (responses, outcome) =
            run_session(&format!("{}\n", initialize_frame(1, "2025-11-25")));
        outcome.unwrap();
        assert_eq!(result_field(&responses[0], "protocolVersion"), "2025-11-25");
    }

    #[test]
    fn legacy_negotiates_unsupported_to_20251125() {
        let (responses, outcome) =
            run_session(&format!("{}\n", initialize_frame(1, "1999-01-01")));
        outcome.unwrap();
        assert_eq!(result_field(&responses[0], "protocolVersion"), "2025-11-25");
    }

    #[test]
    fn legacy_negotiates_all_supported_versions() {
        for version in ["2024-11-05", "2025-03-26"] {
            let (responses, outcome) =
                run_session(&format!("{}\n", initialize_frame(1, version)));
            outcome.unwrap();
            assert_eq!(result_field(&responses[0], "protocolVersion"), version);
        }
    }

    #[test]
    fn discover_advertises_modern_version() {
        let request = r#"{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}"#;
        let (responses, outcome) = run_session(&format!("{request}\n"));
        outcome.unwrap();
        let versions = result_field(&responses[0], "supportedVersions");
        assert_eq!(versions[0], "2026-07-28");
        assert!(versions.as_array().unwrap().iter().any(|v| v == "2026-07-28"));
        assert!(result_field(&responses[0], "capabilities").is_object());
        assert_eq!(result_field(&responses[0], "resultType"), "complete");
    }

    #[test]
    fn stateless_requires_client_capabilities() {
        let request = r#"{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}"#;
        let (responses, outcome) = run_session(&format!("{request}\n"));
        outcome.unwrap();
        assert_eq!(error_field(&responses[0], "code"), -32602);
        assert_eq!(
            error_field(&responses[0], "message"),
            "missing or invalid _meta field \"io.modelcontextprotocol/clientCapabilities\""
        );
    }

    #[test]
    fn stateless_rejects_future_version() {
        let request = r#"{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2999-01-01","io.modelcontextprotocol/clientCapabilities":{}}}}"#;
        let (responses, outcome) = run_session(&format!("{request}\n"));
        outcome.unwrap();
        assert_eq!(error_field(&responses[0], "code"), CODE_UNSUPPORTED_PROTOCOL_VERSION);
        assert_eq!(error_field(&responses[0], "message"), "unsupported protocol version");
        assert_eq!(error_field(&responses[0], "data")["requested"], "2999-01-01");
        assert_eq!(error_field(&responses[0], "data")["supported"][0], "2026-07-28");
    }

    #[test]
    fn discover_rejected_on_legacy_path() {
        let request = r#"{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}"#;
        let (responses, outcome) = run_session(&format!("{request}\n"));
        outcome.unwrap();
        assert_eq!(error_field(&responses[0], "code"), -32601);
        assert_eq!(
            error_field(&responses[0], "message"),
            "method not found: \"server/discover\""
        );
    }

    #[test]
    fn legacy_methods_removed_in_new_protocol() {
        let request = r#"{"jsonrpc":"2.0","id":1,"method":"ping","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}"#;
        let (responses, outcome) = run_session(&format!("{request}\n"));
        outcome.unwrap();
        assert_eq!(error_field(&responses[0], "code"), -32601);
        assert_eq!(error_field(&responses[0], "message"), "method not found: \"ping\"");
    }

    // --- Core method behaviors (ports the W1-relevant mcp_test.go cases) ---

    #[test]
    fn unknown_method_maps_to_32601() {
        let (responses, outcome) = run_session(&format!(
            "{}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"no_such_method\",\"params\":{{}}}}\n",
            initialize_frame(1, "2025-06-18")
        ));
        outcome.unwrap();
        assert_eq!(responses.len(), 2);
        assert_eq!(error_field(&responses[1], "code"), -32601);
        assert_eq!(
            error_field(&responses[1], "message"),
            "method not found: \"no_such_method\""
        );
    }

    #[test]
    fn ping_returns_empty_result() {
        let (responses, outcome) = run_session(
            &format!(
                "{}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"ping\"}}\n",
                initialize_frame(1, "2025-06-18")
            ),
        );
        outcome.unwrap();
        assert_eq!(responses[1]["result"], json!({}));
    }

    #[test]
    fn ping_works_before_initialization() {
        let (responses, outcome) =
            run_session("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n");
        outcome.unwrap();
        assert_eq!(responses[0]["result"], json!({}));
    }

    #[test]
    fn method_invalid_during_initialization_is_plain_error() {
        let (responses, outcome) =
            run_session("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}\n");
        outcome.unwrap();
        assert_eq!(error_field(&responses[0], "code"), 0);
        assert_eq!(
            error_field(&responses[0], "message"),
            "method \"tools/list\" is invalid during session initialization"
        );
    }

    #[test]
    fn duplicate_initialize_is_plain_error() {
        let (responses, outcome) = run_session(&format!(
            "{}\n{}\n",
            initialize_frame(1, "2025-06-18"),
            initialize_frame(2, "2025-11-25")
        ));
        outcome.unwrap();
        assert_eq!(error_field(&responses[1], "code"), 0);
        assert_eq!(
            error_field(&responses[1], "message"),
            "duplicate \"initialize\" received"
        );
    }

    #[test]
    fn unknown_tool_maps_to_32602() {
        let (responses, outcome) = run_session(&format!(
            "{}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{{\"name\":\"no_such_tool\",\"arguments\":{{}}}}}}\n",
            initialize_frame(1, "2025-06-18")
        ));
        outcome.unwrap();
        assert_eq!(error_field(&responses[1], "code"), -32602);
        assert_eq!(
            error_field(&responses[1], "message"),
            "unknown tool \"no_such_tool\""
        );
    }

    #[test]
    fn call_tool_missing_params_maps_to_32600() {
        let (responses, outcome) = run_session(&format!(
            "{}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\"}}\n",
            initialize_frame(1, "2025-06-18")
        ));
        outcome.unwrap();
        assert_eq!(error_field(&responses[1], "code"), -32600);
        assert_eq!(
            error_field(&responses[1], "message"),
            "invalid request: missing required \"params\""
        );
    }

    #[test]
    fn call_tool_null_params_maps_to_32602() {
        let (responses, outcome) = run_session(&format!(
            "{}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":null}}\n",
            initialize_frame(1, "2025-06-18")
        ));
        outcome.unwrap();
        assert_eq!(error_field(&responses[1], "code"), -32602);
        assert_eq!(
            error_field(&responses[1], "message"),
            "invalid params: missing required \"params\""
        );
    }

    #[test]
    fn list_methods_work_without_params() {
        let (responses, outcome) = run_session(&format!(
            "{}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\"}}\n{{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"resources/list\"}}\n{{\"jsonrpc\":\"2.0\",\"id\":4,\"method\":\"prompts/list\"}}\n",
            initialize_frame(1, "2025-06-18")
        ));
        outcome.unwrap();
        assert_eq!(result_field(&responses[1], "tools"), json!([]));
        assert_eq!(result_field(&responses[1], "ttlMs"), 0);
        assert_eq!(result_field(&responses[1], "cacheScope"), "public");
        assert_eq!(result_field(&responses[1], "resultType"), Value::Null);
        assert_eq!(result_field(&responses[2], "resources"), json!([]));
        assert_eq!(result_field(&responses[3], "prompts"), json!([]));
    }

    #[test]
    fn modern_list_results_carry_result_type_and_meta() {
        let request = r#"{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}"#;
        let (responses, outcome) = run_session(&format!("{request}\n"));
        outcome.unwrap();
        assert_eq!(result_field(&responses[0], "resultType"), "complete");
        assert_eq!(
            result_field(&responses[0], "_meta")["io.modelcontextprotocol/serverInfo"]["name"],
            SERVER_NAME
        );
    }

    #[test]
    fn resource_read_without_resource_maps_to_not_found() {
        let (responses, outcome) = run_session(&format!(
            "{}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"resources/read\",\"params\":{{\"uri\":\"claims://current\"}}}}\n",
            initialize_frame(1, "2025-06-18")
        ));
        outcome.unwrap();
        assert_eq!(error_field(&responses[1], "code"), -32602);
        assert_eq!(error_field(&responses[1], "message"), "Resource not found");
        assert_eq!(error_field(&responses[1], "data")["uri"], "claims://current");
    }

    #[test]
    fn resource_read_missing_params_maps_to_32600() {
        let (responses, outcome) = run_session(&format!(
            "{}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"resources/read\"}}\n",
            initialize_frame(1, "2025-06-18")
        ));
        outcome.unwrap();
        assert_eq!(error_field(&responses[1], "code"), -32600);
    }

    #[test]
    fn subscribe_is_not_found() {
        let (responses, outcome) = run_session(&format!(
            "{}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"resources/subscribe\",\"params\":{{\"uri\":\"claims://current\"}}}}\n",
            initialize_frame(1, "2025-06-18")
        ));
        outcome.unwrap();
        assert_eq!(error_field(&responses[1], "code"), -32601);
        assert_eq!(
            error_field(&responses[1], "message"),
            "method not found: \"resources/subscribe\""
        );
    }

    // --- Notifications ---

    #[test]
    fn initialized_notification_is_silent() {
        let (responses, outcome) = run_session(&format!(
            "{}\n{{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"ping\"}}\n",
            initialize_frame(1, "2025-06-18")
        ));
        outcome.unwrap();
        assert_eq!(responses.len(), 2);
        assert_eq!(responses[0]["id"], 1);
        assert_eq!(responses[1]["id"], 2);
    }

    #[test]
    fn unknown_notification_is_silent() {
        let (responses, outcome) = run_session(&format!(
            "{}\n{{\"jsonrpc\":\"2.0\",\"method\":\"no_such_notification\"}}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"ping\"}}\n",
            initialize_frame(1, "2025-06-18")
        ));
        outcome.unwrap();
        assert_eq!(responses.len(), 2);
    }

    #[test]
    fn notification_with_id_maps_to_32600() {
        let (responses, outcome) = run_session(
            "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"notifications/initialized\"}\n",
        );
        outcome.unwrap();
        assert_eq!(error_field(&responses[0], "code"), -32600);
        assert_eq!(
            error_field(&responses[0], "message"),
            "invalid request: unexpected id for \"notifications/initialized\""
        );
    }

    // --- Transport-level session behaviors ---

    #[test]
    fn malformed_json_ends_session_without_response() {
        let requests = format!(
            "{}\nnot-json-at-all\n{{\"jsonrpc\":\"2.0\",\"id\":11,\"method\":\"ping\"}}\n",
            initialize_frame(1, "2025-06-18")
        );
        let (responses, outcome) = run_session(&requests);
        assert!(outcome.is_err());
        // The initialize response was written; the malformed frame ends the
        // session with no error frame and no response to the trailing ping.
        assert_eq!(responses.len(), 1);
        assert_eq!(responses[0]["id"], 1);
    }

    #[test]
    fn clean_eof_ends_session_ok() {
        let (_, outcome) = run_session(&format!("{}\n", initialize_frame(1, "2025-06-18")));
        outcome.unwrap();
    }

    #[test]
    fn stdout_purity_all_frames_are_jsonrpc() {
        let requests = format!(
            "{}\n{{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{{\"name\":\"no_such_tool\"}}}}\n{{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}}\n",
            initialize_frame(1, "2025-06-18")
        );
        let (responses, outcome) = run_session(&requests);
        outcome.unwrap();
        for frame in &responses {
            assert_eq!(frame["jsonrpc"], "2.0");
            assert!(
                frame.get("result").is_some() || frame.get("error").is_some(),
                "frame {frame} carries neither result nor error"
            );
        }
        assert_eq!(responses.len(), 2);
    }

    #[test]
    fn stateless_call_adopts_session_state_without_handshake() {
        // A stateless tools/list without a prior handshake must NOT hit the
        // initialization gate (go-sdk per-request `_meta` path).
        let request = r#"{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}"#;
        let (responses, outcome) = run_session(&format!("{request}\n"));
        outcome.unwrap();
        assert!(responses[0].get("result").is_some());
        assert_eq!(result_field(&responses[0], "resultType"), "complete");
    }

    #[test]
    fn legacy_meta_version_stays_on_legacy_path() {
        // _meta protocolVersion below 2026-07-28 falls back to the legacy
        // path (no resultType/_meta on results).
        let request = r#"{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"workspace_current","arguments":{},"_meta":{"io.modelcontextprotocol/protocolVersion":"2025-11-25","io.modelcontextprotocol/clientCapabilities":{}}}}"#;
        let (responses, outcome) = run_session(&format!(
            "{}\n{request}\n",
            initialize_frame(1, "2025-06-18")
        ));
        outcome.unwrap();
        assert_eq!(error_field(&responses[1], "message"), "unknown tool \"workspace_current\"");
    }
}
