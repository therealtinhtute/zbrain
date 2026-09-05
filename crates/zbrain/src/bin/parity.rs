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

// ---------------------------------------------------------------------------
// m3 lifecycle parity (mirrors fixture-gen --op lifecycle[-verify]).
// ---------------------------------------------------------------------------

#[derive(Serialize)]
struct TransitionSummary {
    kind: String,
    at: String,
    by: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    reason: String,
    #[serde(serialize_with = "null_if_empty")]
    related_claim_ids: Vec<String>,
    #[serde(skip_serializing_if = "String::is_empty")]
    prior_verification_digest: String,
}

// Go emits nil slices as JSON null; keep the manifests byte-identical.
fn null_if_empty<S: serde::Serializer>(value: &[String], serializer: S) -> Result<S::Ok, S::Error> {
    if value.is_empty() {
        serializer.serialize_none()
    } else {
        serializer.serialize_some(value)
    }
}

#[derive(Serialize)]
struct LifecycleClaimSummary {
    id: String,
    path: String,
    status: String,
    title: String,
    verified_digest: String,
    transitions: Vec<TransitionSummary>,
}

#[derive(Serialize)]
struct LifecycleManifest {
    workspace: String,
    generation: String,
    claims: Vec<LifecycleClaimSummary>,
    findings: Vec<String>,
    tree: Vec<TreeDigestEntry>,
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
        "lifecycle" => run_lifecycle(Path::new(&home), &workspace),
        "lifecycle-verify" => run_lifecycle_verify(Path::new(&home), &workspace),
        "index" => run_index(Path::new(&home), &workspace),
        "index-verify" => run_index_verify(Path::new(&home), &workspace),
        "ask" => run_ask(Path::new(&home), &workspace),
        "ask-verify" => run_ask_verify(Path::new(&home), &workspace),
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

fn lifecycle_store(paths: &Paths) -> zbrain::claims::ClaimStore {
    zbrain::claims::ClaimStore::with_clock(paths.clone(), std::sync::Arc::new(FixedClock::new(parity_now())))
}

fn run_lifecycle(home: &Path, workspace: &str) -> Result<(), String> {
    use zbrain::claims::{CLAIM_BASIS_OWNER, CLAIM_STATUS_DRAFT};

    let paths = parity_paths(home)?;
    let clock = FixedClock::new(parity_now());
    create_workspace(&paths, workspace, &clock).map_err(|err| format!("create workspace: {err}"))?;
    let store = lifecycle_store(&paths);
    let created = rfc3339(parity_now());
    store
        .write_draft(
            workspace,
            Claim {
                claim_type: OKF_CLAIM_TYPE.into(),
                id: "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa".into(),
                tier: "projects".into(),
                status: CLAIM_STATUS_DRAFT.into(),
                title: "Parity owner claim".into(),
                basis: CLAIM_BASIS_OWNER.into(),
                created_at: created.clone(),
                created_by: "owner".into(),
                body: "Parity body\n".into(),
                ..Claim::default()
            },
        )
        .map_err(|err| format!("write owner draft: {err}"))?;
    store
        .approve(workspace, "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
        .map_err(|err| format!("approve owner claim: {err}"))?;
    store
        .write_superseding_draft(
            workspace,
            "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            Claim {
                claim_type: OKF_CLAIM_TYPE.into(),
                id: "clm_cccccccccccccccccccccccccccccccc".into(),
                tier: "projects".into(),
                status: CLAIM_STATUS_DRAFT.into(),
                title: "Parity replacement claim".into(),
                basis: CLAIM_BASIS_OWNER.into(),
                created_at: created,
                created_by: "owner".into(),
                body: "Parity replacement body\n".into(),
                ..Claim::default()
            },
        )
        .map_err(|err| format!("write superseding draft: {err}"))?;
    store
        .approve(workspace, "clm_cccccccccccccccccccccccccccccccc")
        .map_err(|err| format!("approve replacement claim: {err}"))?;
    store
        .revoke(workspace, "clm_cccccccccccccccccccccccccccccccc", "wrong scope")
        .map_err(|err| format!("revoke replacement claim: {err}"))?;
    emit_lifecycle_manifest(&paths, workspace)
}

fn run_lifecycle_verify(home: &Path, workspace: &str) -> Result<(), String> {
    let paths = parity_paths(home)?;
    emit_lifecycle_manifest(&paths, workspace)
}

fn emit_lifecycle_manifest(paths: &Paths, workspace: &str) -> Result<(), String> {
    zbrain::boundary::validate_workspace(paths, workspace)
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
    let claims = scan
        .claims
        .iter()
        .map(|claim| LifecycleClaimSummary {
            id: claim.id.clone(),
            path: claim.path.clone(),
            status: claim.status.clone(),
            title: claim.title.clone(),
            verified_digest: claim.verified_digest.clone(),
            transitions: claim
                .transitions
                .iter()
                .map(|transition| TransitionSummary {
                    kind: transition.kind.clone(),
                    at: transition.at.clone(),
                    by: transition.by.clone(),
                    reason: transition.reason.clone(),
                    related_claim_ids: transition.related_claim_ids.clone(),
                    prior_verification_digest: transition.prior_verification_digest.clone(),
                })
                .collect(),
        })
        .collect();
    let findings = zbrain::lint::structural_findings(paths, workspace)
        .map_err(|err| format!("structural findings: {err}"))?;
    let root = zbrain::boundary::validate_workspace(paths, workspace)
        .map_err(|err| format!("validate workspace: {err}"))?;
    let generation = std::fs::read_to_string(root.join(".zbrain/generation.json"))
        .map_err(|err| format!("read generation: {err}"))?;
    let tree = walk_tree_digest(&paths.runtime_dir)?;
    emit(&LifecycleManifest {
        workspace: workspace.to_string(),
        generation,
        claims,
        findings,
        tree,
    })
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

// ---------------------------------------------------------------------------
// m4 index parity (mirrors fixture-gen --op index).
// ---------------------------------------------------------------------------

use zbrain::index::{IndexStore, IndexError, SearchOptions};
use zbrain::index_state::read_index_state;
use zbrain::query::{trusted_query, TrustedQueryOptions};

#[derive(Serialize)]
struct IndexSearchResult {
    id: String,
    path: String,
    #[serde(serialize_with = "zbrain::query::go_json_f64")]
    score: f64,
}

#[derive(Serialize)]
struct IndexSearchGroup {
    query: String,
    results: Vec<IndexSearchResult>,
}

#[derive(Serialize)]
struct IndexStateSummary {
    status: String,
    invalid_count: i64,
    manifest_digest: String,
    rebuilt_at: String,
}

#[derive(Serialize)]
struct ClaimStatusRow {
    id: String,
    status: String,
}

#[derive(Serialize)]
struct IndexManifest {
    workspace: String,
    summary: zbrain::index::IndexSummary,
    search: Vec<IndexSearchGroup>,
    claims: Vec<ClaimStatusRow>,
    state: IndexStateSummary,
    generation: String,
    check_fresh: String,
    tree: Vec<TreeDigestEntry>,
}

fn round_score(score: f64) -> f64 {
    (score * 1e6).round() / 1e6
}

fn is_derived_index_file(name: &str) -> bool {
    name.ends_with(".sqlite")
        || name.ends_with(".dirty")
        || name.contains(".sqlite-")
        || name.contains(".embeddings.sqlite.")
}

fn walk_tree_derived(runtime_dir: &Path) -> Result<Vec<TreeDigestEntry>, String> {
    let mut entries = BTreeMap::new();
    visit_derived(runtime_dir, runtime_dir, &mut entries)?;
    Ok(entries.into_values().collect())
}

fn visit_derived(
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
            let name = child.file_name().unwrap_or_default().to_string_lossy().to_string();
            if is_derived_index_file(&name) {
                ("file", String::new())
            } else {
                let contents = std::fs::read(&child).map_err(|err| format!("read {rel:?}: {err}"))?;
                let normalized = normalize_evidence_ids(&String::from_utf8_lossy(&contents));
                let digest = Sha256::digest(normalized.as_bytes());
                (
                    "file",
                    digest.iter().map(|b| format!("{b:02x}")).collect(),
                )
            }
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
            visit_derived(runtime_dir, &child, entries)?;
        }
    }
    Ok(())
}

