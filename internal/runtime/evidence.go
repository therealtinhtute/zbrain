package runtime

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func NewEvidenceID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "evd_" + hex.EncodeToString(buf), nil
}

func (store EvidenceStore) AddFile(workspace string, sourcePath string, origin string, mediaType string) (Evidence, error) {
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
	if err := store.markDirty(workspace); err != nil {
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
	evidence, err := store.Read(workspace, id)
	if err != nil {
		return err
	}
	rawPath, err := store.evidenceFilePath(workspace, id, "raw")
	if err != nil {
		return err
	}
	raw, err := os.Open(rawPath)
	if err != nil {
		return err
	}
	defer raw.Close()
	hash := sha256.New()
	length, err := io.Copy(hash, raw)
	if err != nil {
		return err
	}
	if length != evidence.ByteLength {
		return fmt.Errorf("evidence %s byte length = %d, want %d", id, length, evidence.ByteLength)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != evidence.SHA256 {
		return fmt.Errorf("evidence %s sha256 = %s, want %s", id, actual, evidence.SHA256)
	}
	return nil
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
