//! JSON-RPC 2.0 / MCP protocol envelope types, error codes, and revision
//! semantics. Ports the wire behavior of `internal/mcp/` (which delegates to
//! github.com/modelcontextprotocol/go-sdk v1.7.0) byte-for-byte.

use serde::{Deserialize, Serialize};
use serde_json::Value;

/// MCP server implementation name reported to clients.
pub const SERVER_NAME: &str = "zbrain";

pub const PROTOCOL_VERSION_20260728: &str = "2026-07-28";
pub const PROTOCOL_VERSION_20251125: &str = "2025-11-25";

/// Versions advertised by `server/discover` and accepted by the stateless
/// per-request `_meta` path (go-sdk `supportedProtocolVersions`).
pub const SUPPORTED_PROTOCOL_VERSIONS: [&str; 5] = [
    "2026-07-28",
    "2025-11-25",
    "2025-06-18",
    "2025-03-26",
    "2024-11-05",
];

// Standard JSON-RPC 2.0 error codes.
pub const CODE_PARSE_ERROR: i64 = -32700;
pub const CODE_INVALID_REQUEST: i64 = -32600;
pub const CODE_METHOD_NOT_FOUND: i64 = -32601;
pub const CODE_INVALID_PARAMS: i64 = -32602;
pub const CODE_INTERNAL_ERROR: i64 = -32603;

/// SEP-2575: unsupported protocol version in per-request `_meta`.
pub const CODE_UNSUPPORTED_PROTOCOL_VERSION: i64 = -32022;

pub const META_PROTOCOL_VERSION: &str = "io.modelcontextprotocol/protocolVersion";
pub const META_CLIENT_CAPABILITIES: &str = "io.modelcontextprotocol/clientCapabilities";
pub const META_CLIENT_INFO: &str = "io.modelcontextprotocol/clientInfo";
pub const META_SERVER_INFO: &str = "io.modelcontextprotocol/serverInfo";

/// JSON-RPC request id. Number ids follow the Go SDK: float64 coerced to i64.
#[derive(Debug, Clone, PartialEq)]
pub enum Id {
    Number(i64),
    Text(String),
}

/// An incoming JSON-RPC request (calls and notifications).
#[derive(Debug, Clone)]
pub struct RpcRequest {
    pub id: Option<Id>,
    pub method: String,
    /// Decoded params; JSON `null` or an absent key both yield `None`, with
    /// `params_present` preserving the distinction the Go SDK makes between
    /// absent (`-32600`) and null (`-32602`/code 0) params.
    pub params: Option<Value>,
    pub params_present: bool,
}

/// Handler-level error that maps onto a JSON-RPC error response.
#[derive(Debug, Clone)]
pub enum McpError {
    /// Unknown or unsupported method. The Go SDK rewrites every -32601 error
    /// message to `method not found: "<method>"` on the wire (jsonrpc2
    /// processResult), so the message is derived at serialization time.
    MethodNotFound,
    /// Structured JSON-RPC error with an explicit wire code and optional data.
    Wire {
        code: i64,
        message: String,
        data: Option<Value>,
    },
    /// Plain error: the Go SDK emits these with wire code 0.
    Plain(String),
}

impl McpError {
    pub fn invalid_params(message: impl Into<String>) -> Self {
        Self::Wire {
            code: CODE_INVALID_PARAMS,
            message: message.into(),
            data: None,
        }
    }

    /// go-sdk `checkRequest` wraps ErrInvalidRequest, so the wire message
    /// carries the `invalid request: ` prefix.
    pub fn invalid_request(message: impl Into<String>) -> Self {
        Self::Wire {
            code: CODE_INVALID_REQUEST,
            message: format!("invalid request: {}", message.into()),
            data: None,
        }
    }

    pub fn method_not_found() -> Self {
        Self::MethodNotFound
    }

    pub fn unsupported_protocol_version(requested: &str) -> Self {
        Self::Wire {
            code: CODE_UNSUPPORTED_PROTOCOL_VERSION,
            message: "unsupported protocol version".to_string(),
            data: Some(serde_json::json!({
                "requested": requested,
                "supported": SUPPORTED_PROTOCOL_VERSIONS,
            })),
        }
    }

    pub fn unknown_tool(name: &str) -> Self {
        Self::invalid_params(format!("unknown tool {name:?}"))
    }

