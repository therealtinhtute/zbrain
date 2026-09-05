//! stdio MCP gateway. Ports `internal/mcp/` (go-sdk v1.7.0 wire behavior):
//! protocol framing and revisions in [`protocol`], transports and the
//! diagnostics writer in [`transport`], and dispatch plus the tool/resource
//! registry seam in [`server`].

pub mod gateway;
pub mod protocol;
pub mod server;
pub mod transport;

pub use gateway::ZbrainRegistry;
pub use protocol::{Implementation, McpError, SERVER_NAME};
pub use server::{McpOptions, Server, StubRegistry, ToolRegistry, MCP_MAX_INPUT_BYTES};
pub use transport::{SafeStderr, Transport};

use crate::mcp::transport::IoTransport;
use crate::paths::Paths;

/// Runs the stdio MCP server until stdin closes or a fatal framing error
/// occurs. Diagnostics are routed to `options.stderr` (never stdout); SDK
/// protocol frames always land on stdout, mirroring `internal/mcp.Serve`:
/// `newServer` registers the gateway tools and resources onto the real
/// registry before running the transport loop.
pub fn serve(mut options: McpOptions) -> Result<(), McpError> {
    let stdin = std::io::stdin();
    let stdout = std::io::stdout();
    let mut transport = IoTransport::new(stdin.lock(), stdout.lock());
    let clock = options
        .clock
        .take()
        .unwrap_or_else(|| Box::new(crate::clock::SystemClock));
    let paths = match options.paths.take() {
        Some(paths) => paths,
        None => Paths::resolve(crate::paths::Options::default())
            .map_err(|error| McpError::Plain(error.to_string()))?,
    };
    let registry = ZbrainRegistry::new(paths, clock);
    Server::new(registry, options).run(&mut transport)
}
