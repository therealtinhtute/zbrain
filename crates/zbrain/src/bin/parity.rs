// zbrain-parity mirrors crates/tools/fixture-gen (Go oracle) for the m0
// surface and emits an identical manifest schema for scripts/parity.sh.
use std::collections::BTreeMap;
use std::io::Write as _;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};

use serde::Serialize;

use zbrain::clock::FixedClock;
use zbrain::config::ensure_config;
use zbrain::coordination::generation_path;
use zbrain::paths::{Options, Paths};
use zbrain::workspace::{create_workspace, resolve_current_workspace};

#[derive(Serialize)]
struct TreeEntry {
    path: String,
    kind: &'static str,
    mode: String,
}

#[derive(Serialize)]
struct Manifest {
    config: String,
    workspace_md: String,
    evidence_index: String,
    tree: Vec<TreeEntry>,
    default_read: String,
    generation_rel: String,
}

fn main() {
    let mut home = String::new();
    let mut workspace = String::from("research");
    let mut args = std::env::args().skip(1);
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--home" => home = args.next().expect("--home requires a value"),
            "--workspace" => workspace = args.next().expect("--workspace requires a value"),
            other => {
                eprintln!("zbrain-parity: unknown argument {other:?}");
                std::process::exit(1);
            }
        }
    }
    if home.is_empty() {
        eprintln!("zbrain-parity: --home is required");
        std::process::exit(1);
    }
    if let Err(err) = run(Path::new(&home), &workspace) {
        eprintln!("zbrain-parity: {err}");
        std::process::exit(1);
    }
}

fn run(home: &Path, workspace: &str) -> Result<(), String> {
    let paths = Paths::resolve(Options {
        cwd: Some(home.to_path_buf()),
        home_dir: Some(home.to_path_buf()),
        runtime_dir: Some(home.join("runtime")),
    })
    .map_err(|err| format!("resolve paths: {err}"))?;
    ensure_config(&paths.config_file).map_err(|err| format!("ensure config: {err}"))?;
    let clock = FixedClock::new(
        chrono::TimeZone::with_ymd_and_hms(&chrono::Utc, 2026, 7, 30, 10, 0, 0).unwrap(),
    );    create_workspace(&paths, workspace, &clock).map_err(|err| format!("create workspace: {err}"))?;
    let current =
        resolve_current_workspace(&paths).map_err(|err| format!("resolve current: {err}"))?;

    let root = paths.workspaces_dir.join(workspace);
    let config = std::fs::read(&paths.config_file).map_err(|err| format!("read config: {err}"))?;
    let workspace_md = std::fs::read(root.join("workspace.md"))
        .map_err(|err| format!("read workspace.md: {err}"))?;
    let evidence_index = std::fs::read(root.join("evidence/_index.md"))
        .map_err(|err| format!("read evidence/_index.md: {err}"))?;

    let tree = walk_tree(&paths.runtime_dir)?;
    let root = zbrain::boundary::validate_workspace(&paths, workspace)
        .map_err(|err| format!("validate workspace: {err}"))?;
    let generation = generation_path(&paths, workspace)
        .map_err(|err| format!("generation path: {err}"))?;
    let generation_rel = generation
        .strip_prefix(&root)
        .map(|p| p.to_string_lossy().to_string())
        .map_err(|err| format!("rel generation path: {err}"))?;

    let manifest = Manifest {
        config: String::from_utf8(config).map_err(|err| format!("config utf8: {err}"))?,
        workspace_md: String::from_utf8(workspace_md)
            .map_err(|err| format!("workspace.md utf8: {err}"))?,
        evidence_index: String::from_utf8(evidence_index)
            .map_err(|err| format!("evidence index utf8: {err}"))?,
        tree,
        default_read: current.workspace,
        generation_rel,
    };
    let mut out = serde_json::to_string_pretty(&manifest)
        .map_err(|err| format!("marshal manifest: {err}"))?;
    out.push('\n');
    std::io::stdout()
        .write_all(out.as_bytes())
        .map_err(|err| format!("write manifest: {err}"))?;
    Ok(())
}

fn walk_tree(runtime_dir: &Path) -> Result<Vec<TreeEntry>, String> {
    let mut entries = BTreeMap::new();
    visit(runtime_dir, runtime_dir, &mut entries)?;
    Ok(entries.into_values().collect())
}

fn visit(
    runtime_dir: &Path,
    current: &Path,
    entries: &mut BTreeMap<String, TreeEntry>,
) -> Result<(), String> {
    let mut children: Vec<PathBuf> = std::fs::read_dir(current)
        .map_err(|err| format!("walk tree: {err}"))?
        .filter_map(|entry| entry.ok())
        .map(|entry| entry.path())
        .collect();
    children.sort();
    for child in children {
        let rel = child
            .strip_prefix(runtime_dir)
            .map_err(|err| format!("rel path: {err}"))?
            .to_string_lossy()
            .to_string();
        let metadata = std::fs::symlink_metadata(&child).map_err(|err| format!("walk tree: {err}"))?;
        let kind = if metadata.is_dir() { "dir" } else { "file" };
        let mode = format!("{:04o}", metadata.permissions().mode() & 0o777);
        entries.insert(
            rel.clone(),
            TreeEntry {
                path: rel,
                kind,
                mode,
            },
        );
        if metadata.is_dir() {
            visit(runtime_dir, &child, entries)?;
        }
    }
    Ok(())
}
