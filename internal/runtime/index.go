package runtime

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type IndexStore struct {
	Paths Paths
}

type IndexSummary struct {
	Workspace      string         `json:"workspace"`
	Approved       int            `json:"approved"`
	Draft          int            `json:"draft"`
	Invalid        int            `json:"invalid"`
	InvalidCount   int            `json:"invalid_count"`
	InvalidClaims  []InvalidClaim `json:"invalid_claims,omitempty"`
	Legacy         int            `json:"legacy"`
	RebuildState   RebuildStatus  `json:"rebuild_state"`
	ManifestDigest string         `json:"manifest_digest"`
	RebuiltAt      string         `json:"rebuilt_at"`
}

type SearchOptions struct {
	Query    string
	Statuses []ClaimStatus
	Limit    int
}

type IndexedClaim struct {
	ID          string      `json:"id"`
	Path        string      `json:"path"`
	Tier        string      `json:"tier"`
	Type        string      `json:"type"`
	Status      ClaimStatus `json:"status"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	StaleAfter  string      `json:"stale_after,omitempty"`
	Score       float64     `json:"score"`
}

func (store IndexStore) DatabasePath(workspace string) (string, error) {
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return "", err
	}
	return store.databasePath(workspace), nil
}

func (store IndexStore) DirtyPath(workspace string) (string, error) {
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return "", err
	}
	return store.dirtyPath(workspace), nil
}

func (store IndexStore) databasePath(workspace string) string {
	return filepath.Join(store.Paths.IndexesDir, workspace+".sqlite")
}

func (store IndexStore) dirtyPath(workspace string) string {
	return filepath.Join(store.Paths.IndexesDir, workspace+".dirty")
}

func (store IndexStore) validatedIndexPaths(workspace string) (string, string, error) {
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return "", "", err
	}
	databasePath := store.databasePath(workspace)
	dirtyPath := store.dirtyPath(workspace)
	if err := validateIndexBoundaryPath(store.Paths.IndexesDir, true); err != nil {
		return "", "", fmt.Errorf("validate index directory: %w", err)
	}
	for _, path := range []string{databasePath, dirtyPath} {
		if err := validateIndexBoundaryPath(path, false); err != nil {
			return "", "", fmt.Errorf("validate index path %q: %w", path, err)
		}
	}
	return databasePath, dirtyPath, nil
}

func validateIndexBoundaryPath(path string, directory bool) error {
	clean, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	candidate := clean
	for {
		info, err := os.Lstat(candidate)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("%q must not be a symlink", candidate)
			}
			resolved, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				return err
			}
			resolved, err = filepath.Abs(resolved)
			if err != nil {
				return err
			}
			if resolved != candidate {
				return fmt.Errorf("%q contains a symlink", path)
			}
			if candidate == clean && directory && !info.IsDir() {
				return fmt.Errorf("%q is not a directory", path)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return fmt.Errorf("%q has no existing ancestor", path)
		}
		candidate = parent
	}
}

func (store IndexStore) AssertFTS5() error {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer db.Close()
	var enabled int
	if err := db.QueryRow("select sqlite_compileoption_used('ENABLE_FTS5')").Scan(&enabled); err != nil {
		return err
	}
	if enabled != 1 {
		return fmt.Errorf("sqlite ENABLE_FTS5 compile option is not available")
	}
	if _, err := db.Exec("create virtual table fts_probe using fts5(body)"); err != nil {
		return fmt.Errorf("create FTS5 probe table: %w", err)
	}
	return nil
}

func (store IndexStore) MarkDirty(workspace string) error {
	_, dirtyPath, err := store.validatedIndexPaths(workspace)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(store.Paths.IndexesDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(dirtyPath, []byte("dirty\n"), 0o644)
}

func (store IndexStore) CheckFresh(workspace string) error {
	databasePath, dirtyPath, err := store.validatedIndexPaths(workspace)
	if err != nil {
		return err
	}
	if err := CheckPendingTransition(store.Paths, workspace); err != nil {
		return err
	}
	if _, err := os.Stat(dirtyPath); err == nil {
		return fmt.Errorf("workspace %q index is dirty; run zbrain reindex", workspace)
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(databasePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("workspace %q index does not exist; run zbrain reindex", workspace)
		}
		return err
	}

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return fmt.Errorf("workspace %q index cannot be opened: %w", workspace, err)
	}
	defer db.Close()
	manifest, state, err := ReadIndexState(db)
	if err != nil {
		return fmt.Errorf("workspace %q index state is malformed or missing: %w", workspace, err)
	}
	if state.Status == RebuildStatusRejected {
		return fmt.Errorf("workspace %q index is rejected: %d invalid trust inputs; run zbrain reindex", workspace, state.InvalidCount)
	}
	if state.Status != RebuildStatusClean {
		return fmt.Errorf("workspace %q index state is malformed: unsupported rebuild status %q", workspace, state.Status)
	}

	workspaceRoot, err := ValidateWorkspace(store.Paths, workspace)
	if err != nil {
		return err
	}
	recordedMtimes, err := readTrustInputMtimes(db)
	if err != nil {
		return fmt.Errorf("workspace %q index freshness metadata is missing or malformed: %w; run zbrain reindex", workspace, err)
	}
	if len(recordedMtimes) != len(manifest.Entries) {
		return fmt.Errorf("workspace %q index freshness metadata does not match trust inputs; run zbrain reindex", workspace)
	}
	directories, err := readTrustDirectories(db)
	if err != nil {
		return fmt.Errorf("workspace %q index freshness metadata is missing or malformed: %w; run zbrain reindex", workspace, err)
	}
	for _, recordedDirectory := range directories {
		directory := filepath.Join(workspaceRoot, filepath.FromSlash(recordedDirectory.Path))
		info, err := os.Lstat(directory)
		if os.IsNotExist(err) {
			return fmt.Errorf("workspace %q index is stale; trust directory %q is missing; run zbrain reindex", workspace, directory)
		}
		if err != nil {
			return fmt.Errorf("workspace %q index freshness check failed for %q: %w", workspace, directory, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace %q index freshness check failed: trust directory %q must not be a symlink", workspace, directory)
		}
		if !info.IsDir() {
			return fmt.Errorf("workspace %q index freshness check failed: trust directory %q is not a directory", workspace, directory)
		}
		if info.ModTime().UnixNano() == recordedDirectory.ModifiedAt {
			continue
		}
		offender, err := findFreshnessOffender(workspaceRoot, directory, recordedMtimes)
		if err != nil {
			return fmt.Errorf("workspace %q index freshness check failed: %w", workspace, err)
		}
		if offender != "" {
			return fmt.Errorf("workspace %q index is stale; trust input %q changed after index; run zbrain reindex", workspace, offender)
		}
	}

	for _, entry := range manifest.Entries {
		recordedMtime, ok := recordedMtimes[entry.Path]
		if !ok {
			return fmt.Errorf("workspace %q index freshness metadata is missing for %q; run zbrain reindex", workspace, entry.Path)
		}
		inputPath := filepath.Join(workspaceRoot, filepath.FromSlash(entry.Path))
		info, err := os.Lstat(inputPath)
		if os.IsNotExist(err) {
			return fmt.Errorf("workspace %q index is stale; trust input %q is missing; run zbrain reindex", workspace, inputPath)
		}
		if err != nil {
			return fmt.Errorf("workspace %q index freshness check failed for %q: %w", workspace, inputPath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace %q index freshness check failed: trust input %q must not be a symlink", workspace, inputPath)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("workspace %q index freshness check failed: trust input %q is not a regular file", workspace, inputPath)
		}
		if info.ModTime().UnixNano() != recordedMtime {
			return fmt.Errorf("workspace %q index is stale; trust input %q changed after index; run zbrain reindex", workspace, inputPath)
		}
	}
	return nil
}

var errFreshnessOffender = errors.New("freshness offender found")

type trustInputMtime struct {
	Path       string
	ModifiedAt int64
}

type trustDirectoryMtime struct {
	Path       string
	ModifiedAt int64
}

func readTrustInputMtimes(db *sql.DB) (map[string]int64, error) {
	rows, err := db.Query("select path, modified_at from trust_input_mtimes order by path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mtimes := make(map[string]int64)
	previous := ""
	for rows.Next() {
		var path string
		var modifiedAt int64
		if err := rows.Scan(&path, &modifiedAt); err != nil {
			return nil, err
		}
		if strings.ContainsAny(path, "\\\\\x00") {
			return nil, fmt.Errorf("input path %q is not slash-normalized", path)
		}
		if _, err := safeRelativePath(path); err != nil || !isTrustInputPath(path) {
			return nil, fmt.Errorf("input path %q is not a canonical trust input", path)
		}
		if previous != "" && path <= previous {
			return nil, fmt.Errorf("trust input mtimes are not unique and sorted at %q", path)
		}
		mtimes[path] = modifiedAt
		previous = path
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return mtimes, nil
}

func readTrustDirectories(db *sql.DB) ([]trustDirectoryMtime, error) {
	rows, err := db.Query("select path, modified_at from trust_directories order by path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	directories := make([]trustDirectoryMtime, 0)
	previous := ""
	for rows.Next() {
		var directory trustDirectoryMtime
		if err := rows.Scan(&directory.Path, &directory.ModifiedAt); err != nil {
			return nil, err
		}
		if strings.ContainsAny(directory.Path, "\\\\\x00") {
			return nil, fmt.Errorf("directory path %q is not slash-normalized", directory.Path)
		}
		if _, err := safeRelativePath(directory.Path); err != nil || !isTrustDirectoryPath(directory.Path) {
			return nil, fmt.Errorf("directory path %q is not a canonical trust directory", directory.Path)
		}
		if previous != "" && directory.Path <= previous {
			return nil, fmt.Errorf("trust directories are not unique and sorted at %q", directory.Path)
		}
		directories = append(directories, directory)
		previous = directory.Path
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return directories, nil
}

func findFreshnessOffender(workspaceRoot string, directory string, knownInputMtimes map[string]int64) (string, error) {
	var offender string
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(workspaceRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("trust input %q must not be a symlink", path)
		}
		if entry.IsDir() {
			if isTrustInputPath(relative) {
				return fmt.Errorf("trust input %q is not a regular file", path)
			}
			return nil
		}
		if !isTrustInputPath(relative) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("trust input %q is not a regular file", path)
		}
		recordedMtime, known := knownInputMtimes[relative]
		if !known || info.ModTime().UnixNano() != recordedMtime {
			offender = path
			return errFreshnessOffender
		}
		return nil
	})
	if errors.Is(err, errFreshnessOffender) {
		return offender, nil
	}
	return "", err
}

func collectTrustInputMtimes(paths Paths, workspace string, manifest TrustInputManifest) ([]trustInputMtime, error) {
	root, err := ValidateWorkspace(paths, workspace)
	if err != nil {
		return nil, err
	}
	mtimes := make([]trustInputMtime, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if _, err := safeRelativePath(entry.Path); err != nil {
			return nil, err
		}
		path := filepath.Join(root, filepath.FromSlash(entry.Path))
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("trust input %q is not a regular file", path)
		}
		mtimes = append(mtimes, trustInputMtime{Path: entry.Path, ModifiedAt: info.ModTime().UnixNano()})
	}
	return mtimes, nil
}

func collectTrustDirectories(paths Paths, workspace string) ([]trustDirectoryMtime, error) {
	root, err := ValidateWorkspace(paths, workspace)
	if err != nil {
		return nil, err
	}
	directorySet := make(map[string]int64)
	add := func(path string) error {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if !pathWithin(root, absolute) {
			return fmt.Errorf("trust directory %q is outside workspace", path)
		}
		info, err := os.Lstat(absolute)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("trust directory %q is not a directory", path)
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !isTrustDirectoryPath(relative) {
			return fmt.Errorf("trust directory %q is not canonical", path)
		}
		directorySet[relative] = info.ModTime().UnixNano()
		return nil
	}

	for _, tier := range WikiTiers {
		tierRoot := filepath.Join(root, "wiki", tier)
		if err := filepath.WalkDir(tierRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("trust directory %q must not be a symlink", path)
			}
			if !entry.IsDir() {
				return nil
			}
			return add(path)
		}); err != nil {
			return nil, err
		}
	}

	sourcesRoot := filepath.Join(root, "evidence", "sources")
	info, err := os.Lstat(sourcesRoot)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("trust directory %q must not be a symlink", sourcesRoot)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("trust directory %q is not a directory", sourcesRoot)
	}
	if err := add(sourcesRoot); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(sourcesRoot)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		path := filepath.Join(sourcesRoot, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("trust directory %q must not be a symlink", path)
		}
		if entry.IsDir() {
			if err := add(path); err != nil {
				return nil, err
			}
		}
	}

	directories := make([]trustDirectoryMtime, 0, len(directorySet))
	for path, modifiedAt := range directorySet {
		directories = append(directories, trustDirectoryMtime{Path: path, ModifiedAt: modifiedAt})
	}
	sort.Slice(directories, func(i, j int) bool {
		return directories[i].Path < directories[j].Path
	})
	return directories, nil
}

func isTrustDirectoryPath(path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[0] == "wiki" && isKnownWikiTier(parts[1]) {
		return true
	}
	return path == "evidence/sources" || (len(parts) == 3 && parts[0] == "evidence" && parts[1] == "sources" && parts[2] != "")
}

func isTrustInputPath(path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) >= 3 && parts[0] == "wiki" && isKnownWikiTier(parts[1]) && filepath.Ext(path) == ".md" {
		return true
	}
	return len(parts) == 4 && parts[0] == "evidence" && parts[1] == "sources" && parts[2] != "" && (parts[3] == "source.yaml" || parts[3] == "raw")
}

func writeTrustInputMtimes(tx *sql.Tx, mtimes []trustInputMtime) error {
	if _, err := tx.Exec("delete from trust_input_mtimes"); err != nil {
		return fmt.Errorf("clear trust input mtimes: %w", err)
	}
	for _, mtime := range mtimes {
		if _, err := tx.Exec("insert into trust_input_mtimes(path, modified_at) values (?, ?)", mtime.Path, mtime.ModifiedAt); err != nil {
			return fmt.Errorf("write trust input mtime %q: %w", mtime.Path, err)
		}
	}
	return nil
}

func writeTrustDirectories(tx *sql.Tx, directories []trustDirectoryMtime) error {
	if _, err := tx.Exec("delete from trust_directories"); err != nil {
		return fmt.Errorf("clear trust directories: %w", err)
	}
	for _, directory := range directories {
		if _, err := tx.Exec("insert into trust_directories(path, modified_at) values (?, ?)", directory.Path, directory.ModifiedAt); err != nil {
			return fmt.Errorf("write trust directory %q: %w", directory.Path, err)
		}
	}
	return nil
}

func validateApprovedClaimsForRebuild(paths Paths, workspace string, claims []Claim) ([]Claim, []InvalidClaim, error) {
	validator, err := NewTrustValidatorFromStore(ClaimStore{Paths: paths}, workspace)
	if err != nil {
		return nil, nil, err
	}
	evidenceValidator, err := NewEvidenceValidator(EvidenceStore{Paths: paths}, workspace)
	if err != nil {
		return nil, nil, err
	}
	validClaims := make([]Claim, 0, len(claims))
	invalidClaims := make([]InvalidClaim, 0)
	for _, claim := range claims {
		if claim.Status != ClaimStatusApproved {
			validClaims = append(validClaims, claim)
			continue
		}
		if err := VerifyClaimDigest(claim); err != nil {
			invalidClaims = append(invalidClaims, InvalidClaim{Path: claim.Path, Error: err.Error()})
			continue
		}
		if err := validateClaimEvidence(evidenceValidator, claim); err != nil {
			invalidClaims = append(invalidClaims, InvalidClaim{Path: claim.Path, Error: err.Error()})
			continue
		}
		validator.validateSupporting = func(support Claim) error {
			return validateClaimEvidence(evidenceValidator, support)
		}
		if err := validator.ValidateClaim(claim); err != nil {
			invalidClaims = append(invalidClaims, InvalidClaim{Path: claim.Path, Error: err.Error()})
			continue
		}
		validClaims = append(validClaims, claim)
	}
	sort.Slice(invalidClaims, func(i, j int) bool {
		if invalidClaims[i].Path == invalidClaims[j].Path {
			return invalidClaims[i].Error < invalidClaims[j].Error
		}
		return invalidClaims[i].Path < invalidClaims[j].Path
	})
	return validClaims, invalidClaims, nil
}

func (store IndexStore) Rebuild(workspace string) (IndexSummary, error) {
	databasePath, dirtyPath, err := store.validatedIndexPaths(workspace)
	if err != nil {
		return IndexSummary{}, err
	}
	if err := RecoverPendingTransitionForMutation(store.Paths, workspace); err != nil {
		return IndexSummary{}, err
	}
	if err := store.AssertFTS5(); err != nil {
		return IndexSummary{}, err
	}
	if err := os.MkdirAll(store.Paths.IndexesDir, 0o755); err != nil {
		return IndexSummary{}, err
	}
	if err := store.MarkDirty(workspace); err != nil {
		return IndexSummary{}, err
	}
	tmpPath := databasePath + ".tmp"
	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		return IndexSummary{}, err
	}
	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return IndexSummary{}, err
	}
	defer db.Close()
	if err := createIndexSchema(db); err != nil {
		return IndexSummary{}, err
	}

	manifestBefore, err := BuildTrustInputManifest(store.Paths, workspace)
	if err != nil {
		return IndexSummary{}, err
	}
	scan, err := (ClaimStore{Paths: store.Paths}).ScanWorkspaceForTrust(workspace)
	if err != nil {
		return IndexSummary{}, err
	}
	manifest, err := BuildTrustInputManifest(store.Paths, workspace)
	if err != nil {
		return IndexSummary{}, err
	}
	if !sameTrustInputManifest(manifestBefore, manifest) {
		return IndexSummary{}, fmt.Errorf("trust inputs changed during rebuild")
	}
	validClaims, dependencyInvalid, err := validateApprovedClaimsForRebuild(store.Paths, workspace, scan.Claims)
	if err != nil {
		return IndexSummary{}, err
	}
	scan.Claims = validClaims
	scan.Invalid = append(scan.Invalid, dependencyInvalid...)
	sort.Slice(scan.Invalid, func(i, j int) bool {
		if scan.Invalid[i].Path == scan.Invalid[j].Path {
			return scan.Invalid[i].Error < scan.Invalid[j].Error
		}
		return scan.Invalid[i].Path < scan.Invalid[j].Path
	})
	inputMtimes, err := collectTrustInputMtimes(store.Paths, workspace, manifest)
	if err != nil {
		return IndexSummary{}, err
	}
	directories, err := collectTrustDirectories(store.Paths, workspace)
	if err != nil {
		return IndexSummary{}, err
	}

	invalidCount := len(scan.Invalid) + len(scan.LegacyUnindexed)
	rebuildStatus := RebuildStatusClean
	if invalidCount > 0 {
		rebuildStatus = RebuildStatusRejected
	}
	rebuiltAt := time.Now().UTC().Format(time.RFC3339)
	summary := IndexSummary{
		Workspace:      workspace,
		Invalid:        len(scan.Invalid),
		InvalidCount:   invalidCount,
		InvalidClaims:  scan.Invalid,
		Legacy:         len(scan.LegacyUnindexed),
		RebuildState:   rebuildStatus,
		ManifestDigest: manifest.Digest,
		RebuiltAt:      rebuiltAt,
	}
	tx, err := db.Begin()
	if err != nil {
		return IndexSummary{}, err
	}
	for _, claim := range scan.Claims {
		if claim.Status == ClaimStatusApproved {
			summary.Approved++
		} else if claim.Status == ClaimStatusDraft {
			summary.Draft++
		}
		if err := insertIndexedClaim(tx, claim); err != nil {
			_ = tx.Rollback()
			return IndexSummary{}, err
		}
	}
	state := RebuildState{
		Status:         rebuildStatus,
		InvalidCount:   invalidCount,
		ManifestDigest: manifest.Digest,
		RebuiltAt:      rebuiltAt,
	}
	if err := WriteIndexState(tx, manifest, state); err != nil {
		_ = tx.Rollback()
		return IndexSummary{}, err
	}
	if err := writeTrustInputMtimes(tx, inputMtimes); err != nil {
		_ = tx.Rollback()
		return IndexSummary{}, err
	}
	if err := writeTrustDirectories(tx, directories); err != nil {
		_ = tx.Rollback()
		return IndexSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return IndexSummary{}, err
	}
	if err := integrityCheck(db); err != nil {
		return IndexSummary{}, err
	}
	if err := db.Close(); err != nil {
		return IndexSummary{}, err
	}
	if err := os.Rename(tmpPath, databasePath); err != nil {
		return IndexSummary{}, err
	}
	publishedAt := time.Now()
	if err := os.Chtimes(databasePath, publishedAt, publishedAt); err != nil {
		return IndexSummary{}, fmt.Errorf("set index mtime: %w", err)
	}
	if err := os.Remove(dirtyPath); err != nil && !os.IsNotExist(err) {
		return IndexSummary{}, err
	}
	return summary, nil
}

func sameTrustInputManifest(left TrustInputManifest, right TrustInputManifest) bool {
	if left.Digest != right.Digest || len(left.Entries) != len(right.Entries) {
		return false
	}
	for i := range left.Entries {
		if left.Entries[i] != right.Entries[i] {
			return false
		}
	}
	return true
}

func (store IndexStore) Search(workspace string, options SearchOptions) ([]IndexedClaim, error) {
	databasePath, _, err := store.validatedIndexPaths(workspace)
	if err != nil {
		return nil, err
	}
	if options.Limit <= 0 {
		options.Limit = 10
	}
	if strings.TrimSpace(options.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if len(options.Statuses) == 0 {
		return nil, fmt.Errorf("at least one status filter is required")
	}
	if err := store.CheckFresh(workspace); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := integrityCheck(db); err != nil {
		return nil, err
	}

	statusValues := make([]string, 0, len(options.Statuses))
	for _, status := range options.Statuses {
		if !isKnownClaimStatus(status) {
			return nil, fmt.Errorf("claim status %q is not supported", status)
		}
		statusValues = append(statusValues, string(status))
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(statusValues)), ",")
	matchQuery := fts5Query(options.Query)
	if matchQuery == "" {
		return nil, fmt.Errorf("query is required")
	}
	args := []any{matchQuery}
	for _, status := range statusValues {
		args = append(args, status)
	}
	args = append(args, options.Limit)
	rows, err := db.Query(`
select c.id, c.path, c.tier, c.type, c.status, c.title, c.description, c.stale_after, rank
from claims_fts
join claims c on c.rowid = claims_fts.rowid
where claims_fts match ? and c.status in (`+placeholders+`)
order by rank, c.path
limit ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []IndexedClaim{}
	for rows.Next() {
		var result IndexedClaim
		if err := rows.Scan(&result.ID, &result.Path, &result.Tier, &result.Type, &result.Status, &result.Title, &result.Description, &result.StaleAfter, &result.Score); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func fts5Query(query string) string {
	tokens := queryTokens(query)
	quoted := make([]string, 0, len(tokens))
	for _, token := range tokens {
		quoted = append(quoted, `"`+strings.ReplaceAll(token, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " ")
}

func createIndexSchema(db *sql.DB) error {
	_, err := db.Exec(`
create table claims (
  id text not null unique,
  path text not null unique,
  tier text not null,
  type text not null,
  status text not null,
  title text not null,
  description text not null,
  stale_after text not null,
  tags text not null,
  body text not null
);
create virtual table claims_fts using fts5(
  title,
  description,
  tags,
  body,
  content='claims',
  content_rowid='rowid'
);
create trigger claims_ai after insert on claims begin
  insert into claims_fts(rowid, title, description, tags, body) values (new.rowid, new.title, new.description, new.tags, new.body);
end;
create trigger claims_ad after delete on claims begin
  insert into claims_fts(claims_fts, rowid, title, description, tags, body) values ('delete', old.rowid, old.title, old.description, old.tags, old.body);
end;
create trigger claims_au after update on claims begin
  insert into claims_fts(claims_fts, rowid, title, description, tags, body) values ('delete', old.rowid, old.title, old.description, old.tags, old.body);
  insert into claims_fts(rowid, title, description, tags, body) values (new.rowid, new.title, new.description, new.tags, new.body);
end;
create table trust_inputs (
  path text not null primary key,
  kind text not null,
  byte_length integer not null,
  sha256 text not null
);
create table trust_input_mtimes (
  path text not null primary key,
  modified_at integer not null
);
create table trust_directories (
  path text not null primary key,
  modified_at integer not null
);
create table rebuild_state (
  id integer not null primary key default 1 check (id = 1),
  status text not null,
  invalid_count integer not null,
  manifest_digest text not null,
  rebuilt_at text not null
);`)
	if err != nil {
		return err
	}
	_, err = db.Exec("pragma user_version = 1")
	return err
}

func insertIndexedClaim(tx *sql.Tx, claim Claim) error {
	_, err := tx.Exec(`insert into claims(id, path, tier, type, status, title, description, stale_after, tags, body) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		claim.ID,
		claim.Path,
		claim.Tier,
		OKFClaimType,
		string(claim.Status),
		claim.Title,
		claim.Description,
		claim.StaleAfter,
		strings.Join(claim.Tags, " "),
		claim.Body,
	)
	return err
}

func integrityCheck(db *sql.DB) error {
	var result string
	if err := db.QueryRow("pragma integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("sqlite integrity_check = %s", result)
	}
	if _, err := db.Exec("insert into claims_fts(claims_fts, rank) values ('integrity-check', 1)"); err != nil {
		return err
	}
	return nil
}
