package runtime

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	pathpkg "path"
	"strings"
	"time"
)

const (
	indexSchemaVersion  = 3
	indexStateSingleton = 1
)

type RebuildStatus string

const (
	RebuildStatusClean    RebuildStatus = "clean"
	RebuildStatusRejected RebuildStatus = "rejected"
)

type RebuildState struct {
	Status         RebuildStatus `json:"status"`
	InvalidCount   int           `json:"invalid_count"`
	ManifestDigest string        `json:"manifest_digest"`
	RebuiltAt      string        `json:"rebuilt_at"`
}

type indexStateSQL interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// WriteIndexState replaces the manifest and rebuild outcome in the caller's transaction.
// The transaction must be committed by the caller to publish both together.
func WriteIndexState(tx *sql.Tx, manifest TrustInputManifest, state RebuildState) error {
	if tx == nil {
		return fmt.Errorf("index state transaction is nil")
	}
	return writeIndexState(tx, manifest, state)
}

func writeIndexState(tx indexStateSQL, manifest TrustInputManifest, state RebuildState) error {
	if err := validateIndexStateSchema(tx); err != nil {
		return err
	}
	if err := validateTrustInputManifest(manifest); err != nil {
		return fmt.Errorf("validate trust input manifest: %w", err)
	}
	if err := validateRebuildState(state, manifest.Digest); err != nil {
		return fmt.Errorf("validate rebuild state: %w", err)
	}

	if _, err := tx.Exec("delete from trust_inputs"); err != nil {
		return fmt.Errorf("clear trust inputs: %w", err)
	}
	for _, entry := range manifest.Entries {
		if _, err := tx.Exec(`insert into trust_inputs(path, kind, byte_length, sha256) values (?, ?, ?, ?)`,
			entry.Path,
			entry.Kind,
			entry.ByteLength,
			entry.SHA256,
		); err != nil {
			return fmt.Errorf("write trust input %q: %w", entry.Path, err)
		}
	}
	if _, err := tx.Exec("delete from rebuild_state"); err != nil {
		return fmt.Errorf("clear rebuild state: %w", err)
	}
	if _, err := tx.Exec(`insert into rebuild_state(id, status, invalid_count, manifest_digest, rebuilt_at) values (?, ?, ?, ?, ?)`,
		indexStateSingleton,
		string(state.Status),
		state.InvalidCount,
		state.ManifestDigest,
		state.RebuiltAt,
	); err != nil {
		return fmt.Errorf("write rebuild state: %w", err)
	}
	return nil
}

// ReadIndexState reads and validates one published index generation.
func ReadIndexState(db *sql.DB) (TrustInputManifest, RebuildState, error) {
	if db == nil {
		return TrustInputManifest{}, RebuildState{}, fmt.Errorf("index state database is nil")
	}
	return readIndexState(db)
}

// ReadIndexStateTx reads and validates state visible inside a transaction.
func ReadIndexStateTx(tx *sql.Tx) (TrustInputManifest, RebuildState, error) {
	if tx == nil {
		return TrustInputManifest{}, RebuildState{}, fmt.Errorf("index state transaction is nil")
	}
	return readIndexState(tx)
}

func readIndexState(reader indexStateSQL) (TrustInputManifest, RebuildState, error) {
	if err := validateIndexStateSchema(reader); err != nil {
		return TrustInputManifest{}, RebuildState{}, err
	}

	manifest, err := readTrustInputManifest(reader)
	if err != nil {
		return TrustInputManifest{}, RebuildState{}, err
	}
	state, err := readRebuildState(reader)
	if err != nil {
		return TrustInputManifest{}, RebuildState{}, err
	}
	if err := validateRebuildState(state, manifest.Digest); err != nil {
		return TrustInputManifest{}, RebuildState{}, fmt.Errorf("validate rebuild state: %w", err)
	}
	return manifest, state, nil
}

