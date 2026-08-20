package runtime

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ChallengeSchemaVersion identifies the persisted challenge record shape.
const ChallengeSchemaVersion = "zbrain.challenge/v1"

// challengeLifetime is the owner-pinned challenge TTL.
const challengeLifetime = 15 * time.Minute

// ChallengeOperation is the lifecycle operation a challenge pins.
type ChallengeOperation string

const (
	ChallengeOperationApprove   ChallengeOperation = "approve"
	ChallengeOperationSupersede ChallengeOperation = "supersede"
	ChallengeOperationRevoke    ChallengeOperation = "revoke"
)

// ChallengePrepare carries every input bound by the challenge action digest.
// All fields are always bound, even when empty for a given operation.
type ChallengePrepare struct {
	Workspace               string
	Operation               ChallengeOperation
	ClaimID                 string
	CanonicalDraftDigest    string
	SupersededIDs           []string
	PriorVerificationDigest string
	RevokeReason            string
}

// Challenge is the persisted owner-pinned challenge record. Only the token
// SHA-256 is stored; the plaintext one-time token is never persisted.
type Challenge struct {
	Schema                  string              `json:"schema"`
	ID                      string              `json:"id"`
	Workspace               string              `json:"workspace"`
	Operation               ChallengeOperation  `json:"operation"`
	ClaimID                 string              `json:"claim_id"`
	CanonicalDraftDigest    string              `json:"canonical_draft_digest,omitempty"`
	SupersededIDs           []string            `json:"superseded_ids,omitempty"`
	PriorVerificationDigest string              `json:"prior_verification_digest,omitempty"`
	RevokeReason            string              `json:"revoke_reason,omitempty"`
	ActionDigest            string              `json:"action_digest"`
	TokenSHA256             string              `json:"token_sha256"`
	ExpiresAt               string              `json:"expires_at"`
	Consumed                bool                `json:"consumed"`
}

// PreparedChallenge returns a prepared challenge together with its plaintext
// one-time token. The token is handed to the caller and never persisted.
type PreparedChallenge struct {
	Challenge Challenge
	Token     string
}

// ChallengeStore prepares and verifies owner-pinned lifecycle challenges.
type ChallengeStore struct {
	Paths Paths
	Now   func() time.Time
}

var challengeIDPattern = regexp.MustCompile(`^chg_[0-9a-f]{32}$`)

// NewChallengeID returns a new challenge ID.
func NewChallengeID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "chg_" + hex.EncodeToString(buf), nil
}

// ComputeChallengeActionDigest binds workspace, operation, claim ID, canonical
// draft digest, superseded IDs, prior verification digest, and revoke reason
// into a deterministic SHA-256 digest. Superseded IDs are sorted so the digest
// is canonical regardless of caller ordering.
func ComputeChallengeActionDigest(prepare ChallengePrepare) string {
	superseded := append([]string(nil), prepare.SupersededIDs...)
	sort.Strings(superseded)
	hasher := sha256.New()
	write := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		hasher.Write(length[:])
		hasher.Write([]byte(value))
	}
	write(prepare.Workspace)
	write(string(prepare.Operation))
	write(prepare.ClaimID)
	write(prepare.CanonicalDraftDigest)
	write(strings.Join(superseded, "\x00"))
	write(prepare.PriorVerificationDigest)
	write(prepare.RevokeReason)
	return "sha256:challenge-v1:" + hex.EncodeToString(hasher.Sum(nil))
}

// Prepare creates a new owner-pinned challenge. It binds every listed input
// into the action digest, generates a one-time token, persists only the token
// SHA-256, and expires the challenge after 15 minutes.
func (store ChallengeStore) Prepare(workspace string, prepare ChallengePrepare) (PreparedChallenge, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return PreparedChallenge{}, err
	}
	defer func() { _ = lock.Close() }()
	return store.prepareUnlocked(workspace, prepare)
}

func (store ChallengeStore) prepareUnlocked(workspace string, prepare ChallengePrepare) (PreparedChallenge, error) {
	if err := store.validatePrepare(workspace, prepare); err != nil {
		return PreparedChallenge{}, err
	}
	id, err := NewChallengeID()
	if err != nil {
		return PreparedChallenge{}, err
	}
	token, tokenHash, err := newChallengeToken()
	if err != nil {
		return PreparedChallenge{}, err
	}
	superseded := append([]string(nil), prepare.SupersededIDs...)
	sort.Strings(superseded)
	challenge := Challenge{
		Schema:                  ChallengeSchemaVersion,
		ID:                      id,
		Workspace:               workspace,
		Operation:               prepare.Operation,
		ClaimID:                 prepare.ClaimID,
		CanonicalDraftDigest:    prepare.CanonicalDraftDigest,
		SupersededIDs:           superseded,
		PriorVerificationDigest: prepare.PriorVerificationDigest,
		RevokeReason:            prepare.RevokeReason,
		ActionDigest:            ComputeChallengeActionDigest(prepare),
		TokenSHA256:             tokenHash,
		ExpiresAt:               store.now().UTC().Add(challengeLifetime).Format(time.RFC3339),
	}
	if err := store.writeChallenge(workspace, challenge); err != nil {
		return PreparedChallenge{}, err
	}
	return PreparedChallenge{Challenge: challenge, Token: token}, nil
}

