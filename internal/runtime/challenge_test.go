package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	if challenge.TokenExpiresAt != "" {
		t.Fatalf("TokenExpiresAt = %q, want empty before owner grant", challenge.TokenExpiresAt)
	}
	if challenge.TokenSHA256 != "" || challenge.TokenExpiresAt != "" {
		t.Fatalf("Prepare() persisted token material: %#v", challenge)
	}

	// The persisted record binds every bound input and recomputes the same digest.
	read, err := store.Read("research", challenge.ID)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if read.ActionDigest != challenge.ActionDigest {
		t.Fatalf("Read() digest = %q, want %q", read.ActionDigest, challenge.ActionDigest)
	}
	if read.TokenSHA256 != "" || read.TokenExpiresAt != "" {
		t.Fatalf("Read() exposed token material before grant: %#v", read)
	}
	if _, err := store.Verify("research", challenge.ID, "not-issued"); err == nil || !strings.Contains(err.Error(), "has not been owner-granted") {
		t.Fatalf("Verify(before grant) error = %v, want owner-grant rejection", err)
	}

	// The owner grant issues the one-time token without consuming it.
	granted, err := store.Grant("research", challenge.ID)
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if !granted.Granted || granted.GrantedAt == "" || granted.Token == "" {
		t.Fatalf("Grant() state = %#v, want grant marker and token", granted)
	}
	token := granted.Token
	if _, err := store.Verify("research", challenge.ID, token); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if _, err := store.Grant("research", challenge.ID); err == nil || !strings.Contains(err.Error(), "already been owner-granted") {
		t.Fatalf("Grant(repeated) error = %v, want one-shot rejection", err)
	}
	// Consume once succeeds, a second consume fails closed (one-time token).
	if _, err := store.Consume("research", challenge.ID, token); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if _, err := store.Consume("research", challenge.ID, token); err == nil {
		t.Fatal("Consume() second use error = nil, want already consumed")
	}
	if _, err := store.Verify("research", challenge.ID, token); err == nil {
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
	var beforeGrant Challenge
	if err := json.Unmarshal(contents, &beforeGrant); err != nil {
		t.Fatalf("decode challenge before grant: %v", err)
	}
	if beforeGrant.TokenSHA256 != "" || beforeGrant.TokenExpiresAt != "" {
		t.Fatalf("token material persisted before grant: %#v", beforeGrant)
	}
	granted, err := (ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}).Grant("research", prepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	grantedContents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after grant) error = %v", err)
	}
	if strings.Contains(string(grantedContents), granted.Token) {
		t.Fatalf("plaintext token persisted after grant: %q", granted.Token)
	}
	sum := sha256.Sum256([]byte(granted.Token))
	wantHash := "sha256:" + hex.EncodeToString(sum[:])
	if !strings.Contains(string(grantedContents), wantHash) {
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

	granted, err := store.Grant("research", prepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	token := granted.Token
	// The token remains valid at the five-minute boundary from grant.
	atTokenExpiry := ChallengeStore{Paths: paths, Now: func() time.Time {
		return baseTime.Add(challengeTokenLifetime)
	}}
	if _, err := atTokenExpiry.Verify("research", prepared.Challenge.ID, token); err != nil {
		t.Fatalf("Verify(at token expiry) error = %v", err)
	}
	// Just after five minutes, the token expires while the challenge remains valid.
	afterTokenExpiry := ChallengeStore{Paths: paths, Now: func() time.Time {
		return baseTime.Add(challengeTokenLifetime).Add(time.Microsecond)
	}}
	if _, err := afterTokenExpiry.Verify("research", prepared.Challenge.ID, token); err == nil || !strings.Contains(err.Error(), "token expired") {
		t.Fatalf("Verify(after token expiry) error = %v, want token expired", err)
	}

	// Expired one microsecond past the independent 15-minute challenge lifetime.
	afterExpiry := ChallengeStore{Paths: paths, Now: func() time.Time {
		return baseTime.Add(challengeLifetime).Add(time.Microsecond)
	}}
	if _, err := afterExpiry.Verify("research", prepared.Challenge.ID, token); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("Verify(after challenge expiry) error = %v, want expired", err)
	}
	if _, err := afterExpiry.Consume("research", prepared.Challenge.ID, token); err == nil {
		t.Fatal("Consume(after challenge expiry) error = nil, want expired")
	}
}