    pub fn unknown_prompt(name: &str) -> Self {
        Self::invalid_params(format!("unknown prompt {name:?}"))
    }

    pub fn resource_not_found(uri: &str) -> Self {
        Self::Wire {
            code: CODE_INVALID_PARAMS,
            message: "Resource not found".to_string(),
            data: Some(serde_json::json!({ "uri": uri })),
        }
    }

    /// Maps this error onto the wire form for a request to `method`,
    /// replicating the Go SDK's error handling: -32601 codes are rewritten to
    /// `method not found: "<method>"` (dropping data), plain errors carry
    /// code 0.
    pub fn to_wire(&self, method: &str) -> WireError {
        match self {
            Self::MethodNotFound => WireError {
                code: CODE_METHOD_NOT_FOUND,
                message: format!("method not found: {method:?}"),
                data: None,
            },
            Self::Wire {
                code: CODE_METHOD_NOT_FOUND,
                message,
                data,
            } => {
                let _ = message;
                let _ = data;
                WireError {
                    code: CODE_METHOD_NOT_FOUND,
                    message: format!("method not found: {method:?}"),
                    data: None,
                }
            }
            Self::Wire { code, message, data } => WireError {
                code: *code,
                message: message.clone(),
                data: data.clone(),
            },
            Self::Plain(message) => WireError {
                code: 0,
                message: message.clone(),
                data: None,
            },
        }
    }
}

impl std::fmt::Display for McpError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::MethodNotFound => write!(f, "method not found"),
            Self::Wire { message, .. } => write!(f, "{message}"),
            Self::Plain(message) => write!(f, "{message}"),
        }
    }
}

impl std::error::Error for McpError {}

/// Server identity reported in `serverInfo` / `_meta`.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Implementation {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub version: String,
}

/// Wire error object: field order `code, message, data`.
#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct WireError {
    pub code: i64,
    pub message: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub data: Option<Value>,
}

/// Result `_meta` carrying the SEP-2575 server identity annotation.
#[derive(Debug, Default, Serialize)]
pub struct ResultMeta {
    #[serde(rename = "io.modelcontextprotocol/serverInfo", skip_serializing_if = "Option::is_none")]
    pub server_info: Option<Implementation>,
}

/// Server capabilities, field order matches the Go SDK's sorted output
/// (`logging`, `prompts`, `resources`, `tools`).
#[derive(Debug, Default, Serialize)]
pub struct ServerCapabilities {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub logging: Option<LoggingCapabilities>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub prompts: Option<EmptyCapabilities>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub resources: Option<ResourceCapabilities>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tools: Option<ToolCapabilities>,
}

#[derive(Debug, Default, Serialize)]
pub struct LoggingCapabilities {}

#[derive(Debug, Default, Serialize)]
pub struct EmptyCapabilities {
    #[serde(rename = "listChanged", skip_serializing_if = "std::ops::Not::not")]
    pub list_changed: bool,
}

#[derive(Debug, Default, Serialize)]
pub struct ResourceCapabilities {
    #[serde(rename = "listChanged", skip_serializing_if = "std::ops::Not::not")]
    pub list_changed: bool,
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub subscribe: bool,
}

#[derive(Debug, Default, Serialize)]
pub struct ToolCapabilities {
    #[serde(rename = "listChanged", skip_serializing_if = "std::ops::Not::not")]
    pub list_changed: bool,
}

/// `initialize` result; the Go SDK emits this with sorted keys.
#[derive(Debug, Serialize)]
pub struct InitializeResult {
    pub capabilities: ServerCapabilities,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub instructions: Option<String>,
    #[serde(rename = "protocolVersion")]
    pub protocol_version: String,
    #[serde(rename = "serverInfo")]
    pub server_info: Implementation,
}

/// initialize params as accepted on the legacy handshake.
#[derive(Debug, Clone, Default)]
pub struct InitializeParams {
    pub protocol_version: String,
    pub client_info: Option<Implementation>,
}

/// `server/discover` result (SEP-2575); field order per wire capture:
/// resultType, _meta, ttlMs, cacheScope, supportedVersions, capabilities.
#[derive(Debug, Serialize)]
pub struct DiscoverResult {
    #[serde(rename = "resultType")]
    pub result_type: &'static str,
    #[serde(rename = "_meta")]
    pub meta: ResultMeta,
    #[serde(rename = "ttlMs")]
    pub ttl_ms: i64,
    #[serde(rename = "cacheScope")]
    pub cache_scope: &'static str,
    #[serde(rename = "supportedVersions")]
    pub supported_versions: Vec<String>,
    pub capabilities: ServerCapabilities,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub instructions: Option<String>,
}

