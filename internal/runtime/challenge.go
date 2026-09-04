package runtime

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ChallengeSchemaVersion identifies the persisted challenge record shape.
// Security-critical challenge records are fail-closed across schema changes.
const ChallengeSchemaVersion = "zbrain.challenge/v3"

const (
	// challengeLifetime is the owner-pinned challenge TTL.
	challengeLifetime = 15 * time.Minute
	// challengeTokenLifetime is independent from the challenge TTL. A valid
	// challenge may outlive the one-time grant token it carries.
	challengeTokenLifetime = 5 * time.Minute
)

// ChallengeOperation is the lifecycle operation a challenge pins.
type ChallengeOperation string

const (
	ChallengeOperationApprove   ChallengeOperation = "approve"
	ChallengeOperationSupersede ChallengeOperation = "supersede"
	ChallengeOperationRevoke    ChallengeOperation = "revoke"
)

// ChallengeItem is one ordered approve action bound by a batch challenge.
type ChallengeItem struct {
	ClaimID              string `json:"claim_id"`
	CanonicalDraftDigest string `json:"canonical_draft_digest"`
}

// ChallengePrepare carries every input bound by the challenge action digest.
// All fields are always bound, even when empty for a given operation. A
// non-empty Items list binds an ordered batch of approve items instead of the
// single-claim fields.
type ChallengePrepare struct {
	Workspace               string
	Operation               ChallengeOperation
	ClaimID                 string
	CanonicalDraftDigest    string
	SupersededIDs           []string
	PriorVerificationDigest string
	RevokeReason            string
	Items                   []ChallengeItem
}

// Challenge is the persisted owner-pinned challenge record. Only the token
// SHA-256 is stored; the plaintext one-time token is never persisted. The
// owner-granted marker is persisted so apply cannot bypass the local ceremony.
type Challenge struct {
	Schema                  string             `json:"schema"`
	ID                      string             `json:"id"`
	Workspace               string             `json:"workspace"`
	Operation               ChallengeOperation `json:"operation"`
	ClaimID                 string             `json:"claim_id"`
	CanonicalDraftDigest    string             `json:"canonical_draft_digest,omitempty"`
	SupersededIDs           []string           `json:"superseded_ids,omitempty"`
	PriorVerificationDigest string             `json:"prior_verification_digest,omitempty"`
	RevokeReason            string             `json:"revoke_reason,omitempty"`
	Items                   []ChallengeItem    `json:"items,omitempty"`
	GrantedItems            []string           `json:"granted_items,omitempty"`
	SkippedItems            []string           `json:"skipped_items,omitempty"`
	ActionDigest            string             `json:"action_digest"`
	TokenSHA256             string             `json:"token_sha256"`
	ExpiresAt               string             `json:"expires_at"`
	TokenExpiresAt          string             `json:"token_expires_at"`
	Granted                 bool               `json:"granted"`
	GrantedAt               string             `json:"granted_at,omitempty"`
	Consumed                bool               `json:"consumed"`
}

// PreparedChallenge contains the owner-pinned action summary. No token is
// issued until the local owner ceremony calls Grant.
type PreparedChallenge struct {
	Challenge Challenge
}

// GrantedChallenge contains the owner-approved challenge and the plaintext
// one-time token released exactly once by Grant. The token is never persisted.
type GrantedChallenge struct {
	Challenge
	Token string
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
// is canonical regardless of caller ordering. A non-empty Items list binds an
// ordered batch of approve items instead; item order is significant.
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
	if len(prepare.Items) > 0 {
		var count [8]byte
		binary.BigEndian.PutUint64(count[:], uint64(len(prepare.Items)))
		hasher.Write(count[:])
		for _, item := range prepare.Items {
			write(item.ClaimID)
			write(item.CanonicalDraftDigest)
		}
		return "sha256:challenge-v1:" + hex.EncodeToString(hasher.Sum(nil))
	}
	write(prepare.ClaimID)
	write(prepare.CanonicalDraftDigest)
	write(strings.Join(superseded, "\x00"))
	write(prepare.PriorVerificationDigest)
	write(prepare.RevokeReason)
	return "sha256:challenge-v1:" + hex.EncodeToString(hasher.Sum(nil))
}

