//! stdio MCP gateway. Ports `internal/mcp/` (go-sdk v1.7.0 wire behavior):
//! protocol framing and revisions in [`protocol`], transports and the
//! diagnostics writer in [`transport`], and dispatch plus the tool/resource
//! registry seam in [`server`].

pub mod protocol;
pub mod server;
pub mod transport;

pub use protocol::{Implementation, McpError, SERVER_NAME};
pub use server::{McpOptions, Server, StubRegistry, ToolRegistry, MCP_MAX_INPUT_BYTES};
pub use transport::{SafeStderr, Transport};

use crate::mcp::transport::IoTransport;

/// Runs the stdio MCP server until stdin closes or a fatal framing error
/// occurs. Diagnostics are routed to `options.stderr` (never stdout); SDK
/// protocol frames always land on stdout, mirroring `internal/mcp.Serve`.
pub fn serve(options: McpOptions) -> Result<(), McpError> {
    let stdin = std::io::stdin();
    let stdout = std::io::stdout();
    let mut transport = IoTransport::new(stdin.lock(), stdout.lock());
    Server::new(StubRegistry, options).run(&mut transport)
}