/// Shared envelope of the `*/list` results: resultType, _meta, ttlMs,
/// cacheScope, then the flattened payload (omitted fields vanish on the
/// legacy path).
#[derive(Debug, Serialize)]
pub struct ListResult<P> {
    #[serde(rename = "resultType", skip_serializing_if = "Option::is_none")]
    pub result_type: Option<&'static str>,
    #[serde(rename = "_meta", skip_serializing_if = "Option::is_none")]
    pub meta: Option<ResultMeta>,
    #[serde(rename = "ttlMs")]
    pub ttl_ms: i64,
    #[serde(rename = "cacheScope")]
    pub cache_scope: &'static str,
    #[serde(flatten)]
    pub payload: P,
}

/// Tool listing entry; the Go SDK emits tools with sorted keys.
#[derive(Debug, Serialize)]
pub struct ToolEntry {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(rename = "inputSchema")]
    pub input_schema: Value,
    pub name: String,
}

#[derive(Debug, Serialize)]
pub struct ListToolsPayload {
    pub tools: Vec<ToolEntry>,
}

#[derive(Debug, Serialize)]
pub struct ListResourcesPayload {
    pub resources: Vec<Value>,
}

#[derive(Debug, Serialize)]
pub struct ListResourceTemplatesPayload {
    #[serde(rename = "resourceTemplates")]
    pub resource_templates: Vec<Value>,
}

#[derive(Debug, Serialize)]
pub struct ListPromptsPayload {
    pub prompts: Vec<Value>,
}

/// Text content block; field order `type, text`.
#[derive(Debug, Serialize)]
pub struct ContentBlock {
    pub r#type: &'static str,
    pub text: String,
}

/// `tools/call` result; field order per wire capture: _meta, content,
/// structuredContent, isError, resultType.
#[derive(Debug, Default, Serialize)]
pub struct CallToolResult {
    #[serde(rename = "_meta", skip_serializing_if = "Option::is_none")]
    pub meta: Option<ResultMeta>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub content: Vec<ContentBlock>,
    #[serde(rename = "structuredContent", skip_serializing_if = "Option::is_none")]
    pub structured_content: Option<Value>,
    #[serde(rename = "isError", skip_serializing_if = "std::ops::Not::not")]
    pub is_error: bool,
    #[serde(rename = "resultType", skip_serializing_if = "Option::is_none")]
    pub result_type: Option<&'static str>,
}

/// Response frame; field order `jsonrpc, id, result, error`.
#[derive(Debug, Serialize)]
pub struct ResponseFrame {
    jsonrpc: &'static str,
    id: Value,
    #[serde(skip_serializing_if = "Option::is_none")]
    result: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<WireError>,
}

pub fn id_to_value(id: &Id) -> Value {
    match id {
        Id::Number(n) => Value::from(*n),
        Id::Text(s) => Value::from(s.clone()),
    }
}

pub fn success_response(id: &Id, result: Value) -> String {
    serialize(&ResponseFrame {
        jsonrpc: "2.0",
        id: id_to_value(id),
        result: Some(result),
        error: None,
    })
}

pub fn error_response(id: &Id, wire_error: WireError) -> String {
    serialize(&ResponseFrame {
        jsonrpc: "2.0",
        id: id_to_value(id),
        result: None,
        error: Some(wire_error),
    })
}

fn serialize<T: Serialize>(frame: &T) -> String {
    serde_json::to_string(frame).unwrap_or_else(|_| "{}".to_string())
}

/// Result of decoding one wire frame: a request, or a client response frame
/// the server must skip.
#[derive(Debug)]
pub enum Frame {
    Request(RpcRequest),
    Response,
}

/// Fatal framing error: the Go SDK ends the session without responding.
#[derive(Debug, Clone)]
pub struct TransportError(pub String);

impl std::fmt::Display for TransportError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.0)
    }
}

impl std::error::Error for TransportError {}

