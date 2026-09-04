package runtime

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

type EvidenceStore struct {
	Paths Paths
	Now   func() time.Time
}

type Evidence struct {
	ID         string `yaml:"id" json:"id"`
	Origin     string `yaml:"origin" json:"origin"`
	CapturedAt string `yaml:"captured_at" json:"captured_at"`
	MediaType  string `yaml:"media_type" json:"media_type"`
	ByteLength int64  `yaml:"byte_length" json:"byte_length"`
	SHA256     string `yaml:"sha256" json:"sha256"`
	Deduped    bool   `yaml:"-" json:"deduped"`
}

type EvidenceVerificationError struct {
	ID     string
	Path   string
	Reason string
	Err    error
}

func (err *EvidenceVerificationError) Error() string {
	return fmt.Sprintf("evidence verification failed for %s at %s: %s", err.ID, err.Path, err.Reason)
}

func (err *EvidenceVerificationError) Unwrap() error {
	return err.Err
}

type EvidenceValidator struct {
	store           EvidenceStore
	workspace       string
	cache           map[string]error
	snapshotDigests map[string]string
	verifyCount     map[string]int
}

func NewEvidenceValidator(store EvidenceStore, workspace string) (*EvidenceValidator, error) {
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return nil, err
	}
	return &EvidenceValidator{
		store:           store,
		workspace:       workspace,
		cache:           make(map[string]error),
		snapshotDigests: make(map[string]string),
		verifyCount:     make(map[string]int),
	}, nil
}

func NewEvidenceID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "evd_" + hex.EncodeToString(buf), nil
}