// Prepare creates a new owner-pinned challenge. It binds every listed input
// into the action digest and persists no token until the local owner ceremony
// releases one through Grant.
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
	superseded := append([]string(nil), prepare.SupersededIDs...)
	sort.Strings(superseded)
	preparedAt := store.now().UTC()
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
		ExpiresAt:               preparedAt.Add(challengeLifetime).Format(time.RFC3339),
	}
	if err := store.writeChallenge(workspace, challenge); err != nil {
		return PreparedChallenge{}, err
	}
	return PreparedChallenge{Challenge: challenge}, nil
}

// PrepareBatch creates a new owner-pinned batch challenge binding an ordered
// list of approve items. It persists no token until GrantItems records the
// owner's per-item decisions at the end of the grant walk.
func (store ChallengeStore) PrepareBatch(workspace string, items []ChallengeItem) (PreparedChallenge, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return PreparedChallenge{}, err
	}
	defer func() { _ = lock.Close() }()
	return store.prepareBatchUnlocked(workspace, items)
}

func (store ChallengeStore) prepareBatchUnlocked(workspace string, items []ChallengeItem) (PreparedChallenge, error) {
	normalized, err := normalizeChallengeItems(items)
	if err != nil {
		return PreparedChallenge{}, err
	}
	prepare := ChallengePrepare{
		Workspace: workspace,
		Operation: ChallengeOperationApprove,
		Items:     normalized,
	}
	if err := store.validatePrepare(workspace, prepare); err != nil {
		return PreparedChallenge{}, err
	}
	id, err := NewChallengeID()
	if err != nil {
		return PreparedChallenge{}, err
	}
	challenge := Challenge{
		Schema:       ChallengeSchemaVersion,
		ID:           id,
		Workspace:    workspace,
		Operation:    ChallengeOperationApprove,
		Items:        normalized,
		ActionDigest: ComputeChallengeActionDigest(prepare),
		ExpiresAt:    store.now().UTC().Add(challengeLifetime).Format(time.RFC3339),
	}
	if err := store.writeChallenge(workspace, challenge); err != nil {
		return PreparedChallenge{}, err
	}
	return PreparedChallenge{Challenge: challenge}, nil
}

func normalizeChallengeItems(items []ChallengeItem) ([]ChallengeItem, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("batch challenge requires at least one item")
	}
	normalized := append([]ChallengeItem(nil), items...)
	seen := make(map[string]struct{}, len(normalized))
	for _, item := range normalized {
		if !claimIDPattern.MatchString(item.ClaimID) {
			return nil, fmt.Errorf("batch challenge claim id %q must match clm_<32 lowercase hex chars>", item.ClaimID)
		}
		if !strings.HasPrefix(item.CanonicalDraftDigest, "sha256:") {
			return nil, fmt.Errorf("batch challenge canonical draft digest for %q must use sha256:<hex>", item.ClaimID)
		}
		if _, ok := seen[item.ClaimID]; ok {
			return nil, fmt.Errorf("batch challenge binds claim %s more than once", item.ClaimID)
		}
		seen[item.ClaimID] = struct{}{}
	}
	return normalized, nil
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

// FindChallenge resolves a challenge ID to its owning workspace by scanning
// every workspace directory under the runtime. Challenge IDs are globally
// unique, so at most one workspace can own a challenge. A non-missing read
// error is surfaced rather than masked by a later workspace.
func (store ChallengeStore) FindChallenge(challengeID string) (Challenge, string, error) {
	if !challengeIDPattern.MatchString(challengeID) {
		return Challenge{}, "", fmt.Errorf("challenge id must match chg_<32 lowercase hex chars>")
	}
	entries, err := os.ReadDir(store.Paths.WorkspacesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Challenge{}, "", fmt.Errorf("challenge %s not found in any workspace", challengeID)
		}
		return Challenge{}, "", fmt.Errorf("list workspaces: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !IsSafeWorkspaceName(entry.Name()) {
			continue
		}
		challenge, err := store.Read(entry.Name(), challengeID)
		if err == nil {
			return challenge, entry.Name(), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Challenge{}, "", err
		}
	}
	return Challenge{}, "", fmt.Errorf("challenge %s not found in any workspace", challengeID)
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

