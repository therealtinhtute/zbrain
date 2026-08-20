package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestChallengeLifecycle(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	fixedNow := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	store := ChallengeStore{Paths: paths, Now: func() time.Time { return fixedNow }}

	claimID := "clm_0123456789abcdef0123456789abcdef"
	prepare := ChallengePrepare{
		Workspace:               "research",
		Operation:               ChallengeOperationApprove,
		ClaimID:                 claimID,
		CanonicalDraftDigest:    "sha256:" + strings.Repeat("a", 64),
		SupersededIDs:           []string{"clm_11111111111111111111111111111111"},
		PriorVerificationDigest: "sha256:" + strings.Repeat("b", 64),
		RevokeReason:            "corrected scope",
	}

	prepared, err := store.Prepare("research", prepare)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	challenge := prepared.Challenge
	if !challengeIDPattern.MatchString(challenge.ID) {
		t.Fatalf("challenge id = %q, want chg_<32 hex>", challenge.ID)
	}
	if !strings.HasPrefix(challenge.ActionDigest, "sha256:challenge-v1:") {
		t.Fatalf("action digest = %q, want sha256:challenge-v1: prefix", challenge.ActionDigest)
	}
	wantExpiry := fixedNow.UTC().Add(challengeLifetime).Format(time.RFC3339)
	if challenge.ExpiresAt != wantExpiry {
		t.Fatalf("ExpiresAt = %q, want %q", challenge.ExpiresAt, wantExpiry)
	}
	if prepared.Token == "" {
		t.Fatal("Prepare() returned an empty token")
	}
	if challenge.TokenSHA256 == prepared.Token {
		t.Fatalf("plaintext token persisted as token hash")
	}

	// The persisted record binds every bound input and recomputes the same digest.
	read, err := store.Read("research", challenge.ID)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if read.ActionDigest != challenge.ActionDigest {
		t.Fatalf("Read() digest = %q, want %q", read.ActionDigest, challenge.ActionDigest)
	}
	if read.TokenSHA256 != challenge.TokenSHA256 {
		t.Fatalf("Read() token hash = %q, want %q", read.TokenSHA256, challenge.TokenSHA256)
	}

	// Verify token is valid and not consumed by Verify.
	if _, err := store.Verify("research", challenge.ID, prepared.Token); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	// Consume once succeeds, a second consume fails closed (one-time token).
	if _, err := store.Consume("research", challenge.ID, prepared.Token); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if _, err := store.Consume("research", challenge.ID, prepared.Token); err == nil {
		t.Fatal("Consume() second use error = nil, want already consumed")
	}
	if _, err := store.Verify("research", challenge.ID, prepared.Token); err == nil {
		t.Fatal("Verify() after consume error = nil, want already consumed")
	}

	// A wrong token fails even on a fresh challenge.
	second, err := store.Prepare("research", prepare)
	if err != nil {
		t.Fatalf("Prepare() second error = %v", err)
	}
	if _, err := store.Verify("research", second.Challenge.ID, "not-the-token"); err == nil {
		t.Fatal("Verify(wrong token) error = nil")
	}
}

func TestChallengeLifecycleNoPlaintextTokenOnDisk(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	prepare := ChallengePrepare{
		Workspace:            "research",
		Operation:            ChallengeOperationApprove,
		ClaimID:              "clm_0123456789abcdef0123456789abcdef",
		CanonicalDraftDigest: "sha256:" + strings.Repeat("a", 64),
	}
	prepared, err := store.Prepare("research", prepare)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	path := filepath.Join(paths.WorkspacesDir, "research", ".zbrain", "challenges", prepared.Challenge.ID+".json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(contents), prepared.Token) {
		t.Fatalf("plaintext token persisted on disk: %q", prepared.Token)
	}
	sum := sha256.Sum256([]byte(prepared.Token))
	wantHash := "sha256:" + hex.EncodeToString(sum[:])
	if !strings.Contains(string(contents), wantHash) {
		t.Fatalf("token SHA-256 %q not found in persisted challenge", wantHash)
	}
}

func TestChallengeLifecycleDigestBindsAllInputs(t *testing.T) {
	base := ChallengePrepare{
		Workspace:               "research",
		Operation:               ChallengeOperationSupersede,
		ClaimID:                 "clm_0123456789abcdef0123456789abcdef",
		CanonicalDraftDigest:    "sha256:" + strings.Repeat("a", 64),
		SupersededIDs:           []string{"clm_11111111111111111111111111111111"},
		PriorVerificationDigest: "sha256:" + strings.Repeat("b", 64),
		RevokeReason:            "corrected scope",
	}
	baseline := ComputeChallengeActionDigest(base)

	mutations := map[string]func(c ChallengePrepare) ChallengePrepare{
		"workspace": func(c ChallengePrepare) ChallengePrepare {
			c.Workspace = "other"
			return c
		},
		"operation": func(c ChallengePrepare) ChallengePrepare {
			c.Operation = ChallengeOperationRevoke
			return c
		},
		"claim_id": func(c ChallengePrepare) ChallengePrepare {
			c.ClaimID = "clm_22222222222222222222222222222222"
			return c
		},
		"canonical_draft_digest": func(c ChallengePrepare) ChallengePrepare {
			c.CanonicalDraftDigest = "sha256:" + strings.Repeat("c", 64)
			return c
		},
		"superseded_ids": func(c ChallengePrepare) ChallengePrepare {
			c.SupersededIDs = []string{"clm_33333333333333333333333333333333"}
			return c
		},
		"prior_verification_digest": func(c ChallengePrepare) ChallengePrepare {
			c.PriorVerificationDigest = "sha256:" + strings.Repeat("d", 64)
			return c
		},
		"revoke_reason": func(c ChallengePrepare) ChallengePrepare {
			c.RevokeReason = "different reason"
			return c
		},
	}
	for name, mutate := range mutations {
		got := ComputeChallengeActionDigest(mutate(base))
		if got == baseline {
			t.Fatalf("changing %s did not change the action digest", name)
		}
	}

	// Superseded ID ordering must be canonical (order-insensitive).
	reordered := base
	reordered.SupersededIDs = []string{base.SupersededIDs[0]}
	if got := ComputeChallengeActionDigest(reordered); got != baseline {
		t.Fatalf("single superseded id digest differs from sorted canonical digest")
	}
}