func (store EvidenceStore) AddFile(workspace string, sourcePath string, origin string, mediaType string) (Evidence, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return Evidence{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := recoverPendingTransitionForMutationUnlocked(store.Paths, workspace); err != nil {
		return Evidence{}, err
	}
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return Evidence{}, err
	}
	if strings.TrimSpace(origin) == "" {
		return Evidence{}, fmt.Errorf("evidence origin is required")
	}
	if strings.TrimSpace(mediaType) == "" {
		mediaType = "application/octet-stream"
	}
	digest, err := hashEvidenceSourceFile(sourcePath)
	if err != nil {
		return Evidence{}, err
	}
	if existing, found, err := store.lookupEvidenceBySHA256(workspace, digest); err != nil {
		return Evidence{}, err
	} else if found {
		existing.Deduped = true
		return existing, nil
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return Evidence{}, err
	}
	defer func() { _ = source.Close() }()

	id, err := NewEvidenceID()
	if err != nil {
		return Evidence{}, err
	}
	root, err := store.evidenceRoot(workspace, id)
	if err != nil {
		return Evidence{}, err
	}
	if _, err := os.Stat(root); err == nil {
		return Evidence{}, fmt.Errorf("evidence %s already exists", id)
	} else if !os.IsNotExist(err) {
		return Evidence{}, err
	}
	if _, err := beginCanonicalMutationUnlocked(store.Paths, workspace); err != nil {
		return Evidence{}, err
	}
	for _, relative := range []string{"evidence", "evidence/sources"} {
		directory, err := ResolveWorkspacePath(store.Paths, workspace, relative)
		if err != nil {
			return Evidence{}, err
		}
		if err := ensureDirectoryMode(directory, evidenceDirectoryMode); err != nil {
			return Evidence{}, err
		}
	}
	if err := ensureDirectoryMode(root, evidenceDirectoryMode); err != nil {
		return Evidence{}, err
	}
	created := false
	defer func() {
		if !created {
			_ = os.RemoveAll(root)
		}
	}()

	rawPath, err := store.evidenceFilePath(workspace, id, "raw")
	if err != nil {
		return Evidence{}, err
	}
	runWorkspaceGenerationTestHook(workspaceGenerationHookBeforeCanonicalWrite)
	raw, err := os.OpenFile(rawPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, evidenceFileMode)
	if err != nil {
		return Evidence{}, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(raw, hash), source)
	closeErr := raw.Close()
	if copyErr != nil {
		return Evidence{}, copyErr
	}
	if closeErr != nil {
		return Evidence{}, closeErr
	}
	if err := ensureFileMode(rawPath, evidenceFileMode); err != nil {
		return Evidence{}, err
	}

	now := time.Now
	if store.Now != nil {
		now = store.Now
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != digest {
		return Evidence{}, fmt.Errorf("source file changed during capture")
	}
	evidence := Evidence{
		ID:         id,
		Origin:     origin,
		CapturedAt: now().UTC().Format(time.RFC3339),
		MediaType:  mediaType,
		ByteLength: written,
		SHA256:     digest,
	}
	metadata, err := yaml.Marshal(evidence)
	if err != nil {
		return Evidence{}, err
	}
	metadataPath, err := store.evidenceFilePath(workspace, id, "source.yaml")
	if err != nil {
		return Evidence{}, err
	}
	if err := os.WriteFile(metadataPath, metadata, evidenceFileMode); err != nil {
		return Evidence{}, err
	}
	if err := ensureFileMode(metadataPath, evidenceFileMode); err != nil {
		return Evidence{}, err
	}
	created = true
	return evidence, nil
}

func (store EvidenceStore) Read(workspace string, id string) (Evidence, error) {
	if !evidenceIDPattern.MatchString(id) {
		return Evidence{}, fmt.Errorf("evidence id must match evd_<32 lowercase hex chars>")
	}
	metadataPath, err := store.evidenceFilePath(workspace, id, "source.yaml")
	if err != nil {
		return Evidence{}, err
	}
	contents, err := os.ReadFile(metadataPath)
	if err != nil {
		return Evidence{}, err
	}
	var evidence Evidence
	if err := yaml.Unmarshal(contents, &evidence); err != nil {
		return Evidence{}, err
	}
	if evidence.ID != id {
		return Evidence{}, fmt.Errorf("evidence metadata id %q does not match path id %q", evidence.ID, id)
	}
	return evidence, nil
}

func (store EvidenceStore) Verify(workspace string, id string) error {
	validator, err := NewEvidenceValidator(store, workspace)
	if err != nil {
		return err
	}
	return validator.Verify(id)
}

// ReadRaw returns the raw captured bytes of an evidence snapshot.
func (store EvidenceStore) ReadRaw(workspace string, id string) ([]byte, error) {
	if !evidenceIDPattern.MatchString(id) {
		return nil, fmt.Errorf("evidence id must match evd_<32 lowercase hex chars>")
	}
	rawPath, err := store.evidenceFilePath(workspace, id, "raw")
	if err != nil {
		return nil, err
	}
	return os.ReadFile(rawPath)
}

func (validator *EvidenceValidator) Verify(id string) error {
	if result, cached := validator.cache[id]; cached {
		return result
	}
	validator.verifyCount[id]++
	err := validator.verifyUncached(id)
	validator.cache[id] = err
	return err
}

func (validator *EvidenceValidator) verifyUncached(id string) error {
	if !evidenceIDPattern.MatchString(id) {
		return newEvidenceVerificationError(id, evidenceMetadataPath(id), "evidence id is unsafe", nil)
	}
	metadataPath, err := validator.store.evidenceFilePath(validator.workspace, id, "source.yaml")
	if err != nil {
		return newEvidenceVerificationError(id, evidenceMetadataPath(id), "resolve metadata path: "+err.Error(), err)
	}
	contents, err := os.ReadFile(metadataPath)
	if err != nil {
		return newEvidenceVerificationError(id, evidenceMetadataPath(id), "read metadata: "+err.Error(), err)
	}
	var evidence Evidence
	if err := yaml.Unmarshal(contents, &evidence); err != nil {
		return newEvidenceVerificationError(id, evidenceMetadataPath(id), "parse metadata: "+err.Error(), err)
	}
	if err := validateEvidenceMetadata(id, evidence); err != nil {
		return newEvidenceVerificationError(id, evidenceMetadataPath(id), err.Error(), err)
	}

	rawPath, err := validator.store.evidenceFilePath(validator.workspace, id, "raw")
	if err != nil {
		return newEvidenceVerificationError(id, evidenceRawPath(id), "resolve raw path: "+err.Error(), err)
	}
	raw, err := os.Open(rawPath)
	if err != nil {
		return newEvidenceVerificationError(id, evidenceRawPath(id), "read raw bytes: "+err.Error(), err)
	}
	hash := sha256.New()
	length, copyErr := io.Copy(hash, raw)
	closeErr := raw.Close()
	if copyErr != nil {
		return newEvidenceVerificationError(id, evidenceRawPath(id), "read raw bytes: "+copyErr.Error(), copyErr)
	}
	if closeErr != nil {
		return newEvidenceVerificationError(id, evidenceRawPath(id), "close raw bytes: "+closeErr.Error(), closeErr)
	}
	if length != evidence.ByteLength {
		err := fmt.Errorf("byte length = %d, want %d", length, evidence.ByteLength)
		return newEvidenceVerificationError(id, evidenceRawPath(id), err.Error(), err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != evidence.SHA256 {
		err := fmt.Errorf("sha256 = %s, want %s", actual, evidence.SHA256)
		return newEvidenceVerificationError(id, evidenceRawPath(id), err.Error(), err)
	}
	validator.snapshotDigests[id] = evidenceSnapshotDigest(contents, evidence)
	return nil
}

const evidenceSnapshotDigestPrefix = "sha256:evidence-v1:"

func evidenceSnapshotDigest(metadata []byte, evidence Evidence) string {
	hash := sha256.New()
	hash.Write([]byte("zbrain.evidence/v1\n"))
	_, _ = fmt.Fprintf(hash, "metadata-length:%d\n", len(metadata))
	hash.Write(metadata)
	_, _ = fmt.Fprintf(hash, "\nraw-byte-length:%d\nraw-sha256:%s\n", evidence.ByteLength, evidence.SHA256)
	return evidenceSnapshotDigestPrefix + hex.EncodeToString(hash.Sum(nil))
}

func isLegacyEvidenceDigest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && isEvidenceSHA256(strings.TrimPrefix(value, "sha256:"))
}

func validateEvidenceMetadata(id string, evidence Evidence) error {
	if !evidenceIDPattern.MatchString(evidence.ID) {
		return fmt.Errorf("metadata id %q is unsafe", evidence.ID)
	}
	if evidence.ID != id {
		return fmt.Errorf("metadata id %q does not match requested id %q", evidence.ID, id)
	}
	if strings.TrimSpace(evidence.Origin) == "" {
		return fmt.Errorf("metadata origin is required")
	}
	if _, err := time.Parse(time.RFC3339, evidence.CapturedAt); err != nil {
		return fmt.Errorf("metadata captured_at must be RFC3339: %w", err)
	}
	if _, _, err := mime.ParseMediaType(evidence.MediaType); err != nil {
		return fmt.Errorf("metadata media_type is invalid: %w", err)
	}
	if evidence.ByteLength < 0 {
		return fmt.Errorf("metadata byte_length must be non-negative")
	}
	if !isEvidenceSHA256(evidence.SHA256) {
		return fmt.Errorf("metadata sha256 must be 64 lowercase hexadecimal characters")
	}
	return nil
}

func validateClaimEvidence(validator *EvidenceValidator, claim Claim) error {
	ids := append([]string(nil), claim.EvidenceIDs...)
	sort.Strings(ids)
	seenIDs := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seenIDs[id]; exists {
			return fmt.Errorf("claim %s has duplicate evidence id %s", claim.ID, id)
		}
		seenIDs[id] = struct{}{}
		if err := validator.Verify(id); err != nil {
			return fmt.Errorf("evidence %s: %w", id, err)
		}
	}
	if claim.Status != ClaimStatusApproved {
		return nil
	}

	approvedSources := make(map[string]ClaimSource, len(claim.Sources))
	for _, source := range claim.Sources {
		if _, exists := approvedSources[source.ID]; exists {
			return fmt.Errorf("claim %s has duplicate evidence source %s", claim.ID, source.ID)
		}
		if _, exists := seenIDs[source.ID]; !exists {
			return fmt.Errorf("claim %s evidence source closure does not match approved evidence ids", claim.ID)
		}
		approvedSources[source.ID] = source
	}
	if len(approvedSources) != len(seenIDs) {
		return fmt.Errorf("claim %s evidence source closure does not match approved evidence ids", claim.ID)
	}
	for _, id := range ids {
		evidence, err := validator.store.Read(validator.workspace, id)
		if err != nil {
			return fmt.Errorf("evidence %s: read current metadata: %w", id, err)
		}
		source, ok := approvedSources[id]
		if !ok {
			return fmt.Errorf("claim %s evidence source closure does not match approved evidence ids", claim.ID)
		}
		expectedResource := filepath.ToSlash(filepath.Join("evidence", "sources", id, "raw"))
		if source.Resource != expectedResource || source.Title != evidence.Origin {
			return fmt.Errorf("evidence %s source reference does not match current metadata", id)
		}
		currentDigest := validator.snapshotDigests[id]
		if source.Digest == currentDigest {
			if err := validateEvidenceSpans(validator, id, source, currentDigest); err != nil {
				return err
			}
			continue
		}
		if isLegacyEvidenceDigest(source.Digest) {
			return fmt.Errorf("evidence %s uses legacy raw digest; supersede and reapprove claim %s to bind metadata and raw bytes", id, claim.ID)
		}
		return fmt.Errorf("evidence %s digest mismatch: approved %s, current %s", id, source.Digest, currentDigest)
	}
	return nil
}

func validateEvidenceSpans(validator *EvidenceValidator, evidenceID string, source ClaimSource, snapshotDigest string) error {
	if len(source.Spans) == 0 {
		return nil
	}
	rawPath, err := validator.store.evidenceFilePath(validator.workspace, evidenceID, "raw")
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		return fmt.Errorf("evidence %s: read span bytes: %w", evidenceID, err)
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("evidence %s spans require valid UTF-8 raw evidence", evidenceID)
	}
	lines := splitEvidenceLines(raw)
	for _, span := range source.Spans {
		if span.EvidenceID != evidenceID {
			return fmt.Errorf("evidence span id %s does not match source %s", span.EvidenceID, evidenceID)
		}
		if span.StartLine < 1 || span.EndLine < span.StartLine || span.EndLine > len(lines) {
			return fmt.Errorf("evidence %s span line range %d-%d is out of range", evidenceID, span.StartLine, span.EndLine)
		}
		want := EvidenceSpanDigest(snapshotDigest, span.StartLine, span.EndLine, bytes.Join(lines[span.StartLine-1:span.EndLine], nil))
		if span.Digest != want {
			return fmt.Errorf("evidence %s span digest mismatch", evidenceID)
		}
	}
	return nil
}