// Grant records the local owner's approval and releases a fresh plaintext
// token without consuming it. The token hash and its grant-time expiry are
// persisted; the plaintext is returned only to the owner ceremony. A second
// grant is rejected because the original plaintext cannot be reconstructed.
func (store ChallengeStore) Grant(workspace string, challengeID string) (GrantedChallenge, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return GrantedChallenge{}, err
	}
	defer func() { _ = lock.Close() }()
	challenge, err := store.readChallengeUnlocked(workspace, challengeID)
	if err != nil {
		return GrantedChallenge{}, err
	}
	if challenge.Granted {
		return GrantedChallenge{}, fmt.Errorf("challenge %s has already been owner-granted", challenge.ID)
	}
	now := store.now().UTC()
	challengeExpiresAt, err := time.Parse(time.RFC3339, challenge.ExpiresAt)
	if err != nil {
		return GrantedChallenge{}, fmt.Errorf("challenge %s has invalid challenge expiry: %w", challenge.ID, err)
	}
	if now.After(challengeExpiresAt) {
		return GrantedChallenge{}, fmt.Errorf("challenge %s expired", challenge.ID)
	}
	token, tokenHash, err := newChallengeToken()
	if err != nil {
		return GrantedChallenge{}, err
	}
	tokenExpiresAt := now.Add(challengeTokenLifetime)
	if tokenExpiresAt.After(challengeExpiresAt) {
		tokenExpiresAt = challengeExpiresAt
	}
	challenge.TokenSHA256 = tokenHash
	challenge.TokenExpiresAt = tokenExpiresAt.Format(time.RFC3339)
	challenge.Granted = true
	challenge.GrantedAt = now.Format(time.RFC3339)
	if err := store.writeChallenge(workspace, challenge); err != nil {
		return GrantedChallenge{}, err
	}
	return GrantedChallenge{Challenge: challenge, Token: token}, nil
}

// GrantItems records the owner's per-item decisions for a batch challenge and
// releases one plaintext token for the whole batch. The granted claim IDs
// together with the skipped claim IDs must partition the bound items exactly.
// A walk with no granted items is rejected so a fully skipped challenge stays
// ungranted and can never yield a token. The token hash and its grant-time
// expiry are persisted; the plaintext is returned only to the owner ceremony.
func (store ChallengeStore) GrantItems(workspace string, challengeID string, granted []string, skipped []string) (GrantedChallenge, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return GrantedChallenge{}, err
	}
	defer func() { _ = lock.Close() }()
	challenge, err := store.readChallengeUnlocked(workspace, challengeID)
	if err != nil {
		return GrantedChallenge{}, err
	}
	if len(challenge.Items) == 0 {
		return GrantedChallenge{}, fmt.Errorf("challenge %s is not a batch challenge", challenge.ID)
	}
	if challenge.Granted {
		return GrantedChallenge{}, fmt.Errorf("challenge %s has already been owner-granted", challenge.ID)
	}
	itemIndex := make(map[string]struct{}, len(challenge.Items))
	for _, item := range challenge.Items {
		itemIndex[item.ClaimID] = struct{}{}
	}
	grantedSeen := make(map[string]struct{}, len(granted))
	for _, id := range granted {
		if _, ok := itemIndex[id]; !ok {
			return GrantedChallenge{}, fmt.Errorf("challenge %s granted item %q is not bound", challenge.ID, id)
		}
		if _, ok := grantedSeen[id]; ok {
			return GrantedChallenge{}, fmt.Errorf("challenge %s grants claim %s more than once", challenge.ID, id)
		}
		grantedSeen[id] = struct{}{}
	}
	skippedSeen := make(map[string]struct{}, len(skipped))
	for _, id := range skipped {
		if _, ok := itemIndex[id]; !ok {
			return GrantedChallenge{}, fmt.Errorf("challenge %s skipped item %q is not bound", challenge.ID, id)
		}
		if _, ok := grantedSeen[id]; ok {
			return GrantedChallenge{}, fmt.Errorf("challenge %s claim %s cannot be both granted and skipped", challenge.ID, id)
		}
		if _, ok := skippedSeen[id]; ok {
			return GrantedChallenge{}, fmt.Errorf("challenge %s skips claim %s more than once", challenge.ID, id)
		}
		skippedSeen[id] = struct{}{}
	}
	if len(granted)+len(skipped) != len(challenge.Items) {
		return GrantedChallenge{}, fmt.Errorf("challenge %s grant decisions do not cover every bound item", challenge.ID)
	}
	if len(granted) == 0 {
		return GrantedChallenge{}, fmt.Errorf("challenge %s granted no items; nothing to record", challenge.ID)
	}
	now := store.now().UTC()
	challengeExpiresAt, err := time.Parse(time.RFC3339, challenge.ExpiresAt)
	if err != nil {
		return GrantedChallenge{}, fmt.Errorf("challenge %s has invalid challenge expiry: %w", challenge.ID, err)
	}
	if now.After(challengeExpiresAt) {
		return GrantedChallenge{}, fmt.Errorf("challenge %s expired", challenge.ID)
	}
	token, tokenHash, err := newChallengeToken()
	if err != nil {
		return GrantedChallenge{}, err
	}
	tokenExpiresAt := now.Add(challengeTokenLifetime)
	if tokenExpiresAt.After(challengeExpiresAt) {
		tokenExpiresAt = challengeExpiresAt
	}
	challenge.GrantedItems = append([]string(nil), granted...)
	challenge.SkippedItems = append([]string(nil), skipped...)
	challenge.TokenSHA256 = tokenHash
	challenge.TokenExpiresAt = tokenExpiresAt.Format(time.RFC3339)
	challenge.Granted = true
	challenge.GrantedAt = now.Format(time.RFC3339)
	if err := store.writeChallenge(workspace, challenge); err != nil {
		return GrantedChallenge{}, err
	}
	return GrantedChallenge{Challenge: challenge, Token: token}, nil
}