func TestChallengeLifecycleExpiryEnforced(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	baseTime := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	store := ChallengeStore{Paths: paths, Now: func() time.Time { return baseTime }}
	prepare := ChallengePrepare{
		Workspace:            "research",
		Operation:            ChallengeOperationApprove,
		ClaimID:              "clm_0123456789abcdef0123456789abcdef",
		CanonicalDraftDigest: "sha256:" + strings.Repeat("a", 64),
	}
	prepared, err := store.Prepare("research", prepare)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	// Still valid at exactly 15 minutes.
	atExpiry := ChallengeStore{Paths: paths, Now: func() time.Time {
		return baseTime.Add(challengeLifetime)
	}}
	if _, err := atExpiry.Verify("research", prepared.Challenge.ID, prepared.Token); err != nil {
		t.Fatalf("Verify(at expiry) error = %v", err)
	}

	// Expired one microsecond past 15 minutes.
	afterExpiry := ChallengeStore{Paths: paths, Now: func() time.Time {
		return baseTime.Add(challengeLifetime).Add(time.Microsecond)
	}}
	if _, err := afterExpiry.Verify("research", prepared.Challenge.ID, prepared.Token); err == nil {
		t.Fatal("Verify(after expiry) error = nil, want expired")
	}
	if _, err := afterExpiry.Consume("research", prepared.Challenge.ID, prepared.Token); err == nil {
		t.Fatal("Consume(after expiry) error = nil, want expired")
	}
}

func TestChallengeLifecycleWorkspaceIsolation(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	prepare := ChallengePrepare{
		Workspace:            "research",
		Operation:            ChallengeOperationApprove,
		ClaimID:              "clm_0123456789abcdef0123456789abcdef",
		CanonicalDraftDigest: "sha256:" + strings.Repeat("a", 64),
	}
	prepared, err := store.Prepare("research", prepare)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	if err := CreateWorkspace(paths, "second", fixedClaimStoreNow()); err != nil {
		t.Fatalf("CreateWorkspace(second) error = %v", err)
	}
	if _, err := store.Read("second", prepared.Challenge.ID); err == nil {
		t.Fatal("Read(challenge from other workspace) error = nil")
	}
	if _, err := store.Verify("second", prepared.Challenge.ID, prepared.Token); err == nil {
		t.Fatal("Verify(challenge from other workspace) error = nil")
	}
}

func TestChallengeLifecycleRejectsInvalidPrepare(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}

	valid := ChallengePrepare{
		Workspace:            "research",
		Operation:            ChallengeOperationApprove,
		ClaimID:              "clm_0123456789abcdef0123456789abcdef",
		CanonicalDraftDigest: "sha256:" + strings.Repeat("a", 64),
	}
	tests := []struct {
		name   string
		mutate func(c ChallengePrepare) ChallengePrepare
	}{
		{
			name: "workspace mismatch",
			mutate: func(c ChallengePrepare) ChallengePrepare {
				c.Workspace = "other"
				return c
			},
		},
		{
			name: "unsupported operation",
			mutate: func(c ChallengePrepare) ChallengePrepare {
				c.Operation = ChallengeOperation("publish")
				return c
			},
		},
		{
			name: "invalid claim id",
			mutate: func(c ChallengePrepare) ChallengePrepare {
				c.ClaimID = "not-a-claim"
				return c
			},
		},
		{
			name: "invalid draft digest",
			mutate: func(c ChallengePrepare) ChallengePrepare {
				c.CanonicalDraftDigest = "md5:abc"
				return c
			},
		},
		{
			name: "invalid prior digest",
			mutate: func(c ChallengePrepare) ChallengePrepare {
				c.PriorVerificationDigest = "not-a-digest"
				return c
			},
		},
		{
			name: "invalid superseded id",
			mutate: func(c ChallengePrepare) ChallengePrepare {
				c.SupersededIDs = []string{"../escape"}
				return c
			},
		},
		{
			name: "revoke without reason",
			mutate: func(c ChallengePrepare) ChallengePrepare {
				c.Operation = ChallengeOperationRevoke
				c.RevokeReason = ""
				return c
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := tt.mutate(valid)
			if _, err := store.Prepare("research", candidate); err == nil {
				t.Fatalf("Prepare() error = nil for %s", tt.name)
			}
		})
	}
}
