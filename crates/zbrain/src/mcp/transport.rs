//! stdio transport and framing: byte-exact port of the Go SDK's stdio
//! behavior — whitespace-separated JSON values, a mandatory `\n`/`\r` byte
//! immediately after each value, response frames skipped, and any framing or
//! decode error treated as fatal (the session ends with no error frame).

use std::collections::VecDeque;
use std::io::{BufRead, Read, Write};
use std::sync::{Arc, Mutex};

use serde_json::Value;

use crate::mcp::protocol::{decode_frame, Frame, RpcRequest, TransportError};

/// Byte stream framing: yields the next JSON value or `None` at clean EOF.
///
/// Mirrors the Go SDK's stdio read loop (`json.Decoder` + the trailing-byte
/// check): values may be separated by a `\n` or `\r` (plus further
/// whitespace); any other byte immediately after a value is a fatal
/// "invalid trailing data" error, matching `newIOConn`.
pub struct FrameReader<R: Read> {
    inner: R,
    buf: Vec<u8>,
    eof: bool,
}

enum ScanError {
    Incomplete,
    Invalid(String),
}

impl<R: Read> FrameReader<R> {
    pub fn new(inner: R) -> Self {
        Self { inner, buf: Vec::with_capacity(4096), eof: false }
    }

    fn fill(&mut self) -> Result<(), std::io::Error> {
        if self.eof {
            return Ok(());
        }
        let mut chunk = [0u8; 4096];
        let n = self.inner.read(&mut chunk)?;
        if n == 0 {
            self.eof = true;
        } else {
            self.buf.extend_from_slice(&chunk[..n]);
        }
        Ok(())
    }

    fn compact(&mut self, keep_from: usize) {
        self.buf.drain(..keep_from);
    }

    /// Reads the next JSON value, or `None` at clean EOF.
    pub fn next_value(&mut self) -> Result<Option<Value>, TransportError> {
        loop {
            let start = skip_whitespace(&self.buf);
            match scan_value_end(&self.buf, start) {
                Ok(end) => {
                    let separator = self.buf.get(end).copied();
                    match separator {
                        Some(b'\n') | Some(b'\r') => {
                            let value = self.take(start, end + 1)?;
                            return parse_value(&value);
                        }
                        Some(_) => {
                            self.take(start, end)?;
                            return Err(TransportError(
                                "invalid trailing data at the end of stream".to_string(),
                            ));
                        }
                        None => {
                            // Value ends exactly at the buffer edge; Go only
                            // accepts this once the reader is at EOF.
                            self.compact(start);
                            if !self.eof {
                                self.fill().map_err(io_error)?;
                            }
                            if self.eof {
                                let value = self.take(0, self.buf.len())?;
                                return parse_value(&value);
                            }
                        }
                    }
                }
                Err(ScanError::Incomplete) => {
                    self.compact(start);
                    self.fill().map_err(io_error)?;
                    if self.eof {
                        if self.buf.is_empty() {
                            return Ok(None);
                        }
                        return Err(TransportError(
                            "unexpected end of JSON input".to_string(),
                        ));
                    }
                }
                Err(ScanError::Invalid(message)) => {
                    return Err(TransportError(message));
                }
            }
        }
    }

    fn take(&mut self, start: usize, end: usize) -> Result<Vec<u8>, TransportError> {
        let value = self.buf[start..end].to_vec();
        self.compact(end);
        Ok(value)
    }
}

fn io_error(error: std::io::Error) -> TransportError {
    TransportError(format!("read: {error}"))
}

fn parse_value(bytes: &[u8]) -> Result<Option<Value>, TransportError> {
    serde_json::from_slice(bytes)
        .map(Some)
        .map_err(|error| TransportError(error.to_string()))
}

fn skip_whitespace(buf: &[u8]) -> usize {
    buf.iter()
        .take_while(|b| matches!(**b, b' ' | b'\t' | b'\n' | b'\r'))
        .count()
}

/// Finds the byte index just past a complete JSON value starting at `start`.
fn scan_value_end(buf: &[u8], start: usize) -> Result<usize, ScanError> {
    if start >= buf.len() {
        return Err(ScanError::Incomplete);
    }
    match buf[start] {
        b'{' | b'[' => {
            let mut depth = 0usize;
            let mut in_string = false;
            let mut escaped = false;
            for (offset, byte) in buf[start..].iter().enumerate() {
                let byte = *byte;
                if in_string {
                    if escaped {
                        escaped = false;
                    } else if byte == b'\\' {
                        escaped = true;
                    } else if byte == b'"' {
                        in_string = false;
                    }
                    continue;
                }
                match byte {
                    b'"' => in_string = true,
                    b'{' | b'[' => depth += 1,
                    b'}' | b']' => {
                        depth = depth.checked_sub(1).ok_or_else(|| {
                            ScanError::Invalid("unexpected closing brace".to_string())
                        })?;
                        if depth == 0 {
                            return Ok(start + offset + 1);
                        }
                    }
                    _ => {}
                }
            }
            Err(ScanError::Incomplete)
        }
        b'"' => {
            let mut escaped = false;
            for (offset, byte) in buf[start..].iter().enumerate() {
                let byte = *byte;
                if escaped {
                    escaped = false;
                } else if byte == b'\\' {
                    escaped = true;
                } else if byte == b'"' {
                    return Ok(start + offset + 1);
                }
            }
            Err(ScanError::Incomplete)
        }
        b't' => literal_end(buf, start, b"true"),
        b'f' => literal_end(buf, start, b"false"),
        b'n' => literal_end(buf, start, b"null"),
        b'-' | b'0'..=b'9' => {
            let end = start
                + buf[start..]
                    .iter()
                    .position(|b| !b"0123456789+-.eE".contains(b))
                    .unwrap_or(buf.len() - start);
            if end == buf.len() {
                return Err(ScanError::Incomplete);
            }
            Ok(end)
        }
        _ => Err(ScanError::Invalid(format!(
            "invalid character {:?} looking for beginning of value",
            buf[start] as char
        ))),
    }
}