// Consume atomically validates a plaintext token, enforces expiry, and marks
// the owner-granted challenge consumed under the workspace lock so the token
// is one-time.
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
	if !challenge.Granted {
		return Challenge{}, fmt.Errorf("challenge %s token has not been owner-granted", challenge.ID)
	}
	challenge.Consumed = true
	if err := store.writeChallenge(workspace, challenge); err != nil {
		return Challenge{}, err
	}
	return challenge, nil
}

func (store ChallengeStore) validateToken(challenge Challenge, token string) error {
	if !challenge.Granted {
		return fmt.Errorf("challenge %s has not been owner-granted", challenge.ID)
	}
	if challenge.Consumed {
		return fmt.Errorf("challenge %s token already consumed", challenge.ID)
	}
	now := store.now()
	challengeExpiresAt, err := time.Parse(time.RFC3339, challenge.ExpiresAt)
	if err != nil {
		return fmt.Errorf("challenge %s has invalid challenge expiry: %w", challenge.ID, err)
	}
	if now.After(challengeExpiresAt) {
		return fmt.Errorf("challenge %s expired", challenge.ID)
	}
	tokenExpiresAt, err := time.Parse(time.RFC3339, challenge.TokenExpiresAt)
	if err != nil {
		return fmt.Errorf("challenge %s has invalid token expiry: %w", challenge.ID, err)
	}
	if now.After(tokenExpiresAt) {
		return fmt.Errorf("challenge %s token expired", challenge.ID)
	}
	sum := sha256.Sum256([]byte(token))
	wantHash := "sha256:" + hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(wantHash), []byte(challenge.TokenSHA256)) != 1 {
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
	if len(prepare.Items) > 0 {
		if prepare.Operation != ChallengeOperationApprove {
			return fmt.Errorf("challenge operation %q is not supported for a batch", prepare.Operation)
		}
		if _, err := normalizeChallengeItems(prepare.Items); err != nil {
			return err
		}
		return nil
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
	if len(challenge.Items) > 0 {
		if challenge.Operation != ChallengeOperationApprove {
			return fmt.Errorf("challenge %s operation %q is not supported for a batch", challenge.ID, challenge.Operation)
		}
		if challenge.ClaimID != "" || challenge.CanonicalDraftDigest != "" || len(challenge.SupersededIDs) != 0 || challenge.PriorVerificationDigest != "" || challenge.RevokeReason != "" {
			return fmt.Errorf("challenge %s batch items exclude single-claim fields", challenge.ID)
		}
		if _, err := normalizeChallengeItems(challenge.Items); err != nil {
			return fmt.Errorf("challenge %s batch items are invalid: %w", challenge.ID, err)
		}
	} else {
		if len(challenge.GrantedItems) != 0 || len(challenge.SkippedItems) != 0 {
			return fmt.Errorf("challenge %s item decisions require bound batch items", challenge.ID)
		}
	}
	if len(challenge.Items) == 0 && !claimIDPattern.MatchString(challenge.ClaimID) {
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
	challengeExpiresAt, err := time.Parse(time.RFC3339, challenge.ExpiresAt)
	if err != nil {
		return fmt.Errorf("challenge %s expires_at must be RFC3339: %w", challenge.ID, err)
	}
	if challenge.Consumed && !challenge.Granted {
		return fmt.Errorf("challenge %s is consumed without an owner grant", challenge.ID)
	}
	if challenge.Granted {
		if !isChallengeTokenHash(challenge.TokenSHA256) {
			return fmt.Errorf("challenge %s token hash must use sha256:<hex>", challenge.ID)
		}
		if strings.TrimSpace(challenge.TokenExpiresAt) == "" {
			return fmt.Errorf("challenge %s token_expires_at is required when granted", challenge.ID)
		}
		tokenExpiresAt, err := time.Parse(time.RFC3339, challenge.TokenExpiresAt)
		if err != nil {
			return fmt.Errorf("challenge %s token_expires_at must be RFC3339: %w", challenge.ID, err)
		}
		if tokenExpiresAt.After(challengeExpiresAt) {
			return fmt.Errorf("challenge %s token expiry must not outlive challenge expiry", challenge.ID)
		}
		if strings.TrimSpace(challenge.GrantedAt) == "" {
			return fmt.Errorf("challenge %s granted_at is required when granted", challenge.ID)
		}
		if _, err := time.Parse(time.RFC3339, challenge.GrantedAt); err != nil {
			return fmt.Errorf("challenge %s granted_at must be RFC3339: %w", challenge.ID, err)
		}
	} else {
		if challenge.TokenSHA256 != "" || challenge.TokenExpiresAt != "" {
			return fmt.Errorf("challenge %s token material requires owner grant", challenge.ID)
		}
		if strings.TrimSpace(challenge.GrantedAt) != "" {
			return fmt.Errorf("challenge %s granted_at requires granted state", challenge.ID)
		}
	}
	if len(challenge.Items) > 0 {
		if err := validateChallengeItemDecisions(challenge); err != nil {
			return err
		}
	}
	expected := ComputeChallengeActionDigest(ChallengePrepare{
		Workspace:               challenge.Workspace,
		Operation:               challenge.Operation,
		ClaimID:                 challenge.ClaimID,
		CanonicalDraftDigest:    challenge.CanonicalDraftDigest,
		SupersededIDs:           challenge.SupersededIDs,
		PriorVerificationDigest: challenge.PriorVerificationDigest,
		RevokeReason:            challenge.RevokeReason,
		Items:                   challenge.Items,
	})
	if expected != challenge.ActionDigest {
		return fmt.Errorf("challenge %s action digest mismatch", challenge.ID)
	}
	return nil
}

// validateChallengeItemDecisions enforces that a granted batch challenge
// records granted and skipped claim IDs that partition the bound items exactly.
func validateChallengeItemDecisions(challenge Challenge) error {
	if !challenge.Granted {
		if len(challenge.GrantedItems) != 0 || len(challenge.SkippedItems) != 0 {
			return fmt.Errorf("challenge %s item decisions require owner grant", challenge.ID)
		}
		return nil
	}
	itemIndex := make(map[string]struct{}, len(challenge.Items))
	for _, item := range challenge.Items {
		itemIndex[item.ClaimID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(challenge.Items))
	for _, id := range append(append([]string(nil), challenge.GrantedItems...), challenge.SkippedItems...) {
		if _, ok := itemIndex[id]; !ok {
			return fmt.Errorf("challenge %s item decision %q is not bound", challenge.ID, id)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("challenge %s records claim %s more than once", challenge.ID, id)
		}
		seen[id] = struct{}{}
	}
	if len(challenge.GrantedItems)+len(challenge.SkippedItems) != len(challenge.Items) {
		return fmt.Errorf("challenge %s item decisions do not cover every bound item", challenge.ID)
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
