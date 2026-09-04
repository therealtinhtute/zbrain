// zbrain-parity mirrors crates/tools/fixture-gen (Go oracle) for the m0/m1
// surface and emits an identical manifest schema for scripts/parity.sh.
use std::collections::BTreeMap;

use std::io::Write as _;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};

use serde::Serialize;
use sha2::{Digest as _, Sha256};

use zbrain::claims::{Claim, ClaimStore, OKF_CLAIM_TYPE};
use zbrain::clock::{rfc3339, FixedClock};
use zbrain::config::ensure_config;
use zbrain::coordination::generation_path;
use zbrain::evidence::{EvidenceStore, EvidenceValidator};
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

#[derive(Serialize)]
struct ClaimSummary {
    id: String,
    path: String,
    status: String,
    title: String,
    verified_digest: String,
}

#[derive(Serialize)]
struct EvidenceSummary {
    id: String,
    origin: String,
    captured_at: String,
    media_type: String,
    byte_length: i64,
    sha256: String,
}

#[derive(Serialize)]
struct TreeDigestEntry {
    path: String,
    kind: &'static str,
    mode: String,
    sha256: String,
}

#[derive(Serialize)]
struct ClaimsManifest {
    workspace: String,
    generation: String,
    claims: Vec<ClaimSummary>,
    evidence: Vec<EvidenceSummary>,
    tree: Vec<TreeDigestEntry>,
}

#[derive(Serialize)]
struct SetupManifest {
    schema_version: u32,
    config_created: bool,
    assets_copied: usize,
    assets_skipped: usize,
    tree: Vec<TreeEntry>,
    runtime_version_line: String,
}

fn main() {
    let mut home = String::new();
    let mut workspace = String::from("research");
    let mut op = String::from("workspace");
    let mut args = std::env::args().skip(1);
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--home" => home = args.next().expect("--home requires a value"),
            "--workspace" => workspace = args.next().expect("--workspace requires a value"),
            "--op" => op = args.next().expect("--op requires a value"),
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
    let result = match op.as_str() {
        "workspace" => run_workspace(Path::new(&home), &workspace),
        "setup" => run_setup(Path::new(&home)),
        "claims" => run_claims(Path::new(&home), &workspace),
        "claims-verify" => run_claims_verify(Path::new(&home), &workspace),
        other => {
            eprintln!("zbrain-parity: unknown op {other:?}");
            std::process::exit(1);
        }
    };
    if let Err(err) = result {
        eprintln!("zbrain-parity: {err}");
        std::process::exit(1);
    }
}

fn resolve_paths(home: &Path) -> Result<Paths, String> {
    Paths::resolve(Options {
        cwd: Some(home.to_path_buf()),
        home_dir: Some(home.to_path_buf()),
        runtime_dir: Some(home.join("runtime")),
    })
    .map_err(|err| format!("resolve paths: {err}"))
}

fn run_workspace(home: &Path, workspace: &str) -> Result<(), String> {
    let paths = resolve_paths(home)?;
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
    emit(&manifest)
}

// ---------------------------------------------------------------------------
// m2 claims/evidence parity (mirrors fixture-gen --op claims[-verify]).
// ---------------------------------------------------------------------------

fn parity_now() -> chrono::DateTime<chrono::Utc> {
    chrono::TimeZone::with_ymd_and_hms(&chrono::Utc, 2026, 7, 30, 10, 0, 0).unwrap()
}

fn parity_paths(home: &Path) -> Result<Paths, String> {
    let paths = resolve_paths(home)?;
    ensure_config(&paths.config_file).map_err(|err| format!("ensure config: {err}"))?;
    Ok(paths)
}

fn normalize_evidence_ids(value: &str) -> String {
    normalize_pattern(value, "evd_", "evd_NORMALIZED")
}