/// Decodes a parsed JSON value into a request or a skippable response frame.
///
/// Mirrors go-sdk `DecodeMessage`: non-`{"jsonrpc":"2.0"}` envelopes and
/// invalid id types are fatal; frames without a method are responses.
pub fn decode_frame(value: &Value) -> Result<Frame, TransportError> {
    let invalid = |message: String| TransportError(format!("unmarshaling jsonrpc message: {message}"));
    let object = value.as_object().ok_or_else(|| invalid("expected object".to_string()))?;
    let version = object
        .get("jsonrpc")
        .and_then(Value::as_str)
        .ok_or_else(|| invalid("jsonrpc must be a string".to_string()))?;
    if version != "2.0" {
        return Err(TransportError(format!(
            "invalid message version tag {version:?}; expected \"2.0\""
        )));
    }
    let id = match object.get("id") {
        None | Some(Value::Null) => None,
        Some(Value::Number(n)) => Some(Id::Number(
            n.as_f64().map(|f| f as i64).unwrap_or_default(),
        )),
        Some(Value::String(s)) => Some(Id::Text(s.clone())),
        Some(other) => {
            return Err(TransportError(format!("invalid ID type {other:?}")));
        }
    };
    match object.get("method") {
        // go-sdk DecodeMessage treats an empty method key as a response.
        Some(Value::String(method)) if !method.is_empty() => {
            Ok(Frame::Request(RpcRequest {
                id,
                method: method.clone(),
                params: object.get("params").filter(|p| !p.is_null()).cloned(),
                // Raw key presence: JSON-null params pass checkRequest
                // (len("null") > 0) and fail later in params decoding.
                params_present: object.contains_key("params"),
            }))
        }
        Some(Value::Null) | Some(Value::String(_)) | None => {
            if id.is_some() {
                Ok(Frame::Response)
            } else {
                Err(TransportError("invalid request".to_string()))
            }
        }
        Some(other) => Err(invalid(format!("method must be a string, got {other:?}"))),
    }
}

/// Legacy `initialize` handshake era detection and per-request `_meta`
/// validation (SEP-2575). Mirrors go-sdk `validateRequestMeta` and
/// `negotiatedVersion`.
#[derive(Debug, Clone, Default)]
pub struct ValidatedMeta {
    pub uses_new_protocol: bool,
    pub protocol_version: Option<String>,
    pub client_info: Option<Implementation>,
    #[allow(dead_code)]
    pub client_capabilities: Option<Value>,
}

pub fn validate_request_meta(params: Option<&Value>) -> Result<ValidatedMeta, McpError> {
    let legacy = || ValidatedMeta::default();
    let Some(params) = params.and_then(Value::as_object) else {
        return Ok(legacy());
    };
    let Some(meta) = params.get("_meta").and_then(Value::as_object) else {
        return Ok(legacy());
    };
    let Some(version) = meta.get(META_PROTOCOL_VERSION).and_then(Value::as_str) else {
        return Ok(legacy());
    };
    if version < PROTOCOL_VERSION_20260728 {
        return Ok(legacy());
    }
    let client_info = match meta.get(META_CLIENT_INFO) {
        None | Some(Value::Null) => None,
        Some(value) => match implementation_from_value(value) {
            Some(implementation) => Some(implementation),
            None => {
                return Err(McpError::invalid_params(format!(
                    "invalid _meta field \"{META_CLIENT_INFO}\""
                )))
            }
        },
    };
    let capabilities = match meta.get(META_CLIENT_CAPABILITIES) {
        Some(value) if value.is_object() => Some(value.clone()),
        _ => {
            return Err(McpError::invalid_params(format!(
                "missing or invalid _meta field \"{META_CLIENT_CAPABILITIES}\""
            )))
        }
    };
    Ok(ValidatedMeta {
        uses_new_protocol: true,
        protocol_version: Some(version.to_string()),
        client_info,
        client_capabilities: capabilities,
    })
}

/// Decodes a `_meta` clientInfo value: null and absent are absent; objects
/// with string-or-null `name`/`version` decode (missing fields become empty,
/// matching Go zero values); anything else is invalid.
fn implementation_from_value(value: &Value) -> Option<Implementation> {
    let object = value.as_object()?;
    let string_or_empty = |key: &str| -> Option<String> {
        match object.get(key) {
            None | Some(Value::Null) => Some(String::new()),
            Some(Value::String(s)) => Some(s.clone()),
            Some(_) => None,
        }
    };
    Some(Implementation {
        name: string_or_empty("name")?,
        version: string_or_empty("version")?,
    })
}