func splitEvidenceLines(raw []byte) [][]byte {
	if len(raw) == 0 {
		return [][]byte{{}}
	}
	lines := make([][]byte, 0, 1)
	start := 0
	for i, b := range raw {
		if b == '\n' {
			lines = append(lines, raw[start:i+1])
			start = i + 1
		}
	}
	if start < len(raw) {
		lines = append(lines, raw[start:])
	}
	return lines
}

func EvidenceSpanDigest(snapshotDigest string, startLine, endLine int, rawBytes []byte) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "zbrain.span/v1\nsnapshot:%s\nrange:%d-%d\n", snapshotDigest, startLine, endLine)
	hash.Write(rawBytes)
	return "sha256:span-v1:" + hex.EncodeToString(hash.Sum(nil))
}

func isEvidenceSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func newEvidenceVerificationError(id string, path string, reason string, cause error) error {
	return &EvidenceVerificationError{ID: id, Path: path, Reason: reason, Err: cause}
}

func evidenceMetadataPath(id string) string {
	return filepath.ToSlash(filepath.Join("evidence", "sources", id, "source.yaml"))
}

func evidenceRawPath(id string) string {
	return filepath.ToSlash(filepath.Join("evidence", "sources", id, "raw"))
}

func (store EvidenceStore) evidenceRoot(workspace string, id string) (string, error) {
	if !evidenceIDPattern.MatchString(id) {
		return "", fmt.Errorf("evidence id must match evd_<32 lowercase hex chars>")
	}
	return ResolveWorkspacePath(store.Paths, workspace, filepath.ToSlash(filepath.Join("evidence", "sources", id)))
}