fn normalize_pattern(value: &str, prefix: &str, replacement: &str) -> String {
    let mut out = String::with_capacity(value.len());
    let bytes = value.as_bytes();
    let mut index = 0;
    while index < bytes.len() {
        if bytes[index..].starts_with(prefix.as_bytes())
            && bytes.len() - index >= prefix.len() + 32
            && bytes[index + prefix.len()..index + prefix.len() + 32]
                .iter()
                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(b))
        {
            out.push_str(replacement);
            index += prefix.len() + 32;
        } else {
            out.push(value[index..].chars().next().unwrap());
            index += value[index..].chars().next().unwrap().len_utf8();
        }
    }
    out
}

fn run_claims(home: &Path, workspace: &str) -> Result<(), String> {
    let paths = parity_paths(home)?;
    let clock = FixedClock::new(parity_now());
    create_workspace(&paths, workspace, &clock).map_err(|err| format!("create workspace: {err}"))?;
    let source_path = home.join("source.txt");
    std::fs::write(&source_path, b"parity evidence payload\n")
        .map_err(|err| format!("write parity source: {err}"))?;
    let evidence = EvidenceStore::new(paths.clone())
        .add_file(workspace, &source_path, "file://source.txt", "text/plain", &clock)
        .map_err(|err| format!("add evidence: {err}"))?;
    let created = rfc3339(parity_now());
    ClaimStore::new(paths.clone())
        .write_draft(
            workspace,
            Claim {
                claim_type: OKF_CLAIM_TYPE.into(),
                id: "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa".into(),
                tier: "projects".into(),
                status: zbrain::claims::CLAIM_STATUS_DRAFT.into(),
                title: "Parity owner claim".into(),
                basis: zbrain::claims::CLAIM_BASIS_OWNER.into(),
                created_at: created.clone(),
                created_by: "owner".into(),
                body: "Parity body\n".into(),
                ..Claim::default()
            },
        )
        .map_err(|err| format!("write owner draft: {err}"))?;
    ClaimStore::new(paths.clone())
        .write_draft(
            workspace,
            Claim {
                claim_type: OKF_CLAIM_TYPE.into(),
                id: "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb".into(),
                tier: "projects".into(),
                status: zbrain::claims::CLAIM_STATUS_DRAFT.into(),
                title: "Parity evidence claim".into(),
                basis: zbrain::claims::CLAIM_BASIS_EVIDENCE.into(),
                created_at: created,
                created_by: "owner".into(),
                evidence_ids: vec![evidence.id],
                body: "Parity evidence body\n".into(),
                ..Claim::default()
            },
        )
        .map_err(|err| format!("write evidence draft: {err}"))?;
    emit_claims_manifest(&paths, workspace, false)
}

fn run_claims_verify(home: &Path, workspace: &str) -> Result<(), String> {
    let paths = parity_paths(home)?;
    emit_claims_manifest(&paths, workspace, true)
}