/// Effective legacy handshake version: echo the client's version when
/// supported and pre-2026-07-28, else fall back to 2025-11-25.
pub fn negotiated_version(client_version: &str) -> String {
    if SUPPORTED_PROTOCOL_VERSIONS.contains(&client_version)
        && client_version < PROTOCOL_VERSION_20260728
    {
        client_version.to_string()
    } else {
        PROTOCOL_VERSION_20251125.to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn negotiated_version_matches_go_sdk() {
        assert_eq!(negotiated_version("2025-06-18"), "2025-06-18");
        assert_eq!(negotiated_version("2025-11-25"), "2025-11-25");
        assert_eq!(negotiated_version("2025-03-26"), "2025-03-26");
        assert_eq!(negotiated_version("2024-11-05"), "2024-11-05");
        // Unsupported legacy versions fall back to the 2025-11-25 cap.
        assert_eq!(negotiated_version("1999-01-01"), "2025-11-25");
        assert_eq!(negotiated_version(""), "2025-11-25");
    }

    #[test]
    fn meta_validation_legacy_paths() {
        let legacy = |v: Option<&Value>| !validate_request_meta(v).unwrap().uses_new_protocol;
        assert!(legacy(None));
        assert!(legacy(Some(&json!({}))));
        assert!(legacy(Some(&json!({"_meta": {}}))));
        assert!(legacy(Some(&json!({"_meta": {"other": 1}}))));
        assert!(legacy(Some(&json!({"_meta": {META_PROTOCOL_VERSION: 20260728}}))));
        // Versions below the stateless era stay on the legacy path.
        assert!(legacy(Some(&json!({"_meta": {META_PROTOCOL_VERSION: "2025-11-25"}}))));
        assert!(validate_request_meta(Some(&json!({
            "_meta": {META_PROTOCOL_VERSION: "2999-01-01"}
        })))
        .is_err()); // future version is new protocol; caps missing -> error
    }

    #[test]
    fn meta_validation_modern_requires_capabilities() {
        let err = validate_request_meta(Some(&json!({
            "_meta": {META_PROTOCOL_VERSION: "2026-07-28"}
        })))
        .unwrap_err();
        match err {
            McpError::Wire { code, message, .. } => {
                assert_eq!(code, CODE_INVALID_PARAMS);
                assert_eq!(
                    message,
                    "missing or invalid _meta field \"io.modelcontextprotocol/clientCapabilities\""
                );
            }
            other => panic!("unexpected error: {other:?}"),
        }
        let err = validate_request_meta(Some(&json!({
            "_meta": {META_PROTOCOL_VERSION: "2026-07-28", META_CLIENT_CAPABILITIES: "wrong"}
        })))
        .unwrap_err();
        assert!(matches!(err, McpError::Wire { code: CODE_INVALID_PARAMS, .. }));

        let ok = validate_request_meta(Some(&json!({
            "_meta": {
                META_PROTOCOL_VERSION: "2026-07-28",
                META_CLIENT_CAPABILITIES: {},
                META_CLIENT_INFO: {"name": "t", "version": "0"}
            }
        })))
        .unwrap();
        assert!(ok.uses_new_protocol);
        assert_eq!(ok.protocol_version.as_deref(), Some("2026-07-28"));
        assert_eq!(
            ok.client_info,
            Some(Implementation { name: "t".into(), version: "0".into() })
        );
    }

    #[test]
    fn meta_validation_rejects_bad_client_info() {
        let err = validate_request_meta(Some(&json!({
            "_meta": {
                META_PROTOCOL_VERSION: "2026-07-28",
                META_CLIENT_CAPABILITIES: {},
                META_CLIENT_INFO: "wrong"
            }
        })))
        .unwrap_err();
        match err {
            McpError::Wire { code, message, .. } => {
                assert_eq!(code, CODE_INVALID_PARAMS);
                assert_eq!(
                    message,
                    "invalid _meta field \"io.modelcontextprotocol/clientInfo\""
                );
            }
            other => panic!("unexpected error: {other:?}"),
        }
    }

    #[test]
    fn decode_frame_request_shapes() {
        let request = decode_frame(&json!({
            "jsonrpc": "2.0", "id": 1, "method": "ping", "params": {}
        }))
        .unwrap();
        match request {
            Frame::Request(req) => {
                assert_eq!(req.id, Some(Id::Number(1)));
                assert_eq!(req.method, "ping");
                assert!(req.params_present);
            }
            Frame::Response => panic!("expected request"),
        }

        let notification = decode_frame(&json!({
            "jsonrpc": "2.0", "method": "notifications/initialized"
        }))
        .unwrap();
        match notification {
            Frame::Request(req) => {
                assert_eq!(req.id, None);
                assert!(!req.params_present);
            }
            Frame::Response => panic!("expected request"),
        }

        // String ids round-trip; float ids coerce like Go's float64 -> int64.
        let string_id = decode_frame(&json!({"jsonrpc": "2.0", "id": "abc", "method": "ping"})).unwrap();
        assert!(matches!(
            string_id,
            Frame::Request(RpcRequest { id: Some(Id::Text(_)), .. })
        ));
        let float_id = decode_frame(&json!({"jsonrpc": "2.0", "id": 1.5, "method": "ping"})).unwrap();
        assert!(matches!(
            float_id,
            Frame::Request(RpcRequest { id: Some(Id::Number(1)), .. })
        ));
    }

    #[test]
    fn decode_frame_skips_client_responses() {
        assert!(matches!(
            decode_frame(&json!({"jsonrpc": "2.0", "id": 7, "result": {}})).unwrap(),
            Frame::Response
        ));
        assert!(matches!(
            decode_frame(&json!({"jsonrpc": "2.0", "id": 7, "error": {"code": 1, "message": "x"}})).unwrap(),
            Frame::Response
        ));
    }

    #[test]
    fn decode_frame_fatal_errors() {
        // Wrong version tag.
        assert!(decode_frame(&json!({"jsonrpc": "1.0", "id": 1, "method": "ping"})).is_err());
        // Non-string jsonrpc.
        assert!(decode_frame(&json!({"jsonrpc": {"bad": 1}})).is_err());
        // Non-scalar id.
        assert!(decode_frame(&json!({"jsonrpc": "2.0", "id": {}, "method": "ping"})).is_err());
        // Non-string method.
        assert!(decode_frame(&json!({"jsonrpc": "2.0", "id": 1, "method": 5})).is_err());
        // No method and no id.
        assert!(decode_frame(&json!({"jsonrpc": "2.0"})).is_err());
        // Non-object frame.
        assert!(decode_frame(&json!(null)).is_err());
        assert!(decode_frame(&json!(5)).is_err());
        // Empty method string falls back to the response branch.
        assert!(matches!(
            decode_frame(&json!({"jsonrpc": "2.0", "id": 3, "method": ""})).unwrap(),
            Frame::Response
        ));
    }

    #[test]
    fn error_wire_mapping() {
        let method_not_found = McpError::MethodNotFound.to_wire("no_such_method");
        assert_eq!(method_not_found.code, CODE_METHOD_NOT_FOUND);
        assert_eq!(method_not_found.message, "method not found: \"no_such_method\"");
        assert_eq!(method_not_found.data, None);

        let unknown_tool = McpError::unknown_tool("no_such_tool").to_wire("tools/call");
        assert_eq!(unknown_tool.code, CODE_INVALID_PARAMS);
        assert_eq!(unknown_tool.message, "unknown tool \"no_such_tool\"");

        let unsupported =
            McpError::unsupported_protocol_version("2999-01-01").to_wire("tools/list");
        assert_eq!(unsupported.code, CODE_UNSUPPORTED_PROTOCOL_VERSION);
        assert_eq!(unsupported.data.as_ref().unwrap()["requested"], "2999-01-01");
        assert_eq!(unsupported.data.as_ref().unwrap()["supported"][0], "2026-07-28");

        let plain = McpError::Plain("duplicate \"initialize\" received".to_string()).to_wire("initialize");
        assert_eq!(plain.code, 0);
        assert_eq!(plain.message, "duplicate \"initialize\" received");
    }

    #[test]
    fn response_frame_field_order() {
        let frame = success_response(&Id::Number(1), json!({}));
        assert_eq!(frame, r#"{"jsonrpc":"2.0","id":1,"result":{}}"#);
        let frame = error_response(
            &Id::Number(4),
            McpError::resource_not_found("claims://current").to_wire("resources/read"),
        );
        assert_eq!(
            frame,
            r#"{"jsonrpc":"2.0","id":4,"error":{"code":-32602,"message":"Resource not found","data":{"uri":"claims://current"}}}"#
        );
    }

    #[test]
    fn initialize_result_field_order() {
        let result = InitializeResult {
            capabilities: ServerCapabilities {
                logging: Some(LoggingCapabilities {}),
                resources: Some(ResourceCapabilities { list_changed: true, subscribe: false }),
                tools: Some(ToolCapabilities { list_changed: true }),
                ..Default::default()
            },
            instructions: None,
            protocol_version: "2025-06-18".to_string(),
            server_info: Implementation { name: SERVER_NAME.into(), version: "0.0.0".into() },
        };
        let encoded = serde_json::to_string(&result).unwrap();
        assert_eq!(
            encoded,
            r#"{"capabilities":{"logging":{},"resources":{"listChanged":true},"tools":{"listChanged":true}},"protocolVersion":"2025-06-18","serverInfo":{"name":"zbrain","version":"0.0.0"}}"#
        );
    }

    #[test]
    fn discover_result_field_order() {
        let result = DiscoverResult {
            result_type: "complete",
            meta: ResultMeta {
                server_info: Some(Implementation { name: SERVER_NAME.into(), version: "0.0.0".into() }),
            },
            ttl_ms: 0,
            cache_scope: "public",
            supported_versions: SUPPORTED_PROTOCOL_VERSIONS.iter().map(|v| v.to_string()).collect(),
            capabilities: ServerCapabilities {
                logging: Some(LoggingCapabilities {}),
                ..Default::default()
            },
            instructions: None,
        };
        let encoded = serde_json::to_string(&result).unwrap();
        assert!(encoded.starts_with(
            r#"{"resultType":"complete","_meta":{"io.modelcontextprotocol/serverInfo":{"name":"zbrain","version":"0.0.0"}},"ttlMs":0,"cacheScope":"public","supportedVersions":"#
        ));
        assert!(encoded.contains(r#""capabilities":{"logging":{}}"#));
    }

    #[test]
    fn list_result_field_order() {
        let legacy = ListResult {
            result_type: None,
            meta: None,
            ttl_ms: 0,
            cache_scope: "public",
            payload: ListToolsPayload { tools: vec![] },
        };
        assert_eq!(
            serde_json::to_string(&legacy).unwrap(),
            r#"{"ttlMs":0,"cacheScope":"public","tools":[]}"#
        );
        let modern = ListResult {
            result_type: Some("complete"),
            meta: Some(ResultMeta {
                server_info: Some(Implementation { name: SERVER_NAME.into(), version: "0.0.0".into() }),
            }),
            ttl_ms: 0,
            cache_scope: "public",
            payload: ListPromptsPayload { prompts: vec![] },
        };
        assert_eq!(
            serde_json::to_string(&modern).unwrap(),
            r#"{"resultType":"complete","_meta":{"io.modelcontextprotocol/serverInfo":{"name":"zbrain","version":"0.0.0"}},"ttlMs":0,"cacheScope":"public","prompts":[]}"#
        );
    }

    #[test]
    fn call_tool_result_field_order() {
        let result = CallToolResult {
            meta: Some(ResultMeta {
                server_info: Some(Implementation { name: SERVER_NAME.into(), version: "0.0.0".into() }),
            }),
            content: vec![ContentBlock { r#type: "text", text: "ok".to_string() }],
            structured_content: Some(json!({"schema_version": 1})),
            is_error: false,
            result_type: Some("complete"),
        };
        let encoded = serde_json::to_string(&result).unwrap();
        assert_eq!(
            encoded,
            r#"{"_meta":{"io.modelcontextprotocol/serverInfo":{"name":"zbrain","version":"0.0.0"}},"content":[{"type":"text","text":"ok"}],"structuredContent":{"schema_version":1},"resultType":"complete"}"#
        );
        let errored = CallToolResult {
            content: vec![ContentBlock { r#type: "text", text: "boom".to_string() }],
            is_error: true,
            ..Default::default()
        };
        assert_eq!(
            serde_json::to_string(&errored).unwrap(),
            r#"{"content":[{"type":"text","text":"boom"}],"isError":true}"#
        );
    }
}