fn literal_end(buf: &[u8], start: usize, literal: &[u8]) -> Result<usize, ScanError> {
    let end = start + literal.len();
    if buf[start..].len() < literal.len() {
        if literal.starts_with(&buf[start..]) {
            return Err(ScanError::Incomplete);
        }
        return Err(ScanError::Invalid(format!(
            "invalid character {:?} looking for beginning of value",
            buf[start] as char
        )));
    }
    if &buf[start..end] == literal {
        Ok(end)
    } else {
        Err(ScanError::Invalid(format!(
            "invalid character {:?} looking for beginning of value",
            buf[start] as char
        )))
    }
}

/// Transport: reads server-bound requests (skipping client response frames)
/// and writes protocol frames. Any read error is fatal to the session.
pub trait Transport {
    fn read_request(&mut self) -> Result<Option<RpcRequest>, TransportError>;
    fn write_frame(&mut self, frame: &str) -> std::io::Result<()>;
}

/// Generic IO transport; the stdio server pairs it with stdin/stdout, tests
/// pair it with in-memory cursors and sinks (mirroring the Go tests'
/// in-memory transports).
pub struct IoTransport<R: BufRead, W: Write> {
    reader: FrameReader<R>,
    writer: W,
}

impl<R: BufRead, W: Write> IoTransport<R, W> {
    pub fn new(reader: R, writer: W) -> Self {
        Self { reader: FrameReader::new(reader), writer }
    }

    pub fn into_writer(self) -> W {
        self.writer
    }
}

impl<R: BufRead, W: Write> Transport for IoTransport<R, W> {
    fn read_request(&mut self) -> Result<Option<RpcRequest>, TransportError> {
        loop {
            match self.reader.next_value()? {
                Some(value) => match decode_frame(&value)? {
                    Frame::Request(request) => return Ok(Some(request)),
                    Frame::Response => continue,
                },
                None => return Ok(None),
            }
        }
    }

    fn write_frame(&mut self, frame: &str) -> std::io::Result<()> {
        self.writer.write_all(frame.as_bytes())?;
        self.writer.write_all(b"\n")?;
        self.writer.flush()
    }
}

use std::io::Cursor;

/// In-memory transport for tests: requests from a byte cursor, responses
/// captured in a `Vec<u8>` sink.
pub type MemoryTransport = IoTransport<Cursor<Vec<u8>>, Vec<u8>>;

impl MemoryTransport {
    pub fn with_requests(requests: impl Into<Vec<u8>>) -> Self {
        IoTransport::new(Cursor::new(requests.into()), Vec::new())
    }

    pub fn responses(&self) -> &[u8] {
        &self.writer
    }
}

/// Diagnostics writer guarded by a mutex — the port of Go's
/// `safeWriter`/`ensureSafeWriter`: stderr never interleaves and stdout
/// carries only protocol frames.
#[derive(Default)]
pub struct SafeStderr {
    inner: Mutex<Option<Box<dyn Write + Send>>>,
}

impl SafeStderr {
    pub fn new(writer: Box<dyn Write + Send>) -> Self {
        Self { inner: Mutex::new(Some(writer)) }
    }

    /// Real stderr, as `mcp serve` uses.
    pub fn system() -> Self {
        Self::new(Box::new(std::io::stderr()))
    }

    pub fn log(&self, message: &str) {
        if let Ok(mut guard) = self.inner.lock() {
            if let Some(writer) = guard.as_mut() {
                let _ = writeln!(writer, "{message}");
            }
        }
    }
}

/// A queue-backed transport pair for multi-request sessions in tests: the
/// client queues raw frames, the server drains them and collects responses.
#[derive(Default)]
struct MemorySessionShared {
    pending: VecDeque<String>,
    responses: Vec<String>,
}

pub struct MemorySession {
    shared: Arc<Mutex<MemorySessionShared>>,
}

pub struct MemoryClient {
    shared: Arc<Mutex<MemorySessionShared>>,
}

impl MemorySession {
    pub fn pair() -> (MemoryClient, MemorySession) {
        let shared = Arc::new(Mutex::new(MemorySessionShared::default()));
        (MemoryClient { shared: Arc::clone(&shared) }, MemorySession { shared })
    }