func TestChallengeTokenLifetimeStartsAtGrant(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	baseTime := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	prepareStore := ChallengeStore{Paths: paths, Now: func() time.Time { return baseTime }}
	prepared, err := prepareStore.Prepare("research", ChallengePrepare{
		Workspace:            "research",
		Operation:            ChallengeOperationApprove,
		ClaimID:              "clm_0123456789abcdef0123456789abcdef",
		CanonicalDraftDigest: "sha256:" + strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	grantTime := baseTime.Add(4 * time.Minute)
	grantStore := ChallengeStore{Paths: paths, Now: func() time.Time { return grantTime }}
	granted, err := grantStore.Grant("research", prepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if want := grantTime.Add(challengeTokenLifetime).Format(time.RFC3339); granted.TokenExpiresAt != want {
		t.Fatalf("TokenExpiresAt = %q, want grant-based %q", granted.TokenExpiresAt, want)
	}
	beforePrepareTTL := ChallengeStore{Paths: paths, Now: func() time.Time {
		return baseTime.Add(6 * time.Minute)
	}}
	if _, err := beforePrepareTTL.Verify("research", prepared.Challenge.ID, granted.Token); err != nil {
		t.Fatalf("Verify(before prepare-based expiry) error = %v", err)
	}
	atGrantTTL := ChallengeStore{Paths: paths, Now: func() time.Time {
		return grantTime.Add(challengeTokenLifetime)
	}}
	if _, err := atGrantTTL.Verify("research", prepared.Challenge.ID, granted.Token); err != nil {
		t.Fatalf("Verify(at grant-based expiry) error = %v", err)
	}
	afterGrantTTL := ChallengeStore{Paths: paths, Now: func() time.Time {
		return grantTime.Add(challengeTokenLifetime).Add(time.Microsecond)
	}}
	if _, err := afterGrantTTL.Verify("research", prepared.Challenge.ID, granted.Token); err == nil || !strings.Contains(err.Error(), "token expired") {
		t.Fatalf("Verify(after grant-based expiry) error = %v, want token expired", err)
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
	if _, err := store.Verify("second", prepared.Challenge.ID, ""); err == nil {
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

func TestChallengeRejectsOldSchemaAndTampering(t *testing.T) {
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
	granted, err := store.Grant("research", prepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	token := granted.Token
	path := filepath.Join(paths.WorkspacesDir, "research", ".zbrain", "challenges", prepared.Challenge.ID+".json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	write := func(t *testing.T, mutate func(*Challenge)) {
		t.Helper()
		var challenge Challenge
		if err := json.Unmarshal(original, &challenge); err != nil {
			t.Fatalf("unmarshal challenge: %v", err)
		}
		mutate(&challenge)
		encoded, err := json.MarshalIndent(challenge, "", "  ")
		if err != nil {
			t.Fatalf("marshal challenge: %v", err)
		}
		if err := os.WriteFile(path, append(encoded, '\n'), runtimeMetadataMode); err != nil {
			t.Fatalf("write tampered challenge: %v", err)
		}
	}
	restore := func(t *testing.T) {
		t.Helper()
		if err := os.WriteFile(path, original, runtimeMetadataMode); err != nil {
			t.Fatalf("restore challenge: %v", err)
		}
	}

	t.Run("old schema", func(t *testing.T) {
		write(t, func(challenge *Challenge) { challenge.Schema = "zbrain.challenge/v2" })
		defer restore(t)
		if _, err := store.Read("research", prepared.Challenge.ID); err == nil || !strings.Contains(err.Error(), "schema mismatch") {
			t.Fatalf("Read(old schema) error = %v, want schema mismatch", err)
		}
	})
	t.Run("granted without timestamp", func(t *testing.T) {
		write(t, func(challenge *Challenge) {
			challenge.Granted = true
			challenge.GrantedAt = ""
		})
		defer restore(t)
		if _, err := store.Read("research", prepared.Challenge.ID); err == nil || !strings.Contains(err.Error(), "granted_at is required") {
			t.Fatalf("Read(granted without timestamp) error = %v, want granted_at error", err)
		}
	})
	t.Run("timestamp without grant", func(t *testing.T) {
		write(t, func(challenge *Challenge) {
			challenge.Granted = false
			challenge.GrantedAt = fixedClaimStoreNow().Format(time.RFC3339)
			challenge.TokenSHA256 = ""
			challenge.TokenExpiresAt = ""
		})
		defer restore(t)
		if _, err := store.Read("research", prepared.Challenge.ID); err == nil || !strings.Contains(err.Error(), "granted_at requires granted state") {
			t.Fatalf("Read(timestamp without grant) error = %v, want grant-state error", err)
		}
	})
	t.Run("consumed without grant", func(t *testing.T) {
		write(t, func(challenge *Challenge) {
			challenge.Granted = false
			challenge.GrantedAt = ""
			challenge.TokenSHA256 = ""
			challenge.TokenExpiresAt = ""
			challenge.Consumed = true
		})
		defer restore(t)
		if _, err := store.Read("research", prepared.Challenge.ID); err == nil || !strings.Contains(err.Error(), "consumed without an owner grant") {
			t.Fatalf("Read(consumed without grant) error = %v, want owner-grant error", err)
		}
	})
	t.Run("tampered action", func(t *testing.T) {
		write(t, func(challenge *Challenge) { challenge.Operation = ChallengeOperationRevoke })
		defer restore(t)
		if _, err := store.Read("research", prepared.Challenge.ID); err == nil || !strings.Contains(err.Error(), "action digest mismatch") {
			t.Fatalf("Read(tampered action) error = %v, want action digest mismatch", err)
		}
	})
	t.Run("tampered token hash", func(t *testing.T) {
		write(t, func(challenge *Challenge) { challenge.TokenSHA256 = "sha256:" + strings.Repeat("0", 64) })
		defer restore(t)
		if _, err := store.Verify("research", prepared.Challenge.ID, token); err == nil || !strings.Contains(err.Error(), "token mismatch") {
			t.Fatalf("Verify(tampered hash) error = %v, want token mismatch", err)
		}
	})
	t.Run("malformed token expiry", func(t *testing.T) {
		write(t, func(challenge *Challenge) { challenge.TokenExpiresAt = "not-a-time" })
		defer restore(t)
		if _, err := store.Read("research", prepared.Challenge.ID); err == nil || !strings.Contains(err.Error(), "token_expires_at") {
			t.Fatalf("Read(malformed expiry) error = %v, want token_expires_at error", err)
		}
	})
	t.Run("token expiry outlives challenge", func(t *testing.T) {
		write(t, func(challenge *Challenge) {
			challenge.TokenExpiresAt = challenge.ExpiresAt
			challenge.ExpiresAt = fixedClaimStoreNow().Add(time.Minute).Format(time.RFC3339)
		})
		defer restore(t)
		if _, err := store.Read("research", prepared.Challenge.ID); err == nil || !strings.Contains(err.Error(), "must not outlive") {
			t.Fatalf("Read(expiry ordering) error = %v, want expiry ordering error", err)
		}
	})
}

func TestChallengeConsumeConcurrentOneWinner(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	prepared, err := store.Prepare("research", ChallengePrepare{
		Workspace:            "research",
		Operation:            ChallengeOperationApprove,
		ClaimID:              "clm_0123456789abcdef0123456789abcdef",
		CanonicalDraftDigest: "sha256:" + strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	granted, err := store.Grant("research", prepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	token := granted.Token
	const attempts = 8
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			_, err := store.Consume("research", prepared.Challenge.ID, token)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	winners := 0
	for err := range errs {
		if err == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent Consume winners = %d, want exactly one", winners)
	}
}