func (store EvidenceStore) evidenceFilePath(workspace string, id string, name string) (string, error) {
	if !evidenceIDPattern.MatchString(id) {
		return "", fmt.Errorf("evidence id must match evd_<32 lowercase hex chars>")
	}
	return ResolveWorkspacePath(store.Paths, workspace, filepath.ToSlash(filepath.Join("evidence", "sources", id, name)))
}

type EvidenceDriftStatus string

const (
	EvidenceDriftUnchanged   EvidenceDriftStatus = "unchanged"
	EvidenceDriftChanged     EvidenceDriftStatus = "changed"
	EvidenceDriftMissing     EvidenceDriftStatus = "missing"
	EvidenceDriftUncheckable EvidenceDriftStatus = "uncheckable"
)

const evidenceDriftRecoveryAction = "supersede and re-approve affected claims against a fresh evidence snapshot"

type EvidenceDriftFinding struct {
	ID                   string              `json:"id"`
	Origin               string              `json:"origin"`
	Status               EvidenceDriftStatus `json:"status"`
	RecordedSHA256       string              `json:"recorded_sha256,omitempty"`
	RecomputedSHA256     string              `json:"recomputed_sha256,omitempty"`
	RecordedByteLength   int64               `json:"recorded_byte_length,omitempty"`
	RecomputedByteLength int64               `json:"recomputed_byte_length,omitempty"`
	AffectedClaimIDs     []string            `json:"affected_claim_ids"`
	RecoveryAction       string              `json:"recovery_action"`
}