// Read returns a persisted challenge by ID without consuming it.
func (store ChallengeStore) Read(workspace string, challengeID string) (Challenge, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, false)
	if err != nil {
		return Challenge{}, err
	}
	defer func() { _ = lock.Close() }()
	return store.readChallengeUnlocked(workspace, challengeID)
}

// Verify validates a plaintext token against a fresh, unconsumed challenge
// without consuming it.
func (store ChallengeStore) Verify(workspace string, challengeID string, token string) (Challenge, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, false)
	if err != nil {
		return Challenge{}, err
	}
	defer func() { _ = lock.Close() }()
	challenge, err := store.readChallengeUnlocked(workspace, challengeID)
	if err != nil {
		return Challenge{}, err
	}
	if err := store.validateToken(challenge, token); err != nil {
		return Challenge{}, err
	}
	return challenge, nil
}

// Consume atomically validates a plaintext token, enforces expiry, and marks
// the challenge consumed under the workspace lock so the token is one-time.
func (store ChallengeStore) Consume(workspace string, challengeID string, token string) (Challenge, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return Challenge{}, err
	}
	defer func() { _ = lock.Close() }()
	return store.consumeUnlocked(workspace, challengeID, token)
}

func (store ChallengeStore) consumeUnlocked(workspace string, challengeID string, token string) (Challenge, error) {
	challenge, err := store.readChallengeUnlocked(workspace, challengeID)
	if err != nil {
		return Challenge{}, err
	}
	if err := store.validateToken(challenge, token); err != nil {
		return Challenge{}, err
	}
	challenge.Consumed = true
	if err := store.writeChallenge(workspace, challenge); err != nil {
		return Challenge{}, err
	}
	return challenge, nil
}

func (store ChallengeStore) validateToken(challenge Challenge, token string) error {
	if challenge.Consumed {
		return fmt.Errorf("challenge %s token already consumed", challenge.ID)
	}
	expiresAt, err := time.Parse(time.RFC3339, challenge.ExpiresAt)
	if err != nil {
		return fmt.Errorf("challenge %s has invalid expiry: %w", challenge.ID, err)
	}
	if store.now().After(expiresAt) {
		return fmt.Errorf("challenge %s token expired", challenge.ID)
	}
	sum := sha256.Sum256([]byte(token))
	if "sha256:"+hex.EncodeToString(sum[:]) != challenge.TokenSHA256 {
		return fmt.Errorf("challenge %s token mismatch", challenge.ID)
	}
	return nil
}

func (store ChallengeStore) validatePrepare(workspace string, prepare ChallengePrepare) error {
	if !IsSafeWorkspaceName(workspace) {
		return fmt.Errorf("challenge workspace name is not safe")
	}
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return err
	}
	if prepare.Workspace != workspace {
		return fmt.Errorf("challenge workspace %q does not match %q", prepare.Workspace, workspace)
	}
	switch prepare.Operation {
	case ChallengeOperationApprove, ChallengeOperationSupersede, ChallengeOperationRevoke:
	default:
		return fmt.Errorf("challenge operation %q is not supported", prepare.Operation)
	}
	if !claimIDPattern.MatchString(prepare.ClaimID) {
		return fmt.Errorf("challenge claim id must match clm_<32 lowercase hex chars>")
	}
	if prepare.CanonicalDraftDigest != "" && !strings.HasPrefix(prepare.CanonicalDraftDigest, "sha256:") {
		return fmt.Errorf("challenge canonical draft digest must use sha256:<hex>")
	}
	if prepare.PriorVerificationDigest != "" && !strings.HasPrefix(prepare.PriorVerificationDigest, "sha256:") {
		return fmt.Errorf("challenge prior verification digest must use sha256:<hex>")
	}
	for _, id := range prepare.SupersededIDs {
		if !claimIDPattern.MatchString(id) {
			return fmt.Errorf("challenge superseded id %q must match clm_<32 lowercase hex chars>", id)
		}
	}
	if prepare.Operation == ChallengeOperationRevoke && strings.TrimSpace(prepare.RevokeReason) == "" {
		return fmt.Errorf("revoke challenge requires a revoke reason")
	}
	return nil
}

func (store ChallengeStore) readChallengeUnlocked(workspace string, challengeID string) (Challenge, error) {
	path, err := store.challengePath(workspace, challengeID)
	if err != nil {
		return Challenge{}, err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return Challenge{}, err
	}
	var challenge Challenge
	if err := json.Unmarshal(contents, &challenge); err != nil {
		return Challenge{}, fmt.Errorf("decode challenge %s: %w", challengeID, err)
	}
	if err := store.validateChallengeRecord(workspace, challenge); err != nil {
		return Challenge{}, err
	}
	return challenge, nil
}