fn index_seed_claims(paths: &Paths, workspace: &str) -> Result<(), String> {
    use zbrain::claims::{CLAIM_BASIS_OWNER, CLAIM_STATUS_DRAFT};
    let clock = FixedClock::new(parity_now());
    let created = rfc3339(parity_now());
    let store = ClaimStore::with_clock(paths.clone(), std::sync::Arc::new(clock));
    for (id, title, body, approve) in [
        (
            "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            "Parity Index Claim",
            "parity index trusted token\n",
            true,
        ),
        (
            "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
            "Parity Second Claim",
            "parity index second token\n",
            true,
        ),
        (
            "clm_dddddddddddddddddddddddddddddddd",
            "Parity Draft Claim",
            "parity index draft token\n",
            false,
        ),
    ] {
        store
            .write_draft(
                workspace,
                Claim {
                    claim_type: OKF_CLAIM_TYPE.into(),
                    id: id.into(),
                    tier: "projects".into(),
                    status: CLAIM_STATUS_DRAFT.into(),
                    title: title.into(),
                    basis: CLAIM_BASIS_OWNER.into(),
                    created_at: created.clone(),
                    created_by: "owner".into(),
                    tags: vec!["memory".into()],
                    body: body.into(),
                    ..Claim::default()
                },
            )
            .map_err(|err| format!("write {id}: {err}"))?;
        if approve {
            store.approve(workspace, id).map_err(|err| format!("approve {id}: {err}"))?;
        }
    }
    Ok(())
}