    pub fn responses(&self) -> Vec<String> {
        self.shared.lock().expect("memory session").responses.clone()
    }
}

impl Transport for MemorySession {
    fn read_request(&mut self) -> Result<Option<RpcRequest>, TransportError> {
        loop {
            let raw = {
                let mut shared = self.shared.lock().expect("memory session");
                shared.pending.pop_front()
            };
            let Some(raw) = raw else { return Ok(None) };
            let value: Value = serde_json::from_str(&raw)
                .map_err(|error| TransportError(error.to_string()))?;
            match decode_frame(&value)? {
                Frame::Request(request) => return Ok(Some(request)),
                Frame::Response => continue,
            }
        }
    }

    fn write_frame(&mut self, frame: &str) -> std::io::Result<()> {
        self.shared
            .lock()
            .expect("memory session")
            .responses
            .push(frame.to_string());
        Ok(())
    }
}

impl MemoryClient {
    pub fn send(&self, request: &str) {
        self.shared
            .lock()
            .expect("memory session")
            .pending
            .push_back(request.to_string());
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::mcp::protocol::Id;
    use serde_json::json;

    fn read_all(transport: &mut MemoryTransport) -> Vec<RpcRequest> {
        let mut requests = Vec::new();
        while let Some(request) = transport.read_request().unwrap() {
            requests.push(request);
        }
        requests
    }

    #[test]
    fn reads_line_delimited_requests() {
        let mut transport = MemoryTransport::with_requests(
            r#"{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
"#,
        );
        let requests = read_all(&mut transport);
        assert_eq!(requests.len(), 2);
        assert_eq!(requests[0].id, Some(Id::Number(1)));
        assert_eq!(requests[1].id, None);
    }

    #[test]
    fn skips_client_response_frames() {
        let mut transport = MemoryTransport::with_requests(
            r#"{"jsonrpc":"2.0","id":9,"result":{}}
{"jsonrpc":"2.0","id":1,"method":"ping"}
"#,
        );
        let requests = read_all(&mut transport);
        assert_eq!(requests.len(), 1);
        assert_eq!(requests[0].method, "ping");
    }

    #[test]
    fn accepts_multiline_json_values() {
        let mut transport = MemoryTransport::with_requests(
            "{\n  \"jsonrpc\": \"2.0\",\n  \"id\": 1,\n  \"method\": \"ping\"\n}\n",
        );
        let requests = read_all(&mut transport);
        assert_eq!(requests.len(), 1);
        assert_eq!(requests[0].method, "ping");
    }

    #[test]
    fn rejects_space_between_values() {
        // Go's stdio transport requires the byte after a value to be \n or
        // \r; the read surfaces the error without delivering the frame
        // (ioConn.Read discards msgOrErr{msg, err} pairs carrying an error).
        let mut transport = MemoryTransport::with_requests(
            r#"{"jsonrpc":"2.0","id":1,"method":"ping"} {"jsonrpc":"2.0"}"#,
        );
        assert!(transport.read_request().is_err());
    }

    #[test]
    fn rejects_non_json_input() {
        let mut transport = MemoryTransport::with_requests("not-json-at-all\n");
        assert!(transport.read_request().is_err());
    }

    #[test]
    fn rejects_wrong_version_tag() {
        let mut transport = MemoryTransport::with_requests(
            r#"{"jsonrpc":"1.0","id":1,"method":"ping"}"#,
        );
        assert!(transport.read_request().is_err());
    }

    #[test]
    fn rejects_truncated_value() {
        let mut transport = MemoryTransport::with_requests(
            r#"{"jsonrpc":"2.0","id":1,"method":"pi"#,
        );
        assert!(transport.read_request().is_err());
    }

    #[test]
    fn value_then_eof_without_newline_is_accepted() {
        let mut transport =
            MemoryTransport::with_requests(r#"{"jsonrpc":"2.0","id":1,"method":"ping"}"#);
        assert_eq!(read_all(&mut transport).len(), 1);
    }

    #[test]
    fn write_frame_appends_newline() {
        let mut transport = MemoryTransport::with_requests(Vec::new());
        transport.write_frame(r#"{"jsonrpc":"2.0","id":1,"result":{}}"#).unwrap();
        assert_eq!(transport.responses(), b"{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n");
    }

    #[test]
    fn memory_session_round_trip() {
        let (client, mut session) = MemorySession::pair();
        client.send(r#"{"jsonrpc":"2.0","id":1,"method":"ping"}"#);
        let request = session.read_request().unwrap().unwrap();
        assert_eq!(request.method, "ping");
        session
            .write_frame(&crate::mcp::protocol::success_response(
                &request.id.unwrap(),
                json!({}),
            ))
            .unwrap();
        assert_eq!(session.responses(), [r#"{"jsonrpc":"2.0","id":1,"result":{}}"#]);
        assert!(session.read_request().unwrap().is_none());
    }

    #[test]
    fn safe_stderr_discards_when_unset() {
        let stderr = SafeStderr::default();
        stderr.log("server run start"); // must not panic
    }
}