type EvidenceDriftReport struct {
	Workspace string                 `json:"workspace"`
	Findings  []EvidenceDriftFinding `json:"findings"`
}

// CheckDrift re-hashes every evidence snapshot origin against its recorded
// metadata. It is strictly read-only: snapshots, canonical Markdown, indexes,
// and mtimes are never touched, and non-local origins are never fetched.
func (store EvidenceStore) CheckDrift(workspace string) (EvidenceDriftReport, error) {
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return EvidenceDriftReport{}, err
	}
	affected := make(map[string][]string)
	scan, err := (ClaimStore{Paths: store.Paths}).ScanWorkspace(workspace)
	if err != nil {
		return EvidenceDriftReport{}, err
	}
	for _, claim := range scan.Claims {
		for _, id := range claim.EvidenceIDs {
			affected[id] = append(affected[id], claim.ID)
		}
	}
	sourcesRoot, err := ResolveWorkspacePath(store.Paths, workspace, filepath.ToSlash(filepath.Join("evidence", "sources")))
	if err != nil {
		return EvidenceDriftReport{}, err
	}
	entries, err := os.ReadDir(sourcesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return EvidenceDriftReport{Workspace: workspace, Findings: []EvidenceDriftFinding{}}, nil
		}
		return EvidenceDriftReport{}, err
	}
	report := EvidenceDriftReport{Workspace: workspace, Findings: make([]EvidenceDriftFinding, 0, len(entries))}
	for _, entry := range entries {
		if !entry.IsDir() || !evidenceIDPattern.MatchString(entry.Name()) {
			continue
		}
		evidence, err := store.Read(workspace, entry.Name())
		if err != nil {
			continue
		}
		report.Findings = append(report.Findings, classifyEvidenceDrift(evidence, affected[evidence.ID]))
	}
	sort.Slice(report.Findings, func(i, j int) bool { return report.Findings[i].ID < report.Findings[j].ID })
	return report, nil
}