fn emit_index_facts(
    paths: &Paths,
    workspace: &str,
    summary: Option<zbrain::index::IndexSummary>,
) -> Result<(), String> {
    let idx = IndexStore::new(paths.clone());
    let queries = [
        "parity index",
        "parity index trusted",
        "parity index draft",
        "parity index second",
        "unmatchable sqlite",
    ];
    let mut search: Vec<IndexSearchGroup> = Vec::with_capacity(queries.len());
    for query in queries {
        let results = idx
            .search(
                workspace,
                SearchOptions {
                    query: query.into(),
                    statuses: vec![
                        zbrain::claims::CLAIM_STATUS_APPROVED.into(),
                        zbrain::claims::CLAIM_STATUS_DRAFT.into(),
                    ],
                    limit: 10,
                },
            )
            .map_err(|err| format!("search {query:?}: {err}"))?;
        search.push(IndexSearchGroup {
            query: query.into(),
            results: results
                .iter()
                .map(|result| IndexSearchResult {
                    id: result.id.clone(),
                    path: result.path.clone(),
                    score: round_score(result.score),
                })
                .collect(),
        });
    }

    let check_fresh = match idx.check_fresh(workspace) {
        Ok(()) => String::new(),
        Err(err) => err.to_string(),
    };

    let database_path = idx.database_path(workspace).map_err(|err| format!("database path: {err}"))?;
    let conn = rusqlite::Connection::open(&database_path)
        .map_err(|err| format!("open index: {err}"))?;
    let (_manifest, state) = read_index_state(&conn).map_err(|err| format!("read index state: {err}"))?;
    let mut statement = conn
        .prepare("select id, status from claims order by id")
        .map_err(|err| format!("query claims: {err}"))?;
    let mut rows = statement.query([]).map_err(|err| format!("query claims: {err}"))?;
    let mut claims: Vec<ClaimStatusRow> = Vec::new();
    while let Some(row) = rows.next().map_err(|err| format!("query claims: {err}"))? {
        claims.push(ClaimStatusRow {
            id: row.get(0).map_err(|err| format!("query claims: {err}"))?,
            status: row.get(1).map_err(|err| format!("query claims: {err}"))?,
        });
    }

    let root = zbrain::boundary::validate_workspace(paths, workspace)
        .map_err(|err| format!("validate workspace: {err}"))?;
    let generation = std::fs::read_to_string(root.join(".zbrain/generation.json"))
        .map_err(|err| format!("read generation: {err}"))?;
    let tree = walk_tree_derived(&paths.runtime_dir)?;

    emit(&IndexManifest {
        workspace: workspace.to_string(),
        summary: summary.unwrap_or_default(),
        search,
        claims,
        state: IndexStateSummary {
            status: state.status,
            invalid_count: state.invalid_count,
            manifest_digest: state.manifest_digest,
            rebuilt_at: "NORMALIZED".into(),
        },
        generation,
        check_fresh,
        tree,
    })
}