func validateIndexStateSchema(reader indexStateSQL) error {
	var version int
	if err := reader.QueryRow("pragma user_version").Scan(&version); err != nil {
		return fmt.Errorf("read index schema version: %w", err)
	}
	if version != indexSchemaVersion {
		return fmt.Errorf("unsupported index schema version %d; want %d", version, indexSchemaVersion)
	}
	if err := requireIndexStateColumns(reader, "trust_inputs", map[string]string{
		"path":        "text",
		"kind":        "text",
		"byte_length": "integer",
		"sha256":      "text",
	}); err != nil {
		return fmt.Errorf("validate trust_inputs schema: %w", err)
	}
	if err := requireIndexStateColumns(reader, "trust_input_mtimes", map[string]string{
		"path":         "text",
		"modified_at":  "integer",
		"change_token": "integer",
	}); err != nil {
		return fmt.Errorf("validate trust input freshness schema: %w", err)
	}
	if err := requireIndexStateColumns(reader, "trust_directories", map[string]string{
		"path":         "text",
		"modified_at":  "integer",
		"change_token": "integer",
	}); err != nil {
		return fmt.Errorf("validate trust directory freshness schema: %w", err)
	}
	if err := requireIndexStateColumns(reader, "rebuild_state", map[string]string{
		"id":              "integer",
		"status":          "text",
		"invalid_count":   "integer",
		"manifest_digest": "text",
		"rebuilt_at":      "text",
	}); err != nil {
		return fmt.Errorf("validate rebuild_state schema: %w", err)
	}
	return nil
}

