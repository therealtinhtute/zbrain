package runtime

import (
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
	source, err := os.Open(sourcePath)
	if err != nil {
		return Evidence{}, err
	}
	defer source.Close()

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
	if err := os.MkdirAll(root, 0o755); err != nil {
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
	raw, err := os.OpenFile(rawPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o444)
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
	if err := os.Chmod(rawPath, 0o444); err != nil {
		return Evidence{}, err
	}

	now := time.Now
	if store.Now != nil {
		now = store.Now
	}
	evidence := Evidence{
		ID:         id,
		Origin:     origin,
		CapturedAt: now().UTC().Format(time.RFC3339),
		MediaType:  mediaType,
		ByteLength: written,
		SHA256:     hex.EncodeToString(hash.Sum(nil)),
	}
	metadata, err := yaml.Marshal(evidence)
	if err != nil {
		return Evidence{}, err
	}
	metadataPath, err := store.evidenceFilePath(workspace, id, "source.yaml")
	if err != nil {
		return Evidence{}, err
	}
	if err := os.WriteFile(metadataPath, metadata, 0o444); err != nil {
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
	fmt.Fprintf(hash, "metadata-length:%d\n", len(metadata))
	hash.Write(metadata)
	fmt.Fprintf(hash, "\nraw-byte-length:%d\nraw-sha256:%s\n", evidence.ByteLength, evidence.SHA256)
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
			continue
		}
		if isLegacyEvidenceDigest(source.Digest) {
			return fmt.Errorf("evidence %s uses legacy raw digest; supersede and reapprove claim %s to bind metadata and raw bytes", id, claim.ID)
		}
		return fmt.Errorf("evidence %s digest mismatch: approved %s, current %s", id, source.Digest, currentDigest)
	}
	return nil
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

func (store EvidenceStore) markDirty(workspace string) error {
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return err
	}
	return (IndexStore{Paths: store.Paths}).MarkDirty(workspace)
}