fn emit_claims_manifest(paths: &Paths, workspace: &str, verify: bool) -> Result<(), String> {
    let root = zbrain::boundary::validate_workspace(paths, workspace)
        .map_err(|err| format!("validate workspace: {err}"))?;
    let scan = ClaimStore::new(paths.clone())
        .scan_workspace(workspace)
        .map_err(|err| format!("scan workspace: {err}"))?;
    if !scan.invalid.is_empty() {
        return Err(format!(
            "workspace scan reported invalid claims: {:?}",
            scan.invalid
        ));
    }
    let claims: Vec<ClaimSummary> = scan
        .claims
        .iter()
        .map(|claim| ClaimSummary {
            id: claim.id.clone(),
            path: claim.path.clone(),
            status: claim.status.clone(),
            title: claim.title.clone(),
            verified_digest: claim.verified_digest.clone(),
        })
        .collect();

    let sources_root = root.join("evidence/sources");
    let mut ids: Vec<String> = std::fs::read_dir(&sources_root)
        .map(|entries| {
            entries
                .filter_map(|entry| entry.ok())
                .filter(|entry| entry.file_type().map(|t| t.is_dir()).unwrap_or(false))
                .map(|entry| entry.file_name().to_string_lossy().to_string())
                .filter(|name| is_evidence_id_shaped(name))
                .collect()
        })
        .unwrap_or_default();
    ids.sort();
    let mut validator = EvidenceValidator::new(paths.clone(), workspace)
        .map_err(|err| format!("new evidence validator: {err}"))?;
    let mut evidence_list = Vec::with_capacity(ids.len());
    for id in &ids {
        let evidence = EvidenceStore::new(paths.clone())
            .read(workspace, id)
            .map_err(|err| format!("read evidence {id}: {err}"))?;
        if verify {
            validator
                .verify(id)
                .map_err(|err| format!("verify evidence {id}: {err}"))?;
        }
        evidence_list.push(EvidenceSummary {
            id: normalize_evidence_ids(&evidence.id),
            origin: evidence.origin,
            captured_at: evidence.captured_at,
            media_type: evidence.media_type,
            byte_length: evidence.byte_length,
            sha256: evidence.sha256,
        });
    }

    let generation = std::fs::read_to_string(root.join(".zbrain/generation.json"))
        .map_err(|err| format!("read generation: {err}"))?;
    let tree = walk_tree_digest(&paths.runtime_dir)?;

    emit(&ClaimsManifest {
        workspace: workspace.to_string(),
        generation,
        claims,
        evidence: evidence_list,
        tree,
    })
}

fn is_evidence_id_shaped(value: &str) -> bool {
    let Some(rest) = value.strip_prefix("evd_") else {
        return false;
    };
    rest.len() == 32 && rest.bytes().all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}

fn walk_tree_digest(runtime_dir: &Path) -> Result<Vec<TreeDigestEntry>, String> {
    let mut entries = BTreeMap::new();
    visit_digest(runtime_dir, runtime_dir, &mut entries)?;
    Ok(entries.into_values().collect())
}

fn visit_digest(
    runtime_dir: &Path,
    current: &Path,
    entries: &mut BTreeMap<String, TreeDigestEntry>,
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
        let (kind, sha256) = if metadata.is_dir() {
            ("dir", String::new())
        } else {
            let contents = std::fs::read(&child).map_err(|err| format!("read {rel:?}: {err}"))?;
            let normalized = normalize_evidence_ids(&String::from_utf8_lossy(&contents));
            let digest = Sha256::digest(normalized.as_bytes());
            (
                "file",
                digest.iter().map(|b| format!("{b:02x}")).collect(),
            )
        };
        let mode = format!("{:04o}", metadata.permissions().mode() & 0o777);
        let normalized_rel = normalize_evidence_ids(&rel);
        entries.insert(
            normalized_rel.clone(),
            TreeDigestEntry {
                path: normalized_rel,
                kind,
                mode,
                sha256,
            },
        );
        if metadata.is_dir() {
            visit_digest(runtime_dir, &child, entries)?;
        }
    }
    Ok(())
}

fn run_setup(home: &Path) -> Result<(), String> {
    let paths = resolve_paths(home)?;
    let summary =
        zbrain::setup::run_setup(&paths).map_err(|err| format!("run setup: {err}"))?;
    let tree = walk_tree(&paths.runtime_dir)?;
    let config =
        std::fs::read_to_string(&paths.config_file).map_err(|err| format!("read config: {err}"))?;
    let runtime_version_line = config
        .lines()
        .find(|line| line.starts_with("runtime_version:"))
        .unwrap_or_default()
        .to_string();
    let manifest = SetupManifest {
        schema_version: summary.schema_version,
        config_created: summary.config_created,
        assets_copied: summary.assets_copied,
        assets_skipped: summary.assets_skipped,
        tree,
        runtime_version_line,
    };
    emit(&manifest)
}

fn emit<T: serde::Serialize>(manifest: &T) -> Result<(), String> {
    let mut out = serde_json::to_string_pretty(manifest)
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
