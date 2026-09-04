// fixture-gen drives the Go (oracle) runtime through the m0 surface and
// emits a normalized manifest of the resulting runtime tree. The Rust
// zbrain-parity binary emits the same schema; scripts/parity.sh diffs them.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

type treeEntry struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Mode string `json:"mode"`
}

type manifest struct {
	Config        string      `json:"config"`
	WorkspaceMD   string      `json:"workspace_md"`
	EvidenceIndex string      `json:"evidence_index"`
	Tree          []treeEntry `json:"tree"`
	DefaultRead   string      `json:"default_read"`
	GenerationRel string      `json:"generation_rel"`
}

type setupManifest struct {
	SchemaVersion      int         `json:"schema_version"`
	ConfigCreated      bool        `json:"config_created"`
	AssetsCopied       int         `json:"assets_copied"`
	AssetsSkipped      int         `json:"assets_skipped"`
	Tree               []treeEntry `json:"tree"`
	RuntimeVersionLine string      `json:"runtime_version_line"`
}

// m2 claims/evidence parity: deterministic claim store + evidence snapshot
// tree, compared byte-for-byte between the Go oracle and the Rust port.
type claimSummary struct {
	ID             string `json:"id"`
	Path           string `json:"path"`
	Status         string `json:"status"`
	Title          string `json:"title"`
	VerifiedDigest string `json:"verified_digest"`
}

type evidenceSummary struct {
	ID         string `json:"id"`
	Origin     string `json:"origin"`
	CapturedAt string `json:"captured_at"`
	MediaType  string `json:"media_type"`
	ByteLength int64  `json:"byte_length"`
	SHA256     string `json:"sha256"`
}

type treeDigestEntry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
}

type claimsManifest struct {
	Workspace  string            `json:"workspace"`
	Generation string            `json:"generation"`
	Claims     []claimSummary    `json:"claims"`
	Evidence   []evidenceSummary `json:"evidence"`
	Tree       []treeDigestEntry `json:"tree"`
}

// evdPattern mirrors the oracle's evidenceIDPattern (^evd_<32 hex>$); only
// used to normalize random snapshot IDs for cross-tree comparison.
var evdPattern = regexp.MustCompile(`evd_[0-9a-f]{32}`)