func classifyEvidenceDrift(evidence Evidence, affectedClaimIDs []string) EvidenceDriftFinding {
	affected := make([]string, 0, len(affectedClaimIDs))
	affected = append(affected, affectedClaimIDs...)
	sort.Strings(affected)
	finding := EvidenceDriftFinding{
		ID:                 evidence.ID,
		Origin:             evidence.Origin,
		Status:             EvidenceDriftUnchanged,
		RecordedSHA256:     evidence.SHA256,
		RecordedByteLength: evidence.ByteLength,
		AffectedClaimIDs:   affected,
		RecoveryAction:     "no action required",
	}
	localPath, checkable := evidenceOriginLocalPath(evidence.Origin)
	if !checkable {
		finding.Status = EvidenceDriftUncheckable
		finding.RecoveryAction = "origin is not locally checkable; supersede and re-approve against a fresh snapshot if drift is suspected"
		return finding
	}
	recomputed, length, err := hashLocalOrigin(localPath)
	if err != nil {
		finding.Status = EvidenceDriftMissing
		finding.RecoveryAction = evidenceDriftRecoveryAction
		if !os.IsNotExist(err) {
			finding.Status = EvidenceDriftUncheckable
			finding.RecoveryAction = "origin cannot be read locally; supersede and re-approve against a fresh snapshot if drift is suspected"
		}
		return finding
	}
	finding.RecomputedSHA256 = recomputed
	finding.RecomputedByteLength = length
	if recomputed != evidence.SHA256 || length != evidence.ByteLength {
		finding.Status = EvidenceDriftChanged
		finding.RecoveryAction = evidenceDriftRecoveryAction
	}
	return finding
}

// evidenceOriginLocalPath resolves an origin recorded at evidence add time to a
// local filesystem path. Non-file URI schemes are reported as uncheckable.
func evidenceOriginLocalPath(origin string) (string, bool) {
	if index := strings.Index(origin, "://"); index > 0 && isURIScheme(origin[:index]) {
		if !strings.EqualFold(origin[:index], "file") {
			return "", false
		}
		return strings.TrimPrefix(origin[index+3:], "//"), true
	}
	return origin, true
}

func isURIScheme(value string) bool {
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'):
		default:
			return false
		}
	}
	return true
}

func hashLocalOrigin(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	written, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func (store EvidenceStore) markDirty(workspace string) error {
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return err
	}
	return (IndexStore{Paths: store.Paths}).MarkDirty(workspace)
}

func hashEvidenceSourceFile(path string) (string, error) {
	source, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = source.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, source); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (store EvidenceStore) lookupEvidenceBySHA256(workspace string, digest string) (Evidence, bool, error) {
	sourcesDir, err := ResolveWorkspacePath(store.Paths, workspace, filepath.ToSlash(filepath.Join("evidence", "sources")))
	if err != nil {
		return Evidence{}, false, err
	}
	entries, err := os.ReadDir(sourcesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return Evidence{}, false, nil
		}
		return Evidence{}, false, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if evidenceIDPattern.MatchString(name) {
			ids = append(ids, name)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		evidence, err := store.Read(workspace, id)
		if err != nil {
			continue
		}
		if evidence.SHA256 == digest {
			return evidence, true, nil
		}
	}
	return Evidence{}, false, nil
}