// validateChallengeRecord enforces the persisted record shape, workspace
// binding, and digest integrity so a tampered or misplaced challenge fails
// closed on every read.
func (store ChallengeStore) validateChallengeRecord(workspace string, challenge Challenge) error {
	if challenge.Schema != ChallengeSchemaVersion {
		return fmt.Errorf("challenge %s schema mismatch", challenge.ID)
	}
	if !challengeIDPattern.MatchString(challenge.ID) {
		return fmt.Errorf("challenge id %q must match chg_<32 lowercase hex chars>", challenge.ID)
	}
	if challenge.Workspace != workspace {
		return fmt.Errorf("challenge %s workspace %q does not match %q", challenge.ID, challenge.Workspace, workspace)
	}
	switch challenge.Operation {
	case ChallengeOperationApprove, ChallengeOperationSupersede, ChallengeOperationRevoke:
	default:
		return fmt.Errorf("challenge %s operation %q is not supported", challenge.ID, challenge.Operation)
	}
	if !claimIDPattern.MatchString(challenge.ClaimID) {
		return fmt.Errorf("challenge %s claim id must match clm_<32 lowercase hex chars>", challenge.ID)
	}
	if challenge.CanonicalDraftDigest != "" && !strings.HasPrefix(challenge.CanonicalDraftDigest, "sha256:") {
		return fmt.Errorf("challenge %s canonical draft digest must use sha256:<hex>", challenge.ID)
	}
	if challenge.PriorVerificationDigest != "" && !strings.HasPrefix(challenge.PriorVerificationDigest, "sha256:") {
		return fmt.Errorf("challenge %s prior verification digest must use sha256:<hex>", challenge.ID)
	}
	for _, id := range challenge.SupersededIDs {
		if !claimIDPattern.MatchString(id) {
			return fmt.Errorf("challenge %s superseded id %q must match clm_<32 lowercase hex chars>", challenge.ID, id)
		}
	}
	if !isChallengeTokenHash(challenge.TokenSHA256) {
		return fmt.Errorf("challenge %s token hash must use sha256:<hex>", challenge.ID)
	}
	if _, err := time.Parse(time.RFC3339, challenge.ExpiresAt); err != nil {
		return fmt.Errorf("challenge %s expires_at must be RFC3339: %w", challenge.ID, err)
	}
	expected := ComputeChallengeActionDigest(ChallengePrepare{
		Workspace:               challenge.Workspace,
		Operation:               challenge.Operation,
		ClaimID:                 challenge.ClaimID,
		CanonicalDraftDigest:    challenge.CanonicalDraftDigest,
		SupersededIDs:           challenge.SupersededIDs,
		PriorVerificationDigest: challenge.PriorVerificationDigest,
		RevokeReason:            challenge.RevokeReason,
	})
	if expected != challenge.ActionDigest {
		return fmt.Errorf("challenge %s action digest mismatch", challenge.ID)
	}
	return nil
}

func (store ChallengeStore) challengePath(workspace string, challengeID string) (string, error) {
	if !challengeIDPattern.MatchString(challengeID) {
		return "", fmt.Errorf("challenge id must match chg_<32 lowercase hex chars>")
	}
	root, err := ValidateWorkspace(store.Paths, workspace)
	if err != nil {
		return "", err
	}
	controlDirectory := filepath.Join(root, workspaceControlDirectoryName)
	if err := validateWorkspaceControlDirectory(root, controlDirectory); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	directory := filepath.Join(controlDirectory, "challenges")
	if err := validateChallengeDirectory(root, directory); err != nil {
		return "", err
	}
	path := filepath.Join(directory, challengeID+".json")
	if err := validateWorkspaceControlFile(path); err != nil {
		return "", err
	}
	return path, nil
}

func (store ChallengeStore) writeChallenge(workspace string, challenge Challenge) error {
	path, err := store.challengePath(workspace, challenge.ID)
	if err != nil {
		return err
	}
	if err := ensureDirectoryMode(filepath.Dir(path), runtimeDirectoryMode); err != nil {
		return fmt.Errorf("create challenges directory: %w", err)
	}
	if err := store.validateChallengeRecord(workspace, challenge); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(challenge, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal challenge %s: %w", challenge.ID, err)
	}
	encoded = append(encoded, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+challenge.ID+".*.tmp")
	if err != nil {
		return fmt.Errorf("create challenge temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(runtimeMetadataMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set challenge temporary permissions: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write challenge %s: %w", challenge.ID, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync challenge %s: %w", challenge.ID, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close challenge %s: %w", challenge.ID, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish challenge %s: %w", challenge.ID, err)
	}
	return ensureFileMode(path, runtimeMetadataMode)
}

func (store ChallengeStore) now() time.Time {
	if store.Now != nil {
		return store.Now()
	}
	return time.Now()
}

func newChallengeToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(token))
	return token, "sha256:" + hex.EncodeToString(sum[:]), nil
}

func isChallengeTokenHash(value string) bool {
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(decoded) == sha256.Size
}

func validateChallengeDirectory(root string, directory string) error {
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("challenges directory must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("challenges directory is not a directory")
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return err
	}
	if !pathWithin(root, resolved) {
		return fmt.Errorf("challenges directory resolves outside workspace")
	}
	return nil
}