func requireIndexStateColumns(reader indexStateSQL, table string, expected map[string]string) error {
	rows, err := reader.Query("pragma table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	found := make(map[string]bool, len(expected))
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		wantType, ok := expected[name]
		if !ok {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(columnType), wantType) {
			return fmt.Errorf("column %q has type %q; want %q", name, columnType, wantType)
		}
		if notNull != 1 {
			return fmt.Errorf("column %q must be not null", name)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for name := range expected {
		if !found[name] {
			return fmt.Errorf("missing column %q", name)
		}
	}
	return nil
}

func readTrustInputManifest(reader indexStateSQL) (TrustInputManifest, error) {
	rows, err := reader.Query(`select path, kind, byte_length, sha256 from trust_inputs order by path`)
	if err != nil {
		return TrustInputManifest{}, fmt.Errorf("read trust inputs: %w", err)
	}
	defer rows.Close()

	entries := make([]TrustInput, 0)
	previousPath := ""
	for rows.Next() {
		var entry TrustInput
		if err := rows.Scan(&entry.Path, &entry.Kind, &entry.ByteLength, &entry.SHA256); err != nil {
			return TrustInputManifest{}, fmt.Errorf("scan trust input: %w", err)
		}
		if previousPath != "" && entry.Path <= previousPath {
			return TrustInputManifest{}, fmt.Errorf("trust inputs are not unique and sorted at %q", entry.Path)
		}
		if err := validateTrustInput(entry); err != nil {
			return TrustInputManifest{}, fmt.Errorf("validate trust input %q: %w", entry.Path, err)
		}
		entries = append(entries, entry)
		previousPath = entry.Path
	}
	if err := rows.Err(); err != nil {
		return TrustInputManifest{}, fmt.Errorf("read trust inputs: %w", err)
	}
	manifest := TrustInputManifest{Entries: entries, Digest: trustInputManifestDigest(entries)}
	if err := validateTrustInputManifest(manifest); err != nil {
		return TrustInputManifest{}, fmt.Errorf("validate trust input manifest: %w", err)
	}
	return manifest, nil
}

func readRebuildState(reader indexStateSQL) (RebuildState, error) {
	rows, err := reader.Query(`select id, status, invalid_count, manifest_digest, rebuilt_at from rebuild_state`)
	if err != nil {
		return RebuildState{}, fmt.Errorf("read rebuild state: %w", err)
	}
	defer rows.Close()

	var state RebuildState
	count := 0
	for rows.Next() {
		count++
		if count > 1 {
			return RebuildState{}, fmt.Errorf("rebuild_state must contain exactly one row; found more than one")
		}
		var id int
		var status string
		if err := rows.Scan(&id, &status, &state.InvalidCount, &state.ManifestDigest, &state.RebuiltAt); err != nil {
			return RebuildState{}, fmt.Errorf("scan rebuild state: %w", err)
		}
		if id != indexStateSingleton {
			return RebuildState{}, fmt.Errorf("rebuild_state singleton id = %d; want %d", id, indexStateSingleton)
		}
		state.Status = RebuildStatus(status)
	}
	if err := rows.Err(); err != nil {
		return RebuildState{}, fmt.Errorf("read rebuild state: %w", err)
	}
	if count != 1 {
		return RebuildState{}, fmt.Errorf("rebuild_state must contain exactly one row; found %d", count)
	}
	return state, nil
}

func validateTrustInputManifest(manifest TrustInputManifest) error {
	previousPath := ""
	for i, entry := range manifest.Entries {
		if err := validateTrustInput(entry); err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
		if previousPath != "" && entry.Path <= previousPath {
			return fmt.Errorf("entries must be strictly sorted by unique path")
		}
		previousPath = entry.Path
	}
	expectedDigest := trustInputManifestDigest(manifest.Entries)
	if !isSHA256Digest(manifest.Digest) {
		return fmt.Errorf("manifest digest must be a lowercase SHA-256 digest")
	}
	if manifest.Digest != expectedDigest {
		return fmt.Errorf("manifest digest does not match entries")
	}
	return nil
}

func validateTrustInput(entry TrustInput) error {
	if entry.Path == "" || strings.TrimSpace(entry.Path) != entry.Path {
		return fmt.Errorf("path must be non-empty and trimmed")
	}
	if strings.ContainsAny(entry.Path, "\\\x00") || pathpkg.IsAbs(entry.Path) {
		return fmt.Errorf("path must be a slash-normalized relative path")
	}
	if pathpkg.Clean(entry.Path) != entry.Path || entry.Path == "." || strings.HasPrefix(entry.Path, "../") {
		return fmt.Errorf("path must be canonical and remain relative")
	}
	if entry.ByteLength < 0 {
		return fmt.Errorf("byte length must not be negative")
	}
	if !isSHA256Digest(entry.SHA256) {
		return fmt.Errorf("sha256 must be a lowercase SHA-256 digest")
	}
	switch entry.Kind {
	case TrustInputKindClaim:
		parts := strings.Split(entry.Path, "/")
		if len(parts) < 3 || parts[0] != "wiki" || !isKnownWikiTier(parts[1]) || pathpkg.Ext(entry.Path) != ".md" {
			return fmt.Errorf("claim path must be wiki/<tier>/**/*.md")
		}
	case TrustInputKindEvidenceMetadata, TrustInputKindEvidenceRaw:
		parts := strings.Split(entry.Path, "/")
		if len(parts) != 4 || parts[0] != "evidence" || parts[1] != "sources" || parts[2] == "" {
			return fmt.Errorf("evidence path must be evidence/sources/<id>/<file>")
		}
		wantName := "source.yaml"
		if entry.Kind == TrustInputKindEvidenceRaw {
			wantName = "raw"
		}
		if parts[3] != wantName {
			return fmt.Errorf("evidence %s path must end in %q", entry.Kind, wantName)
		}
	default:
		return fmt.Errorf("kind %q is not supported", entry.Kind)
	}
	return nil
}

func validateRebuildState(state RebuildState, manifestDigest string) error {
	switch state.Status {
	case RebuildStatusClean:
		if state.InvalidCount != 0 {
			return fmt.Errorf("clean state must have zero invalid claims")
		}
	case RebuildStatusRejected:
		if state.InvalidCount <= 0 {
			return fmt.Errorf("rejected state must have at least one invalid claim")
		}
	default:
		return fmt.Errorf("status %q is not supported", state.Status)
	}
	if state.ManifestDigest != manifestDigest || !isSHA256Digest(state.ManifestDigest) {
		return fmt.Errorf("manifest digest is invalid or does not match trust inputs")
	}
	if state.RebuiltAt == "" {
		return fmt.Errorf("rebuilt_at is required")
	}
	if _, err := time.Parse(time.RFC3339, state.RebuiltAt); err != nil {
		return fmt.Errorf("rebuilt_at must be RFC3339: %w", err)
	}
	return nil
}

func isSHA256Digest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