func isEvidenceIDShaped(value string) bool {
	rest, ok := strings.CutPrefix(value, "evd_")
	if !ok || len(rest) != 32 {
		return false
	}
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func normalizeEvidenceIDs(value string) string {
	return evdPattern.ReplaceAllString(value, "evd_NORMALIZED")
}

func parityNow() time.Time {
	return time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
}

func parityPaths(home string) (zruntime.Paths, error) {
	paths, err := zruntime.ResolvePaths(zruntime.Options{
		CWD:        home,
		HomeDir:    home,
		RuntimeDir: filepath.Join(home, "runtime"),
	})
	if err != nil {
		return zruntime.Paths{}, fmt.Errorf("resolve paths: %w", err)
	}
	if _, err := zruntime.EnsureConfig(paths.ConfigFile); err != nil {
		return zruntime.Paths{}, fmt.Errorf("ensure config: %w", err)
	}
	return paths, nil
}

func runClaims(home, workspace string) error {
	paths, err := parityPaths(home)
	if err != nil {
		return err
	}
	if err := zruntime.CreateWorkspace(paths, workspace, parityNow()); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	sourcePath := filepath.Join(home, "source.txt")
	if err := os.WriteFile(sourcePath, []byte("parity evidence payload\n"), 0o644); err != nil {
		return fmt.Errorf("write parity source: %w", err)
	}
	evidence, err := (zruntime.EvidenceStore{Paths: paths, Now: parityNow}).AddFile(workspace, sourcePath, "file://source.txt", "text/plain")
	if err != nil {
		return fmt.Errorf("add evidence: %w", err)
	}
	store := zruntime.ClaimStore{Paths: paths, Now: parityNow}
	created := parityNow().UTC().Format(time.RFC3339)
	ownerClaim := zruntime.Claim{
		Type:      zruntime.OKFClaimType,
		ID:        "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Tier:      "projects",
		Status:    zruntime.ClaimStatusDraft,
		Title:     "Parity owner claim",
		Basis:     zruntime.ClaimBasisOwner,
		CreatedAt: created,
		CreatedBy: "owner",
		Body:      "Parity body\n",
	}
	if _, err := store.WriteDraft(workspace, ownerClaim); err != nil {
		return fmt.Errorf("write owner draft: %w", err)
	}
	evidenceClaim := zruntime.Claim{
		Type:        zruntime.OKFClaimType,
		ID:          "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Tier:        "projects",
		Status:      zruntime.ClaimStatusDraft,
		Title:       "Parity evidence claim",
		Basis:       zruntime.ClaimBasisEvidence,
		CreatedAt:   created,
		CreatedBy:   "owner",
		EvidenceIDs: []string{evidence.ID},
		Body:        "Parity evidence body\n",
	}
	if _, err := store.WriteDraft(workspace, evidenceClaim); err != nil {
		return fmt.Errorf("write evidence draft: %w", err)
	}
	return emitClaimsManifest(paths, workspace, false)
}

func runClaimsVerify(home, workspace string) error {
	paths, err := parityPaths(home)
	if err != nil {
		return err
	}
	return emitClaimsManifest(paths, workspace, true)
}

func emitClaimsManifest(paths zruntime.Paths, workspace string, verify bool) error {
	root, err := zruntime.ValidateWorkspace(paths, workspace)
	if err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	scan, err := (zruntime.ClaimStore{Paths: paths}).ScanWorkspace(workspace)
	if err != nil {
		return fmt.Errorf("scan workspace: %w", err)
	}
	if len(scan.Invalid) != 0 {
		return fmt.Errorf("workspace scan reported invalid claims: %v", scan.Invalid)
	}

	claims := make([]claimSummary, 0, len(scan.Claims))
	for _, claim := range scan.Claims {
		claims = append(claims, claimSummary{
			ID:             claim.ID,
			Path:           claim.Path,
			Status:         string(claim.Status),
			Title:          claim.Title,
			VerifiedDigest: claim.VerifiedDigest,
		})
	}

	sourcesRoot := filepath.Join(root, "evidence", "sources")
	entries, err := os.ReadDir(sourcesRoot)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read evidence sources: %w", err)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && isEvidenceIDShaped(entry.Name()) {
			ids = append(ids, entry.Name())
		}
	}
	sort.Strings(ids)
	validator, err := zruntime.NewEvidenceValidator(zruntime.EvidenceStore{Paths: paths}, workspace)
	if err != nil {
		return fmt.Errorf("new evidence validator: %w", err)
	}
	evidenceList := make([]evidenceSummary, 0, len(ids))
	for _, id := range ids {
		evidence, err := (zruntime.EvidenceStore{Paths: paths}).Read(workspace, id)
		if err != nil {
			return fmt.Errorf("read evidence %s: %w", id, err)
		}
		if verify {
			if err := validator.Verify(id); err != nil {
				return fmt.Errorf("verify evidence %s: %w", id, err)
			}
		}
		evidenceList = append(evidenceList, evidenceSummary{
			ID:         normalizeEvidenceIDs(evidence.ID),
			Origin:     evidence.Origin,
			CapturedAt: evidence.CapturedAt,
			MediaType:  evidence.MediaType,
			ByteLength: evidence.ByteLength,
			SHA256:     evidence.SHA256,
		})
	}

	generation, err := os.ReadFile(filepath.Join(root, ".zbrain", "generation.json"))
	if err != nil {
		return fmt.Errorf("read generation: %w", err)
	}
	tree, err := walkTreeDigest(paths.RuntimeDir)
	if err != nil {
		return err
	}

	out, err := json.MarshalIndent(claimsManifest{
		Workspace:  workspace,
		Generation: string(generation),
		Claims:     claims,
		Evidence:   evidenceList,
		Tree:       tree,
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func walkTreeDigest(runtimeDir string) ([]treeDigestEntry, error) {
	var tree []treeDigestEntry
	err := filepath.WalkDir(runtimeDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(runtimeDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		kind := "file"
		sum := ""
		if info.IsDir() {
			kind = "dir"
		} else {
			contents, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %q: %w", rel, err)
			}
			digest := sha256.Sum256([]byte(normalizeEvidenceIDs(string(contents))))
			sum = hex.EncodeToString(digest[:])
		}
		tree = append(tree, treeDigestEntry{
			Path:   normalizeEvidenceIDs(rel),
			Kind:   kind,
			Mode:   fmt.Sprintf("%04o", info.Mode().Perm()),
			SHA256: sum,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk tree: %w", err)
	}
	sort.Slice(tree, func(i, j int) bool { return tree[i].Path < tree[j].Path })
	return tree, nil
}

// m3 lifecycle parity: deterministic draft → approve → supersede → revoke
// chain compared byte-for-byte between the Go oracle and the Rust port.
type transitionSummary struct {
	Kind                    string   `json:"kind"`
	At                      string   `json:"at"`
	By                      string   `json:"by"`
	Reason                  string   `json:"reason,omitempty"`
	RelatedClaimIDs         []string `json:"related_claim_ids"`
	PriorVerificationDigest string   `json:"prior_verification_digest,omitempty"`
}

type lifecycleClaimSummary struct {
	ID             string              `json:"id"`
	Path           string              `json:"path"`
	Status         string              `json:"status"`
	Title          string              `json:"title"`
	VerifiedDigest string              `json:"verified_digest"`
	Transitions    []transitionSummary `json:"transitions"`
}

type lifecycleManifest struct {
	Workspace  string                  `json:"workspace"`
	Generation string                  `json:"generation"`
	Claims     []lifecycleClaimSummary `json:"claims"`
	Findings   []string                `json:"findings"`
	Tree       []treeDigestEntry       `json:"tree"`
}

func runLifecycle(home, workspace string) error {
	paths, err := parityPaths(home)
	if err != nil {
		return err
	}
	if err := zruntime.CreateWorkspace(paths, workspace, parityNow()); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	store := zruntime.ClaimStore{Paths: paths, Now: parityNow}
	created := parityNow().UTC().Format(time.RFC3339)
	owner := zruntime.Claim{
		Type:      zruntime.OKFClaimType,
		ID:        "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Tier:      "projects",
		Status:    zruntime.ClaimStatusDraft,
		Title:     "Parity owner claim",
		Basis:     zruntime.ClaimBasisOwner,
		CreatedAt: created,
		CreatedBy: "owner",
		Body:      "Parity body\n",
	}
	if _, err := store.WriteDraft(workspace, owner); err != nil {
		return fmt.Errorf("write owner draft: %w", err)
	}
	if _, err := store.Approve(workspace, "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		return fmt.Errorf("approve owner claim: %w", err)
	}
	replacement := zruntime.Claim{
		Type:      zruntime.OKFClaimType,
		ID:        "clm_cccccccccccccccccccccccccccccccc",
		Tier:      "projects",
		Status:    zruntime.ClaimStatusDraft,
		Title:     "Parity replacement claim",
		Basis:     zruntime.ClaimBasisOwner,
		CreatedAt: created,
		CreatedBy: "owner",
		Body:      "Parity replacement body\n",
	}
	if _, err := store.WriteSupersedingDraft(workspace, "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", replacement); err != nil {
		return fmt.Errorf("write superseding draft: %w", err)
	}
	if _, err := store.Approve(workspace, "clm_cccccccccccccccccccccccccccccccc"); err != nil {
		return fmt.Errorf("approve replacement claim: %w", err)
	}
	if _, err := store.Revoke(workspace, "clm_cccccccccccccccccccccccccccccccc", "wrong scope"); err != nil {
		return fmt.Errorf("revoke replacement claim: %w", err)
	}
	return emitLifecycleManifest(paths, workspace)
}

func runLifecycleVerify(home, workspace string) error {
	paths, err := parityPaths(home)
	if err != nil {
		return err
	}
	return emitLifecycleManifest(paths, workspace)
}

func emitLifecycleManifest(paths zruntime.Paths, workspace string) error {
	if _, err := zruntime.ValidateWorkspace(paths, workspace); err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	scan, err := (zruntime.ClaimStore{Paths: paths}).ScanWorkspace(workspace)
	if err != nil {
		return fmt.Errorf("scan workspace: %w", err)
	}
	if len(scan.Invalid) != 0 {
		return fmt.Errorf("workspace scan reported invalid claims: %v", scan.Invalid)
	}
	claims := make([]lifecycleClaimSummary, 0, len(scan.Claims))
	for _, claim := range scan.Claims {
		transitions := make([]transitionSummary, 0, len(claim.Transitions))
		for _, transition := range claim.Transitions {
			transitions = append(transitions, transitionSummary{
				Kind:                    string(transition.Kind),
				At:                      transition.At,
				By:                      transition.By,
				Reason:                  transition.Reason,
				RelatedClaimIDs:         append([]string(nil), transition.RelatedClaimIDs...),
				PriorVerificationDigest: transition.PriorVerificationDigest,
			})
		}
		claims = append(claims, lifecycleClaimSummary{
			ID:             claim.ID,
			Path:           claim.Path,
			Status:         string(claim.Status),
			Title:          claim.Title,
			VerifiedDigest: claim.VerifiedDigest,
			Transitions:    transitions,
		})
	}
	findings, err := zruntime.StructuralFindings(paths, workspace)
	if err != nil {
		return fmt.Errorf("structural findings: %w", err)
	}
	root, err := zruntime.ValidateWorkspace(paths, workspace)
	if err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	generation, err := os.ReadFile(filepath.Join(root, ".zbrain", "generation.json"))
	if err != nil {
		return fmt.Errorf("read generation: %w", err)
	}
	tree, err := walkTreeDigest(paths.RuntimeDir)
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(lifecycleManifest{
		Workspace:  workspace,
		Generation: string(generation),
		Claims:     claims,
		Findings:   findings,
		Tree:       tree,
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func main() {
	home := flag.String("home", "", "runtime home directory (required)")
	workspace := flag.String("workspace", "research", "workspace name to create")
	op := flag.String("op", "workspace", "operation to exercise (workspace|setup|claims|claims-verify|lifecycle|lifecycle-verify)")
	flag.Parse()
	if *home == "" {
		fail("home is required")
	}
	var err error
	switch *op {
	case "workspace":
		err = runWorkspace(*home, *workspace)
	case "setup":
		err = runSetup(*home)
	case "claims":
		err = runClaims(*home, *workspace)
	case "claims-verify":
		err = runClaimsVerify(*home, *workspace)
	case "lifecycle":
		err = runLifecycle(*home, *workspace)
	case "lifecycle-verify":
		err = runLifecycleVerify(*home, *workspace)
	default:
		fail("unknown op " + *op)
	}
	if err != nil {
		fail(err.Error())
	}
}

func runWorkspace(home, workspace string) error {
	paths, err := zruntime.ResolvePaths(zruntime.Options{
		CWD:        home,
		HomeDir:    home,
		RuntimeDir: filepath.Join(home, "runtime"),
	})
	if err != nil {
		return fmt.Errorf("resolve paths: %w", err)
	}
	if _, err := zruntime.EnsureConfig(paths.ConfigFile); err != nil {
		return fmt.Errorf("ensure config: %w", err)
	}
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	if err := zruntime.CreateWorkspace(paths, workspace, now); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	current, err := zruntime.ResolveCurrentWorkspace(paths)
	if err != nil {
		return fmt.Errorf("resolve current: %w", err)
	}

	root := filepath.Join(paths.WorkspacesDir, workspace)
	config, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	workspaceMD, err := os.ReadFile(filepath.Join(root, "workspace.md"))
	if err != nil {
		return fmt.Errorf("read workspace.md: %w", err)
	}
	evidenceIndex, err := os.ReadFile(filepath.Join(root, "evidence", "_index.md"))
	if err != nil {
		return fmt.Errorf("read evidence/_index.md: %w", err)
	}

	tree, err := walkTree(paths.RuntimeDir)
	if err != nil {
		return err
	}
	resolvedRoot, err := zruntime.ValidateWorkspace(paths, workspace)
	if err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	genPath, err := zruntime.IndexStore{Paths: paths}.GenerationPath(workspace)
	if err != nil {
		return fmt.Errorf("generation path: %w", err)
	}
	controlRel, err := filepath.Rel(resolvedRoot, genPath)
	if err != nil {
		return fmt.Errorf("rel generation path: %w", err)
	}

	out, err := json.MarshalIndent(manifest{
		Config:        string(config),
		WorkspaceMD:   string(workspaceMD),
		EvidenceIndex: string(evidenceIndex),
		Tree:          tree,
		DefaultRead:   current.Workspace,
		GenerationRel: controlRel,
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func runSetup(home string) error {
	paths, err := zruntime.ResolvePaths(zruntime.Options{
		CWD:        home,
		HomeDir:    home,
		RuntimeDir: filepath.Join(home, "runtime"),
	})
	if err != nil {
		return fmt.Errorf("resolve paths: %w", err)
	}
	if err := os.MkdirAll(paths.RuntimeDir, 0o755); err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}
	created, err := zruntime.EnsureConfig(paths.ConfigFile)
	if err != nil {
		return fmt.Errorf("ensure config: %w", err)
	}
	extracted, err := zruntime.ExtractBundledAssets(paths)
	if err != nil {
		return fmt.Errorf("extract assets: %w", err)
	}
	tree, err := walkTree(paths.RuntimeDir)
	if err != nil {
		return err
	}
	config, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	versionLine := ""
	for _, line := range strings.Split(string(config), "\n") {
		if strings.HasPrefix(line, "runtime_version:") {
			versionLine = line
			break
		}
	}

	out, err := json.MarshalIndent(setupManifest{
		SchemaVersion:      1,
		ConfigCreated:      created,
		AssetsCopied:       len(extracted.Copied),
		AssetsSkipped:      len(extracted.Skipped),
		Tree:               tree,
		RuntimeVersionLine: versionLine,
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func walkTree(runtimeDir string) ([]treeEntry, error) {
	var tree []treeEntry
	err := filepath.WalkDir(runtimeDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(runtimeDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		kind := "file"
		if info.IsDir() {
			kind = "dir"
		}
		tree = append(tree, treeEntry{Path: rel, Kind: kind, Mode: fmt.Sprintf("%04o", info.Mode().Perm())})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk tree: %w", err)
	}
	sort.Slice(tree, func(i, j int) bool { return tree[i].Path < tree[j].Path })
	return tree, nil
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "fixture-gen: "+message)
	os.Exit(1)
}