fn run_index(home: &Path, workspace: &str) -> Result<(), String> {
    let paths = parity_paths(home)?;
    let clock = FixedClock::new(parity_now());
    create_workspace(&paths, workspace, &clock).map_err(|err| format!("create workspace: {err}"))?;
    index_seed_claims(&paths, workspace)?;

    let idx = IndexStore::new(paths.clone());
    let mut summary = idx.rebuild(workspace).map_err(|err| format!("rebuild: {err}"))?;
    summary.rebuilt_at = "NORMALIZED".into();
    emit_index_facts(&paths, workspace, Some(summary))
}

// run_index_verify is read-only: it re-reads an index tree produced by either
// runtime, proving the SQLite file opens and searches identically cross-engine.
fn run_index_verify(home: &Path, workspace: &str) -> Result<(), String> {
    let paths = parity_paths(home)?;
    emit_index_facts(&paths, workspace, None)
}

// ---------------------------------------------------------------------------
// m4 ask parity (mirrors fixture-gen --op ask).
// ---------------------------------------------------------------------------

#[derive(Serialize)]
struct AskCase {
    name: String,
    error: String,
    response: Option<zbrain::query::TrustedQueryResponse>,
}

#[derive(Serialize)]
struct AskManifest {
    workspace: String,
    cases: Vec<AskCase>,
    generation: String,
    tree: Vec<TreeDigestEntry>,
}

fn normalize_home(mut value: String, paths: &Paths) -> String {
    let canonical_workspaces = std::fs::canonicalize(&paths.workspaces_dir)
        .map(|p| p.to_string_lossy().to_string());
    if let Ok(canonical) = canonical_workspaces {
        value = value.replace(&canonical, "HOME");
    }
    let canonical_runtime =
        std::fs::canonicalize(&paths.runtime_dir).map(|p| p.to_string_lossy().to_string());
    if let Ok(canonical) = canonical_runtime {
        value = value.replace(&canonical, "HOME");
    }
    value
        .replace(
            &paths.workspaces_dir.to_string_lossy().to_string(),
            "HOME",
        )
        .replace(&paths.runtime_dir.to_string_lossy().to_string(), "HOME")
}

fn with_rounded_scores(
    mut response: zbrain::query::TrustedQueryResponse,
) -> zbrain::query::TrustedQueryResponse {
    for claims in [&mut response.claims, &mut response.promotion_candidates] {
        for claim in claims.iter_mut().flatten() {
            claim.score = round_score(claim.score);
        }
    }
    response
}

fn run_ask(home: &Path, workspace: &str) -> Result<(), String> {
    use zbrain::claims::{CLAIM_BASIS_OWNER, CLAIM_STATUS_DRAFT};

    let paths = parity_paths(home)?;
    let clock = FixedClock::new(parity_now());
    create_workspace(&paths, workspace, &clock).map_err(|err| format!("create workspace: {err}"))?;
    create_workspace(&paths, "elsewhere", &clock)
        .map_err(|err| format!("create elsewhere workspace: {err}"))?;
    let created = rfc3339(parity_now());
    let store = ClaimStore::with_clock(paths.clone(), std::sync::Arc::new(clock));
    for (id, title, body, approve) in [
        (
            "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            "Parity uses SQLite for indexes",
            "parity ask trusted alpha token\n",
            true,
        ),
        (
            "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
            "Parity Second Claim",
            "parity ask trusted beta token\n",
            true,
        ),
        (
            "clm_dddddddddddddddddddddddddddddddd",
            "Parity uses BoltDB for indexes",
            "parity ask draft candidate token\n",
            false,
        ),
    ] {
        store
            .write_draft(
                workspace,
                Claim {
                    claim_type: OKF_CLAIM_TYPE.into(),
                    id: id.into(),
                    tier: "projects".into(),
                    status: CLAIM_STATUS_DRAFT.into(),
                    title: title.into(),
                    basis: CLAIM_BASIS_OWNER.into(),
                    created_at: created.clone(),
                    created_by: "owner".into(),
                    tags: vec!["memory".into()],
                    body: body.into(),
                    ..Claim::default()
                },
            )
            .map_err(|err| format!("write {id}: {err}"))?;
        if approve {
            store.approve(workspace, id).map_err(|err| format!("approve {id}: {err}"))?;
        }
    }

    let idx = IndexStore::new(paths.clone());
    idx.rebuild(workspace).map_err(|err| format!("rebuild: {err}"))?;

    let mut cases: Vec<AskCase> = Vec::new();
    let capture = |name: &str, result: Result<zbrain::query::TrustedQueryResponse, IndexError>, cases: &mut Vec<AskCase>| {
        match result {
            Ok(response) => cases.push(AskCase {
                name: name.into(),
                error: String::new(),
                response: Some(with_rounded_scores(response)),
            }),
            Err(err) => cases.push(AskCase {
                name: name.into(),
                error: normalize_home(err.to_string(), &paths),
                response: None,
            }),
        }
    };

    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            query: "parity ask trusted".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("happy", result, &mut cases);

    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            query: "parity ask draft".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("draft-conflict", result, &mut cases);

    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            query: "unmatchable".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("gap", result, &mut cases);

    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            query: "parity ask trusted".into(),
            as_of: "2026-01-01T00:00:00Z".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("as-of-early", result, &mut cases);

    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            workspace: "elsewhere".into(),
            query: "parity ask trusted".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("missing-index", result, &mut cases);

    idx.mark_dirty(workspace).map_err(|err| format!("mark dirty: {err}"))?;
    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            query: "parity ask trusted".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("dirty", result, &mut cases);

    idx.rebuild(workspace).map_err(|err| format!("recovery rebuild: {err}"))?;
    let claim_path = paths
        .workspaces_dir
        .join(workspace)
        .join("wiki/projects/clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.md");
    let contents = std::fs::read_to_string(&claim_path).map_err(|err| format!("read claim: {err}"))?;
    let changed = contents.replacen(
        "parity ask trusted alpha",
        "parity ask stale alpha",
        1,
    );
    std::fs::write(&claim_path, changed).map_err(|err| format!("edit claim: {err}"))?;
    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            query: "parity ask trusted".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("stale", result, &mut cases);

    let root = zbrain::boundary::validate_workspace(&paths, workspace)
        .map_err(|err| format!("validate workspace: {err}"))?;
    let generation = std::fs::read_to_string(root.join(".zbrain/generation.json"))
        .map_err(|err| format!("read generation: {err}"))?;
    let tree = walk_tree_derived(&paths.runtime_dir)?;

    emit(&AskManifest {
        workspace: workspace.to_string(),
        cases,
        generation,
        tree,
    })
}

fn run_ask_verify(home: &Path, workspace: &str) -> Result<(), String> {
    let paths = parity_paths(home)?;
    let mut cases: Vec<AskCase> = Vec::new();
    let capture = |name: &str, result: Result<zbrain::query::TrustedQueryResponse, IndexError>, cases: &mut Vec<AskCase>| {
        match result {
            Ok(response) => cases.push(AskCase {
                name: name.into(),
                error: String::new(),
                response: Some(with_rounded_scores(response)),
            }),
            Err(err) => cases.push(AskCase {
                name: name.into(),
                error: normalize_home(err.to_string(), &paths),
                response: None,
            }),
        }
    };

    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            query: "parity ask trusted".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("happy", result, &mut cases);
    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            query: "parity ask draft".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("draft-conflict", result, &mut cases);
    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            query: "unmatchable".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("gap", result, &mut cases);
    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            query: "parity ask trusted".into(),
            as_of: "2026-01-01T00:00:00Z".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("as-of-early", result, &mut cases);
    let result = trusted_query(
        &paths,
        TrustedQueryOptions {
            workspace: "elsewhere".into(),
            query: "parity ask trusted".into(),
            limit: 10,
            ..TrustedQueryOptions::default()
        },
    );
    capture("missing-index", result, &mut cases);

    let root = zbrain::boundary::validate_workspace(&paths, workspace)
        .map_err(|err| format!("validate workspace: {err}"))?;
    let generation = std::fs::read_to_string(root.join(".zbrain/generation.json"))
        .map_err(|err| format!("read generation: {err}"))?;
    let tree = walk_tree_derived(&paths.runtime_dir)?;
    emit(&AskManifest {
        workspace: workspace.to_string(),
        cases,
        generation,
        tree,
    })
}
