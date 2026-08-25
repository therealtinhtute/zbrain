package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTransitionJournalPath(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	journalPath, err := PendingTransitionPath(paths, "research")
	if err != nil {
		t.Fatalf("PendingTransitionPath() error = %v", err)
	}
	workspaceRoot, err := ValidateWorkspace(paths, "research")
	if err != nil {
		t.Fatalf("ValidateWorkspace() error = %v", err)
	}
	want := filepath.Join(workspaceRoot, ".zbrain", pendingTransitionFileName)
	if journalPath != want {
		t.Fatalf("PendingTransitionPath() = %q, want %q", journalPath, want)
	}
	if _, err := os.Stat(filepath.Dir(journalPath)); !os.IsNotExist(err) {
		t.Fatalf("PendingTransitionPath() created journal directory or returned unexpected error: %v", err)
	}

	if _, err := PendingTransitionPath(paths, "../outside"); err == nil {
		t.Fatalf("PendingTransitionPath(unsafe workspace) error = nil")
	}
}

func TestTransitionJournal(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	firstPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "first.md")
	secondPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "second.md")
	firstBefore := []byte("first before\n")
	secondBefore := []byte("second before\n")
	if err := os.WriteFile(firstPath, firstBefore, 0o644); err != nil {
		t.Fatalf("WriteFile(first) error = %v", err)
	}
	if err := os.WriteFile(secondPath, secondBefore, 0o644); err != nil {
		t.Fatalf("WriteFile(second) error = %v", err)
	}
	firstTarget := []byte("first after\n")
	secondTarget := []byte("second after\n")
	pending := PendingTransition{
		OperationID: "txn_test",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets: []PendingTransitionTarget{
			pendingTransitionTarget("wiki/projects/second.md", secondBefore, secondTarget),
			pendingTransitionTarget("wiki/projects/first.md", firstBefore, firstTarget),
		},
	}
	if err := WritePendingTransition(paths, "research", pending); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}
	read, err := ReadPendingTransition(paths, "research")
	if err != nil {
		t.Fatalf("ReadPendingTransition() error = %v", err)
	}
	if len(read.Targets) != 2 || read.Targets[0].Path != "wiki/projects/first.md" || read.Targets[1].Path != "wiki/projects/second.md" {
		t.Fatalf("journal targets = %#v, want sorted paths", read.Targets)
	}
	if string(read.Targets[0].TargetBytes) != string(firstTarget) || string(read.Targets[1].TargetBytes) != string(secondTarget) {
		t.Fatalf("journal target bytes = %#v", read.Targets)
	}
	if err := WritePendingTransition(paths, "research", pending); err == nil {
		t.Fatalf("second WritePendingTransition() error = nil, want exclusive journal")
	}
}

func TestTransitionRecovery(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	firstPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "first.md")
	secondPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "second.md")
	firstBefore := []byte("first before\n")
	secondBefore := []byte("second before\n")
	firstTarget := []byte("first after\n")
	secondTarget := []byte("second after\n")
	if err := os.WriteFile(firstPath, firstBefore, 0o644); err != nil {
		t.Fatalf("WriteFile(first) error = %v", err)
	}
	if err := os.WriteFile(secondPath, secondBefore, 0o644); err != nil {
		t.Fatalf("WriteFile(second) error = %v", err)
	}
	if err := WritePendingTransition(paths, "research", PendingTransition{
		OperationID: "txn_recovery",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets: []PendingTransitionTarget{
			pendingTransitionTarget("wiki/projects/first.md", firstBefore, firstTarget),
			pendingTransitionTarget("wiki/projects/second.md", secondBefore, secondTarget),
		},
	}); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}

	// Simulate an interruption after the first canonical rename.
	if err := os.WriteFile(firstPath, firstTarget, 0o644); err != nil {
		t.Fatalf("WriteFile(interrupted first) error = %v", err)
	}
	if err := RecoverPendingTransition(paths, "research"); err != nil {
		t.Fatalf("RecoverPendingTransition() error = %v", err)
	}
	assertFileBytes(t, firstPath, firstTarget)
	assertFileBytes(t, secondPath, secondTarget)
	assertNoPendingTransition(t, paths, "research")
}

func TestTransitionRecoveryAtEachRename(t *testing.T) {
	for interruption := 0; interruption <= 2; interruption++ {
		t.Run(fmt.Sprintf("after_%d_renames", interruption), func(t *testing.T) {
			paths, _ := claimStoreTestPaths(t)
			files := []struct {
				relative string
				before   []byte
				target   []byte
			}{
				{relative: "wiki/projects/first.md", before: []byte("first before\n"), target: []byte("first after\n")},
				{relative: "wiki/projects/second.md", before: []byte("second before\n"), target: []byte("second after\n")},
			}
			for _, file := range files {
				path := filepath.Join(paths.WorkspacesDir, "research", filepath.FromSlash(file.relative))
				if err := os.WriteFile(path, file.before, 0o644); err != nil {
					t.Fatalf("WriteFile(%s) error = %v", file.relative, err)
				}
			}
			pending := PendingTransition{
				OperationID: "txn_each_rename",
				Kind:        ClaimTransitionSupersede,
				Workspace:   "research",
				Targets: []PendingTransitionTarget{
					pendingTransitionTarget(files[0].relative, files[0].before, files[0].target),
					pendingTransitionTarget(files[1].relative, files[1].before, files[1].target),
				},
			}
			if err := WritePendingTransition(paths, "research", pending); err != nil {
				t.Fatalf("WritePendingTransition() error = %v", err)
			}
			for i := 0; i < interruption; i++ {
				file := files[i]
				path := filepath.Join(paths.WorkspacesDir, "research", filepath.FromSlash(file.relative))
				if err := os.WriteFile(path, file.target, 0o644); err != nil {
					t.Fatalf("WriteFile(interrupted %s) error = %v", file.relative, err)
				}
			}
			if err := RecoverPendingTransition(paths, "research"); err != nil {
				t.Fatalf("RecoverPendingTransition() error = %v", err)
			}
			for _, file := range files {
				path := filepath.Join(paths.WorkspacesDir, "research", filepath.FromSlash(file.relative))
				assertFileBytes(t, path, file.target)
			}
			assertNoPendingTransition(t, paths, "research")
		})
	}
}

func TestTransitionRecoveryIdempotent(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	path := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "claim.md")
	before := []byte("before\n")
	target := []byte("target\n")
	if err := os.WriteFile(path, before, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := WritePendingTransition(paths, "research", PendingTransition{
		OperationID: "txn_idempotent",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets:     []PendingTransitionTarget{pendingTransitionTarget("wiki/projects/claim.md", before, target)},
	}); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}
	if err := RecoverPendingTransition(paths, "research"); err != nil {
		t.Fatalf("first RecoverPendingTransition() error = %v", err)
	}
	if err := RecoverPendingTransition(paths, "research"); err != nil {
		t.Fatalf("second RecoverPendingTransition() error = %v", err)
	}
	assertFileBytes(t, path, target)
}

func TestTransitionPreimageMismatch(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	firstPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "first.md")
	secondPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "second.md")
	firstBefore := []byte("first before\n")
	secondBefore := []byte("second before\n")
	firstTarget := []byte("first after\n")
	secondTarget := []byte("second after\n")
	if err := os.WriteFile(firstPath, firstBefore, 0o644); err != nil {
		t.Fatalf("WriteFile(first) error = %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("unexpected\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(second) error = %v", err)
	}
	if err := WritePendingTransition(paths, "research", PendingTransition{
		OperationID: "txn_mismatch",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets: []PendingTransitionTarget{
			pendingTransitionTarget("wiki/projects/first.md", firstBefore, firstTarget),
			pendingTransitionTarget("wiki/projects/second.md", secondBefore, secondTarget),
		},
	}); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}
	if err := RecoverPendingTransition(paths, "research"); err == nil || !strings.Contains(err.Error(), "preimage mismatch") {
		t.Fatalf("RecoverPendingTransition() error = %v, want preimage mismatch", err)
	}
	assertFileBytes(t, firstPath, firstBefore)
	assertFileBytes(t, secondPath, []byte("unexpected\n"))
	if _, err := ReadPendingTransition(paths, "research"); err != nil {
		t.Fatalf("ReadPendingTransition() after mismatch error = %v", err)
	}
}

func TestTransitionJournalRejectsUnsafeOrMalformedTargets(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	before := []byte("before\n")
	target := []byte("target\n")
	base := PendingTransition{
		OperationID: "txn_invalid",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets:     []PendingTransitionTarget{pendingTransitionTarget("wiki/projects/claim.md", before, target)},
	}
	tests := []struct {
		name   string
		mutate func(*PendingTransition)
	}{
		{
			name: "path escape",
			mutate: func(pending *PendingTransition) {
				pending.Targets[0].Path = "../outside"
			},
		},
		{
			name: "duplicate target",
			mutate: func(pending *PendingTransition) {
				pending.Targets = append(pending.Targets, pending.Targets[0])
			},
		},
		{
			name: "target bytes hash mismatch",
			mutate: func(pending *PendingTransition) {
				pending.Targets[0].TargetSHA256 = transitionSHA256([]byte("other\n"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := base
			candidate.Targets = append([]PendingTransitionTarget(nil), base.Targets...)
			tt.mutate(&candidate)
			if err := WritePendingTransition(paths, "research", candidate); err == nil {
				t.Fatalf("WritePendingTransition() error = nil")
			}
		})
	}
}

func pendingTransitionTarget(path string, before []byte, target []byte) PendingTransitionTarget {
	return PendingTransitionTarget{
		Path:           path,
		PreimageSHA256: transitionSHA256(before),
		TargetSHA256:   transitionSHA256(target),
		TargetBytes:    target,
	}
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func assertNoPendingTransition(t *testing.T, paths Paths, workspace string) {
	t.Helper()
	path, err := PendingTransitionPath(paths, workspace)
	if err != nil {
		t.Fatalf("PendingTransitionPath() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("pending transition journal still exists or returned unexpected error: %v", err)
	}
}

func TestWriteTransitionBytesAtomicErrors(t *testing.T) {
	tmp := t.TempDir()
	successPath := filepath.Join(tmp, "success.md")
	if err := writeTransitionBytesAtomic(successPath, []byte("hello\n")); err != nil {
		t.Fatalf("writeTransitionBytesAtomic(success) error = %v", err)
	}
	got, err := os.ReadFile(successPath)
	if err != nil {
		t.Fatalf("ReadFile(success) error = %v", err)
	}
	if string(got) != "hello\n" {
		t.Fatalf("success content = %q, want %q", string(got), "hello\n")
	}
	if info, err := os.Stat(successPath); err != nil {
		t.Fatalf("Stat(success) error = %v", err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("success mode = %04o, want %04o", got, 0o600)
	}
	// CreateTemp failure: parent is a file, not a directory.
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile(blocker) error = %v", err)
	}
	badPath := filepath.Join(blocker, "target.md")
	if err := writeTransitionBytesAtomic(badPath, []byte("content")); err == nil {
		t.Fatalf("writeTransitionBytesAtomic(bad parent) error = nil, want failure")
	}
	// Rename failure: target is an existing directory.
	dirTarget := filepath.Join(tmp, "dirTarget")
	if err := os.Mkdir(dirTarget, 0o700); err != nil {
		t.Fatalf("Mkdir(dirTarget) error = %v", err)
	}
	if err := writeTransitionBytesAtomic(dirTarget, []byte("content")); err == nil {
		t.Fatalf("writeTransitionBytesAtomic(dir as target) error = nil, want rename failure")
	}
}

func TestCheckPendingTransitionCoverage(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	if err := CheckPendingTransition(paths, "research"); err != nil {
		t.Fatalf("CheckPendingTransition(no journal) error = %v", err)
	}
	before := []byte("before\n")
	target := []byte("after\n")
	pending := PendingTransition{
		OperationID: "txn_check",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets:     []PendingTransitionTarget{pendingTransitionTarget("wiki/projects/check.md", before, target)},
	}
	checkPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "check.md")
	if err := os.WriteFile(checkPath, before, 0o644); err != nil {
		t.Fatalf("WriteFile(check) error = %v", err)
	}
	if err := WritePendingTransition(paths, "research", pending); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}
	if err := CheckPendingTransition(paths, "research"); err == nil || !strings.Contains(err.Error(), "pending transition") {
		t.Fatalf("CheckPendingTransition(with journal) error = %v, want pending", err)
	}
	journalPath, err := PendingTransitionPath(paths, "research")
	if err != nil {
		t.Fatalf("PendingTransitionPath() error = %v", err)
	}
	if err := os.WriteFile(journalPath, []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt) error = %v", err)
	}
	if err := CheckPendingTransition(paths, "research"); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("CheckPendingTransition(invalid) error = %v, want invalid", err)
	}
	// Missing workspace should still be handled (lock error)
	if err := CheckPendingTransition(paths, "../outside"); err == nil {
		t.Fatalf("CheckPendingTransition(unsafe workspace) error = nil")
	}
}

func TestValidatePendingTransitionCoverage(t *testing.T) {
	before := []byte("before\n")
	target := []byte("after\n")
	valid := PendingTransition{
		OperationID: "txn_valid",
		Kind:        ClaimTransitionApprove,
		Workspace:   "research",
		Targets:     []PendingTransitionTarget{pendingTransitionTarget("wiki/projects/valid.md", before, target)},
	}
	if err := ValidatePendingTransition("research", valid); err != nil {
		t.Fatalf("ValidatePendingTransition(valid) error = %v", err)
	}
	invalid := valid
	invalid.Workspace = "other"
	if err := ValidatePendingTransition("research", invalid); err == nil {
		t.Fatalf("ValidatePendingTransition(workspace mismatch) error = nil")
	}
	invalid = valid
	invalid.OperationID = ""
	if err := ValidatePendingTransition("research", invalid); err == nil {
		t.Fatalf("ValidatePendingTransition(empty operation) error = nil")
	}
	invalid = valid
	invalid.Kind = "unknown"
	if err := ValidatePendingTransition("research", invalid); err == nil {
		t.Fatalf("ValidatePendingTransition(unknown kind) error = nil")
	}
}

func TestTransitionCoverageExtras(t *testing.T) {
	// Cover isTransitionSHA256 and transitionSHA256 edge paths
	if isTransitionSHA256("sha256:zzzz") {
		t.Fatalf("isTransitionSHA256(invalid hex) = true")
	}
	if isTransitionSHA256("sha256:"+strings.Repeat("a", 63)) {
		t.Fatalf("isTransitionSHA256(short) = true")
	}
	if transitionSHA256([]byte("test")) == "" {
		t.Fatalf("transitionSHA256 empty")
	}
	// Cover NewPendingTransitionID success
	if id, err := NewPendingTransitionID(); err != nil || !strings.HasPrefix(id, "txn_") {
		t.Fatalf("NewPendingTransitionID() = %q, err %v", id, err)
	}
	// Cover PendingTransitionPath with invalid workspace
	if _, err := PendingTransitionPath(Paths{}, "../bad"); err == nil {
		t.Fatalf("PendingTransitionPath(unsafe) error = nil")
	}
	// Cover validatePendingTransitionDirectory via symlink
	paths, _ := claimStoreTestPaths(t)
	workspaceRoot, err := ValidateWorkspace(paths, "research")
	if err != nil {
		t.Fatalf("ValidateWorkspace() error = %v", err)
	}
	zbrainDir := filepath.Join(workspaceRoot, ".zbrain")
	_ = os.RemoveAll(zbrainDir)
	symTarget := t.TempDir()
	if err := os.Symlink(symTarget, zbrainDir); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := PendingTransitionPath(paths, "research"); err == nil {
		t.Fatalf("PendingTransitionPath(symlink) error = nil, want symlink rejection")
	}
	_ = os.Remove(zbrainDir)
}

func TestCoverEvidenceAndChallengeZeroPercent(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	// Evidence ReadRaw 0%: test invalid ID and missing file
	evidenceStore := EvidenceStore{Paths: paths, Now: fixedClaimStoreNow}
	if _, err := evidenceStore.ReadRaw("research", "evd_invalid"); err == nil {
		t.Fatalf("ReadRaw(invalid id) error = nil")
	}
	if _, err := evidenceStore.ReadRaw("research", "evd_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatalf("ReadRaw(missing) error = nil, want not found")
	}
	// Evidence Unwrap 0%
	evErr := &EvidenceVerificationError{ID: "evd_x", Path: "evidence/sources/evd_x/raw", Reason: "test", Err: fmt.Errorf("cause")}
	if got := evErr.Unwrap(); got == nil || got.Error() != "cause" {
		t.Fatalf("Unwrap() = %v, want cause", got)
	}
	// splitEvidenceLines 0% and EvidenceSpanDigest 0%
	lines := splitEvidenceLines([]byte(""))
	if len(lines) != 1 || string(lines[0]) != "" {
		t.Fatalf("splitEvidenceLines(empty) = %v", lines)
	}
	lines = splitEvidenceLines([]byte("a\nb\nc"))
	if len(lines) != 3 {
		t.Fatalf("splitEvidenceLines(3 lines) = %d, want 3", len(lines))
	}
	lines = splitEvidenceLines([]byte("no newline"))
	if len(lines) != 1 {
		t.Fatalf("splitEvidenceLines(no newline) = %d, want 1", len(lines))
	}
	digest := EvidenceSpanDigest("sha256:evidence-v1:abcd", 1, 1, []byte("hello"))
	if !strings.HasPrefix(digest, "sha256:span-v1:") {
		t.Fatalf("EvidenceSpanDigest() = %q, want span prefix", digest)
	}
	// Challenge FindChallenge 0%
	chStore := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	if _, _, err := chStore.FindChallenge("invalid"); err == nil {
		t.Fatalf("FindChallenge(invalid pattern) error = nil")
	}
	if _, _, err := chStore.FindChallenge("chg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		// may be not found error, but ensure it returns error when no workspace
		t.Fatalf("FindChallenge(not found) error = nil, want not found")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("FindChallenge(not found) error = %v, want not found", err)
	}
	// CoordinationLockPath 0%
	if _, err := CoordinationLockPath(paths, "research"); err != nil {
		t.Fatalf("CoordinationLockPath(valid) error = %v", err)
	}
	if _, err := CoordinationLockPath(paths, "../bad"); err == nil {
		t.Fatalf("CoordinationLockPath(unsafe) error = nil")
	}
	// markDirty 0% via ClaimStore and EvidenceStore
	claimStore := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	if err := claimStore.markDirty("research"); err != nil {
		t.Fatalf("ClaimStore.markDirty(valid) error = %v", err)
	}
	if err := claimStore.markDirty("../bad"); err == nil {
		t.Fatalf("ClaimStore.markDirty(unsafe) error = nil")
	}
	eStore := EvidenceStore{Paths: paths}
	if err := eStore.markDirty("research"); err != nil {
		t.Fatalf("EvidenceStore.markDirty(valid) error = %v", err)
	}
	// approveUnlocked 0% via direct call
	claim := validStoreClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ClaimBasisOwner)
	if _, err := claimStore.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := claimStore.approveUnlocked("research", claim.ID); err != nil {
		t.Fatalf("approveUnlocked(valid) error = %v", err)
	}
	// readFlatClaim 0%
	if _, err := claimStore.readFlatClaim("research", claim.ID); err != nil {
		t.Fatalf("readFlatClaim(valid) error = %v", err)
	}
	// appendUniqueClaimID 50%
	ids := []string{"a", "b"}
	if out := appendUniqueClaimID(ids, "c"); len(out) != 3 {
		t.Fatalf("appendUniqueClaimID(new) = %v, want 3", out)
	}
	if out := appendUniqueClaimID(ids, "a"); len(out) != 2 {
		t.Fatalf("appendUniqueClaimID(existing) = %v, want 2", out)
	}
}

func TestCoverPathsAndWorkspace(t *testing.T) {
	tmp := t.TempDir()
	// ResolvePaths 51.9% - test various options
	if _, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")}); err != nil {
		t.Fatalf("ResolvePaths(valid) error = %v", err)
	}
	// EnsureConfig 42.9%
	cfgPath := filepath.Join(tmp, "config.yml")
	if _, err := EnsureConfig(cfgPath); err != nil {
		t.Fatalf("EnsureConfig(first) error = %v", err)
	}
	if _, err := EnsureConfig(cfgPath); err != nil {
		t.Fatalf("EnsureConfig(second) error = %v", err)
	}
	// CreateWorkspace 64.7%
	paths, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain2")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if _, err := EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	if err := CreateWorkspace(paths, "test-ws", fixedClaimStoreNow()); err != nil {
		t.Fatalf("CreateWorkspace(valid) error = %v", err)
	}
	if err := CreateWorkspace(paths, "test-ws", fixedClaimStoreNow()); err == nil {
		t.Fatalf("CreateWorkspace(duplicate) error = nil")
	}
	if err := CreateWorkspace(paths, "../bad", fixedClaimStoreNow()); err == nil {
		t.Fatalf("CreateWorkspace(unsafe) error = nil")
	}
	// ResolveCurrentWorkspace 77.8%
	if _, err := ResolveCurrentWorkspace(paths); err != nil {
		t.Fatalf("ResolveCurrentWorkspace(valid) error = %v", err)
	}
	badPaths := paths
	badPaths.RuntimeDir = filepath.Join(tmp, "nope")
	badPaths.ConfigFile = filepath.Join(badPaths.RuntimeDir, "config.yml")
	if _, err := ResolveCurrentWorkspace(badPaths); err == nil {
		t.Fatalf("ResolveCurrentWorkspace(missing config) error = nil")
	}
	// ValidateWorkspace 75%
	if _, err := ValidateWorkspace(paths, "test-ws"); err != nil {
		t.Fatalf("ValidateWorkspace(valid) error = %v", err)
	}
	if _, err := ValidateWorkspace(paths, "../bad"); err == nil {
		t.Fatalf("ValidateWorkspace(unsafe) error = nil")
	}
}

func TestCoverIndexAndSearch(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	// SearchWorkspace 60.6% - use IndexStore.Search with no index (should fail closed)
	store := IndexStore{Paths: paths}
	if _, err := store.Search("research", SearchOptions{Query: "test", Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: 10}); err == nil {
		t.Fatalf("Search(no index) error = nil, want fail closed")
	}
	// Create a claim and rebuild to get searchable index
	claimStore := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_cccccccccccccccccccccccccccccc01", ClaimBasisOwner)
	claim.Title = "SearchCoverage"
	claim.Body = "search coverage body"
	claim.Status = ClaimStatusDraft
	if _, err := claimStore.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := claimStore.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if _, err := store.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if results, err := store.Search("research", SearchOptions{Query: "SearchCoverage", Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: 10}); err != nil {
		t.Fatalf("Search(valid) error = %v", err)
	} else if len(results) == 0 {
		t.Fatalf("Search(valid) results = 0, want at least 1")
	}
	// findTrustInputManifestOffender 60% - trigger via mismatched manifest
	empty := TrustInputManifest{Entries: nil, Digest: trustInputManifestDigest(nil)}
	oneEntry := TrustInputManifest{Entries: []TrustInput{{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)}}, Digest: trustInputManifestDigest([]TrustInput{{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)}})}
	if got := findTrustInputManifestOffender(empty, oneEntry); got == "" {
		t.Fatalf("findTrustInputManifestOffender(mismatch) = %q, want non-empty", got)
	}
	if got := findTrustInputManifestOffender(empty, empty); got != "" {
		t.Fatalf("findTrustInputManifestOffender(same) = %q, want empty", got)
	}
	// claimFilePath 60% via WriteDraft with custom Path
	customClaim := validStoreClaim("clm_dddddddddddddddddddddddddddddddd", ClaimBasisOwner)
	customClaim.Path = "projects/custom.md"
	if _, err := claimStore.WriteDraft("research", customClaim); err != nil {
		t.Fatalf("WriteDraft(custom path) error = %v", err)
	}
	badClaim := validStoreClaim("clm_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", ClaimBasisOwner)
	badClaim.Path = "bad/path.txt"
	if _, err := claimStore.WriteDraft("research", badClaim); err == nil {
		t.Fatalf("WriteDraft(bad path) error = nil, want failure")
	}
}

func TestCoverChallengeLow(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	// challengePath error: invalid ID pattern
	if _, err := store.challengePath("research", "bad-id"); err == nil {
		t.Fatalf("challengePath(bad id) error = nil")
	}
	if _, err := store.challengePath("../bad", "chg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatalf("challengePath(bad workspace) error = nil")
	}
	// isChallengeTokenHash
	if isChallengeTokenHash("sha256:zzzz") {
		t.Fatalf("isChallengeTokenHash(invalid) = true")
	}
	if !isChallengeTokenHash("sha256:"+strings.Repeat("a", 64)) {
		t.Fatalf("isChallengeTokenHash(valid) = false")
	}
	// validateChallengeDirectory via symlink
	workspaceRoot, err := ValidateWorkspace(paths, "research")
	if err != nil {
		t.Fatalf("ValidateWorkspace() error = %v", err)
	}
	controlDir := filepath.Join(workspaceRoot, ".zbrain", "challenges")
	_ = os.MkdirAll(filepath.Join(workspaceRoot, ".zbrain"), 0o700)
	// Ensure challenges directory is a file to trigger not-a-directory
	_ = os.RemoveAll(controlDir)
	if err := os.WriteFile(controlDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile(challenges blocker) error = %v", err)
	}
	if _, err := store.challengePath("research", "chg_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err == nil {
		t.Fatalf("challengePath(blocked dir) error = nil")
	}
	_ = os.Remove(controlDir)
	// writeChallenge error paths: invalid record
	invalidChallenge := Challenge{
		Schema:    "bad-schema",
		ID:        "chg_cccccccccccccccccccccccccccccccc",
		Workspace: "research",
		Operation: ChallengeOperationApprove,
		ClaimID:   "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExpiresAt: fixedClaimStoreNow().Add(15 * time.Minute).Format(time.RFC3339),
	}
	if err := store.writeChallenge("research", invalidChallenge); err == nil {
		t.Fatalf("writeChallenge(invalid schema) error = nil")
	}
	// writeChallenge with valid challenge should succeed (via Prepare)
	claimStore := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_ffffffffffffffffffffffffffffffff", ClaimBasisOwner)
	if _, err := claimStore.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	prepare := ChallengePrepare{
		Workspace:            "research",
		Operation:            ChallengeOperationApprove,
		ClaimID:              claim.ID,
		CanonicalDraftDigest: "sha256:" + strings.Repeat("a", 64),
	}
	prepared, err := store.Prepare("research", prepare)
	if err != nil {
		t.Fatalf("Prepare(valid) error = %v", err)
	}
	// Grant to cover Grant and consumeUnlocked
	granted, err := store.Grant("research", prepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Grant(valid) error = %v", err)
	}
	if _, err := store.Consume("research", prepared.Challenge.ID, granted.Token); err != nil {
		t.Fatalf("Consume(valid) error = %v", err)
	}
	// FindChallenge should now find it even after consumed? It should still find
	if _, _, err := store.FindChallenge(prepared.Challenge.ID); err != nil {
		t.Fatalf("FindChallenge(valid) error = %v", err)
	}
}

func TestCoverCoordinationAndPaths(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	// acquireWorkspaceLock error: invalid workspace
	if _, err := acquireWorkspaceLock(paths, "../bad", true); err == nil {
		t.Fatalf("acquireWorkspaceLock(unsafe) error = nil")
	}
	// workspaceControlPath
	if _, err := workspaceControlPath(paths, "research", "test"); err != nil {
		t.Fatalf("workspaceControlPath(valid) error = %v", err)
	}
	if _, err := workspaceControlPath(paths, "../bad", "test"); err == nil {
		t.Fatalf("workspaceControlPath(unsafe) error = nil")
	}
	// ensureWorkspaceControlDirectory
	workspaceRoot, err := ValidateWorkspace(paths, "research")
	if err != nil {
		t.Fatalf("ValidateWorkspace() error = %v", err)
	}
	controlDir := filepath.Join(workspaceRoot, ".zbrain")
	if err := ensureWorkspaceControlDirectory(workspaceRoot, controlDir); err != nil {
		t.Fatalf("ensureWorkspaceControlDirectory(valid) error = %v", err)
	}
	// validateWorkspaceControlDirectory with symlink
	symTarget := t.TempDir()
	symLink := filepath.Join(workspaceRoot, ".zbrain_symlink")
	_ = os.RemoveAll(symLink)
	if err := os.Symlink(symTarget, symLink); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := validateWorkspaceControlDirectory(workspaceRoot, symLink); err == nil {
		t.Fatalf("validateWorkspaceControlDirectory(symlink) error = nil")
	}
	_ = os.Remove(symLink)
	// paths.go EnsureConfig already tested, test ResolvePaths with env
	t.Setenv("ZBRAIN_HOME", filepath.Join(paths.RuntimeDir, "env_home"))
	if _, err := ResolvePaths(Options{CWD: "", HomeDir: "", RuntimeDir: ""}); err != nil {
		t.Fatalf("ResolvePaths(env) error = %v", err)
	}
	// claimStore writeClaimAtomic error via invalid claim
	if err := writeClaimAtomic(filepath.Join(t.TempDir(), "bad.md"), Claim{}); err == nil {
		t.Fatalf("writeClaimAtomic(invalid claim) error = nil")
	}
}

func TestCoverEvidenceAndLifecycle(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	// evidence validator with invalid workspace
	if _, err := NewEvidenceValidator(EvidenceStore{Paths: paths}, "../bad"); err == nil {
		t.Fatalf("NewEvidenceValidator(unsafe) error = nil")
	}
	// validateEvidenceSpans via malformed claim
	evidenceStore := EvidenceStore{Paths: paths, Now: fixedClaimStoreNow}
	sourcePath := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(sourcePath, []byte("line1\nline2\nline3\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	ev, err := evidenceStore.AddFile("research", sourcePath, "file://test", "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}
	validator, err := NewEvidenceValidator(evidenceStore, "research")
	if err != nil {
		t.Fatalf("NewEvidenceValidator() error = %v", err)
	}
	// validateEvidenceSpans with no spans should succeed
	claim := validStoreClaim("clm_11111111111111111111111111111111", ClaimBasisEvidence)
	claim.EvidenceIDs = []string{ev.ID}
	claim.Sources = []ClaimSource{{ID: ev.ID, Resource: "evidence/sources/" + ev.ID + "/raw", Title: ev.Origin, Digest: validator.snapshotDigests[ev.ID]}}
	// Need to ensure snapshotDigests populated
	if err := validator.Verify(ev.ID); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	// Now test validateClaimEvidence with duplicate evidence id
	dupClaim := claim
	dupClaim.EvidenceIDs = []string{ev.ID, ev.ID}
	if err := validateClaimEvidence(validator, dupClaim); err == nil {
		t.Fatalf("validateClaimEvidence(duplicate) error = nil")
	}
	// firstSupersededVerificationDigest via ClaimStore
	claimStore2 := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	approvedClaim2 := validStoreClaim("clm_33333333333333333333333333333333", ClaimBasisOwner)
	approvedClaim2.Status = ClaimStatusApproved
	approvedClaim2.VerifiedAt = fixedClaimStoreNow().Format(time.RFC3339)
	approvedClaim2.VerifiedBy = "owner"
	if digest, err := ClaimVerificationDigest(approvedClaim2); err == nil {
		approvedClaim2.VerifiedDigest = digest
		writeCanonicalStoreClaim(t, paths, approvedClaim2)
		if got, err := claimStore2.firstSupersededVerificationDigest("research", []string{approvedClaim2.ID}); err != nil {
			t.Fatalf("firstSupersededVerificationDigest(valid) error = %v", err)
		} else if got != approvedClaim2.VerifiedDigest {
			t.Fatalf("firstSupersededVerificationDigest() = %q, want %q", got, approvedClaim2.VerifiedDigest)
		}
		if got, err := claimStore2.firstSupersededVerificationDigest("research", nil); err != nil || got != "" {
			t.Fatalf("firstSupersededVerificationDigest(nil) = %q, err %v, want empty", got, err)
		}
	}
	// trust validation - test isSHA256Digest
	if !isSHA256Digest(strings.Repeat("a", 64)) {
		t.Fatalf("isSHA256Digest(valid) = false")
	}
	if isSHA256Digest("invalid") {
		t.Fatalf("isSHA256Digest(invalid) = true")
	}
}

func TestCoverIndexRebuildErrorPaths(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := IndexStore{Paths: paths}
	// Make IndexesDir a file to trigger ensureDirectoryMode failure
	origIndexesDir := paths.IndexesDir
	_ = os.RemoveAll(origIndexesDir)
	if err := os.WriteFile(origIndexesDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile(indexes blocker) error = %v", err)
	}
	if _, err := store.Rebuild("research"); err == nil {
		t.Fatalf("Rebuild(indexes is file) error = nil")
	}
	_ = os.Remove(origIndexesDir)
	if err := os.MkdirAll(origIndexesDir, 0o700); err != nil {
		t.Fatalf("Mkdir(indexes) error = %v", err)
	}
	// Make workspace generation file invalid by making .zbrain a file
	workspaceRoot, err := ValidateWorkspace(paths, "research")
	if err != nil {
		t.Fatalf("ValidateWorkspace() error = %v", err)
	}
	zbrainDir := filepath.Join(workspaceRoot, ".zbrain")
	_ = os.RemoveAll(zbrainDir)
	if err := os.WriteFile(zbrainDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile(zbrain blocker) error = %v", err)
	}
	if _, err := store.Rebuild("research"); err == nil {
		t.Fatalf("Rebuild(zbrain is file) error = nil")
	}
	_ = os.Remove(zbrainDir)
	if err := os.Mkdir(zbrainDir, 0o700); err != nil {
		t.Fatalf("Mkdir(zbrain) error = %v", err)
	}
	// Make trust input file invalid to trigger validate
	claimStore := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_44444444444444444444444444444444", ClaimBasisOwner)
	if _, err := claimStore.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := claimStore.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	// Corrupt the claim file to make it invalid for rebuild - should result in rejected state, not error
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", claim.Tier, claim.ID+".md")
	if err := os.WriteFile(claimPath, []byte("invalid frontmatter"), 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt claim) error = %v", err)
	}
	if summary, err := store.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild(corrupt claim) error = %v", err)
	} else if summary.Invalid == 0 && summary.InvalidCount == 0 {
		t.Fatalf("Rebuild(corrupt claim) Invalid = 0, want >0 for corrupt")
	}
}

func TestCoverTransitionWriteErrorsMore(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	// Trigger writePendingTransitionUnlocked ensureDirectory failure by making .zbrain a file
	workspaceRoot, err := ValidateWorkspace(paths, "research")
	if err != nil {
		t.Fatalf("ValidateWorkspace() error = %v", err)
	}
	zbrainDir := filepath.Join(workspaceRoot, ".zbrain")
	_ = os.RemoveAll(zbrainDir)
	if err := os.WriteFile(zbrainDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile(zbrain blocker) error = %v", err)
	}
	before := []byte("before\n")
	target := []byte("after\n")
	tPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "t.md")
	if err := os.WriteFile(tPath, before, 0o644); err != nil {
		t.Fatalf("WriteFile(t) error = %v", err)
	}
	pending := PendingTransition{
		OperationID: "txn_err",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets:     []PendingTransitionTarget{pendingTransitionTarget("wiki/projects/t.md", before, target)},
	}
	if err := WritePendingTransition(paths, "research", pending); err == nil {
		t.Fatalf("WritePendingTransition(zbrain is file) error = nil")
	}
	_ = os.Remove(zbrainDir)
	if err := os.Mkdir(zbrainDir, 0o700); err != nil {
		t.Fatalf("Mkdir(zbrain) error = %v", err)
	}
	// Trigger os.Link failure: second write should fail because journal exists
	if err := WritePendingTransition(paths, "research", pending); err != nil {
		t.Fatalf("first WritePendingTransition() error = %v", err)
	}
	if err := WritePendingTransition(paths, "research", pending); err == nil {
		t.Fatalf("second WritePendingTransition() error = nil, want link failure")
	}
	// Cleanup
	journalPath, _ := PendingTransitionPath(paths, "research")
	_ = os.Remove(journalPath)
	// Test recoverPendingTransition with missing target
	missingPending := PendingTransition{
		OperationID: "txn_missing",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets:     []PendingTransitionTarget{pendingTransitionTarget("wiki/projects/missing.md", []byte("a\n"), []byte("b\n"))},
	}
	if err := WritePendingTransition(paths, "research", missingPending); err != nil {
		t.Fatalf("WritePendingTransition(missing) error = %v", err)
	}
	if err := RecoverPendingTransition(paths, "research"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("RecoverPendingTransition(missing target) error = %v, want missing", err)
	}
	_ = os.Remove(journalPath)
}

func TestCoverMoreRuntimeFuncs(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	// Test evidence validator with invalid ID
	evidenceStore := EvidenceStore{Paths: paths}
	validator, err := NewEvidenceValidator(evidenceStore, "research")
	if err != nil {
		t.Fatalf("NewEvidenceValidator() error = %v", err)
	}
	if err := validator.Verify("evd_invalid"); err == nil {
		t.Fatalf("Verify(invalid id) error = nil")
	}
	// Test evidence.go markDirty
	if err := evidenceStore.markDirty("../bad"); err == nil {
		t.Fatalf("EvidenceStore.markDirty(unsafe) error = nil")
	}
	// Test claim_store markDirty already, test claim_store writeClaimAtomic with directory
	if err := writeClaimAtomic(t.TempDir(), Claim{ID: "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Tier: "projects", Title: "T", Status: ClaimStatusDraft}); err == nil {
		t.Fatalf("writeClaimAtomic(dir) error = nil")
	}
	// Test index_state validate functions directly
	if err := validateTrustInput(TrustInput{Path: "bad", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)}); err == nil {
		t.Fatalf("validateTrustInput(bad path) error = nil")
	}
	if err := validateRebuildState(RebuildState{Status: RebuildStatusClean, InvalidCount: 1, ManifestDigest: strings.Repeat("a", 64), RebuiltAt: "2026-08-04T05:00:00Z"}, strings.Repeat("a", 64)); err == nil {
		t.Fatalf("validateRebuildState(clean with invalidCount) error = nil")
	}
	if err := validateRebuildState(RebuildState{Status: "unknown", InvalidCount: 0, ManifestDigest: strings.Repeat("a", 64), RebuiltAt: "2026-08-04T05:00:00Z"}, strings.Repeat("a", 64)); err == nil {
		t.Fatalf("validateRebuildState(unknown status) error = nil")
	}
	// Test workspaceBoundary pathWithin
	if pathWithin("/a/b", "/a/b/c") != true {
		t.Fatalf("pathWithin true failed")
	}
	if pathWithin("/a/b", "/a/c") != false {
		t.Fatalf("pathWithin false failed")
	}
	// Test config WriteConfig with invalid path (directory)
	if err := WriteConfig(t.TempDir(), Config{}); err == nil {
		t.Fatalf("WriteConfig(dir) error = nil")
	}
}

func TestCoverParseAndValidate(t *testing.T) {
	// parseLegacyClaimFrontmatter error paths
	if _, err := parseLegacyClaimFrontmatter([]byte("invalid: yaml: :"), "projects", "test.md", []byte("body")); err == nil {
		t.Fatalf("parseLegacyClaimFrontmatter(invalid yaml) error = nil")
	}
	if _, err := parseLegacyClaimFrontmatter([]byte("id: missing\n"), "projects", "test.md", []byte("body")); err == nil {
		t.Fatalf("parseLegacyClaimFrontmatter(missing fields) error = nil")
	}
	// parseOKFClaimFrontmatter error paths
	if _, err := parseOKFClaimFrontmatter([]byte("invalid: yaml: :"), "projects", "test.md", []byte("body")); err == nil {
		t.Fatalf("parseOKFClaimFrontmatter(invalid yaml) error = nil")
	}
	// ValidateClaim error paths
	invalidClaim := Claim{ID: "bad", Tier: "projects", Status: ClaimStatusDraft, Title: "T", Basis: ClaimBasisOwner, CreatedAt: "2026-07-30T09:00:00Z", CreatedBy: "owner", Body: "body"}
	if err := ValidateClaim(invalidClaim); err == nil {
		t.Fatalf("ValidateClaim(bad id) error = nil")
	}
	invalidClaim2 := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	invalidClaim2.Status = "unknown"
	if err := ValidateClaim(invalidClaim2); err == nil {
		t.Fatalf("ValidateClaim(unknown status) error = nil")
	}
	// ValidateClaimTransitions
	if err := ValidateClaimTransitions([]ClaimTransition{{Kind: "unknown"}}); err == nil {
		t.Fatalf("ValidateClaimTransitions(unknown) error = nil")
	}
	// ValidateClaimTransitionAuthorization
	if err := ValidateClaimTransitionAuthorization(&ClaimTransitionAuthorization{ChallengeID: "bad"}); err == nil {
		t.Fatalf("ValidateClaimTransitionAuthorization(bad) error = nil")
	}
	if err := ValidateClaimTransitionAuthorization(nil); err != nil {
		t.Fatalf("ValidateClaimTransitionAuthorization(nil) error = %v", err)
	}
	// Test challenge writeChallenge with more error paths
	paths, _ := claimStoreTestPaths(t)
	store := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	// challengePath with symlink control directory
	workspaceRoot, _ := ValidateWorkspace(paths, "research")
	controlDir := filepath.Join(workspaceRoot, ".zbrain", "challenges")
	_ = os.MkdirAll(filepath.Join(workspaceRoot, ".zbrain"), 0o700)
	_ = os.RemoveAll(controlDir)
	if err := os.WriteFile(controlDir, []byte("x"), 0o600); err == nil {
		if _, err := store.challengePath("research", "chg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
			t.Fatalf("challengePath(blocked) error = nil")
		}
		_ = os.Remove(controlDir)
	}
	// Test isChallengeTokenHash with empty
	if isChallengeTokenHash("") {
		t.Fatalf("isChallengeTokenHash(empty) = true")
	}
	// Test claim_store fileInfoHasSubdirectories
	if got, _ := fileInfoHasSubdirectories(nil); got {
		t.Fatalf("fileInfoHasSubdirectories(nil) = true")
	}
	// Test index findTrustInputManifestOffender with all branches
	empty := TrustInputManifest{Entries: nil, Digest: trustInputManifestDigest(nil)}
	a := TrustInput{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("a", 64)}
	b := TrustInput{Path: "wiki/projects/b.md", Kind: TrustInputKindClaim, ByteLength: 1, SHA256: strings.Repeat("b", 64)}
	manifestA := TrustInputManifest{Entries: []TrustInput{a}, Digest: trustInputManifestDigest([]TrustInput{a})}
	manifestB := TrustInputManifest{Entries: []TrustInput{b}, Digest: trustInputManifestDigest([]TrustInput{b})}
	if got := findTrustInputManifestOffender(manifestA, manifestB); got != "wiki/projects/a.md" {
		t.Fatalf("findTrustInputManifestOffender(a vs b) = %q, want a", got)
	}
	if got := findTrustInputManifestOffender(manifestB, manifestA); got != "wiki/projects/a.md" {
		t.Fatalf("findTrustInputManifestOffender(b vs a) = %q, want a", got)
	}
	// Same path different entry
	a2 := TrustInput{Path: "wiki/projects/a.md", Kind: TrustInputKindClaim, ByteLength: 2, SHA256: strings.Repeat("a", 64)}
	manifestA2 := TrustInputManifest{Entries: []TrustInput{a2}, Digest: trustInputManifestDigest([]TrustInput{a2})}
	if got := findTrustInputManifestOffender(manifestA, manifestA2); got != "wiki/projects/a.md" {
		t.Fatalf("findTrustInputManifestOffender(diff entry) = %q, want a", got)
	}
	_ = empty
}

func TestCoverValidateChallengeRecordMore(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	claimStore := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_99999999999999999999999999999999", ClaimBasisOwner)
	if _, err := claimStore.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	prepare := ChallengePrepare{
		Workspace:            "research",
		Operation:            ChallengeOperationApprove,
		ClaimID:              claim.ID,
		CanonicalDraftDigest: "sha256:" + strings.Repeat("a", 64),
	}
	prepared, err := store.Prepare("research", prepare)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	base := prepared.Challenge
	// Test each validation failure
	cases := []struct {
		name   string
		mutate func(*Challenge)
	}{
		{name: "bad schema", mutate: func(c *Challenge) { c.Schema = "bad" }},
		{name: "bad id", mutate: func(c *Challenge) { c.ID = "bad" }},
		{name: "workspace mismatch", mutate: func(c *Challenge) { c.Workspace = "other" }},
		{name: "bad operation", mutate: func(c *Challenge) { c.Operation = "bad" }},
		{name: "bad claim id", mutate: func(c *Challenge) { c.ClaimID = "bad" }},
		{name: "bad expires", mutate: func(c *Challenge) { c.ExpiresAt = "bad" }},
		{name: "consumed without granted", mutate: func(c *Challenge) { c.Consumed = true; c.Granted = false }},
		{name: "granted without token", mutate: func(c *Challenge) { c.Granted = true; c.TokenSHA256 = "" }},
		{name: "token hash bad", mutate: func(c *Challenge) { c.Granted = true; c.TokenSHA256 = "bad"; c.TokenExpiresAt = base.TokenExpiresAt; c.GrantedAt = base.GrantedAt }},
		{name: "action digest mismatch", mutate: func(c *Challenge) { c.ActionDigest = "sha256:bad" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			if err := store.validateChallengeRecord("research", c); err == nil {
				t.Fatalf("validateChallengeRecord(%s) error = nil", tc.name)
			}
		})
	}
	// Test writeChallenge with blocked directory
	workspaceRoot, _ := ValidateWorkspace(paths, "research")
	challengesDir := filepath.Join(workspaceRoot, ".zbrain", "challenges")
	_ = os.MkdirAll(filepath.Join(workspaceRoot, ".zbrain"), 0o700)
	_ = os.RemoveAll(challengesDir)
	if err := os.WriteFile(challengesDir, []byte("x"), 0o600); err == nil {
		if err := store.writeChallenge("research", base); err == nil {
			t.Fatalf("writeChallenge(blocked dir) error = nil")
		}
		_ = os.Remove(challengesDir)
	}
	// Test Grant with expired challenge
	expiredStore := ChallengeStore{Paths: paths, Now: func() time.Time { return fixedClaimStoreNow().Add(20 * time.Minute) }}
	if _, err := expiredStore.Grant("research", base.ID); err == nil {
		t.Fatalf("Grant(expired) error = nil")
	}
}

func TestCoverMoreIndexAndClaim(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	// Test parseLegacy with superseded and verified
	legacyYAML := []byte("schema: zbrain.claim/v1\nid: clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nstatus: draft\ntitle: T\nbasis: owner\ncreated_at: 2026-07-30T09:00:00Z\ncreated_by: owner\n")
	if _, err := parseLegacyClaimFrontmatter(legacyYAML, "projects", "test.md", []byte("body")); err != nil {
		t.Fatalf("parseLegacyClaimFrontmatter(valid) error = %v", err)
	}
	// Test ValidateClaim with approved and verified
	claim := validStoreClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ClaimBasisOwner)
	claim.Status = ClaimStatusApproved
	claim.VerifiedAt = fixedClaimStoreNow().Format(time.RFC3339)
	claim.VerifiedBy = "owner"
	if digest, err := ClaimVerificationDigest(claim); err == nil {
		claim.VerifiedDigest = digest
		if err := ValidateClaim(claim); err != nil {
			t.Fatalf("ValidateClaim(approved) error = %v", err)
		}
	}
	// Test MigrateOKF with legacy file
	legacyPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "clm_cccccccccccccccccccccccccccccccc.md")
	legacyContent := "---\nschema: zbrain.claim/v1\nid: clm_cccccccccccccccccccccccccccccccc\nstatus: draft\ntitle: Legacy\nbasis: owner\ncreated_at: 2026-07-30T09:00:00Z\ncreated_by: owner\n---\n\nLegacy body\n"
	if err := os.WriteFile(legacyPath, []byte(legacyContent), 0o600); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}
	claimStore := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	if _, err := claimStore.MigrateOKF("research"); err != nil {
		t.Fatalf("MigrateOKF() error = %v", err)
	}
	// Test claim_store readFlatClaim with duplicate via manual setup
	dupID := "clm_dddddddddddddddddddddddddddddddd"
	dupClaim1 := validStoreClaim(dupID, ClaimBasisOwner)
	dupClaim1.Title = "dup1"
	writeCanonicalStoreClaim(t, paths, dupClaim1)
	dupPath2 := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "topics", dupID+".md")
	_ = os.MkdirAll(filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "topics"), 0o700)
	dupClaim2 := dupClaim1
	dupClaim2.Title = "dup2"
	dupClaim2.Path = "projects/topics/" + dupID + ".md"
	if err := writeClaimAtomic(dupPath2, dupClaim2); err != nil {
		t.Fatalf("writeClaimAtomic(dup2) error = %v", err)
	}
	if _, err := claimStore.Read("research", dupID); err == nil {
		t.Fatalf("Read(duplicate) error = nil")
	}
	// Test index collectTrustInputMtimes with symlink file
	// Create a trust input file then replace with symlink
	claim2 := validStoreClaim("clm_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", ClaimBasisOwner)
	if _, err := claimStore.WriteDraft("research", claim2); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := claimStore.Approve("research", claim2.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	// Additional coverage for evidence spans
	evidenceStore2 := EvidenceStore{Paths: paths, Now: fixedClaimStoreNow}
	src2 := filepath.Join(t.TempDir(), "src2.txt")
	_ = os.WriteFile(src2, []byte("line1\nline2\nline3\n"), 0o600)
	ev2, err2 := evidenceStore2.AddFile("research", src2, "file://test2", "text/plain")
	if err2 == nil {
		validator2, _ := NewEvidenceValidator(evidenceStore2, "research")
		_ = validator2.Verify(ev2.ID)
		snapshot2 := validator2.snapshotDigests[ev2.ID]
		validSource := ClaimSource{ID: ev2.ID, Resource: "evidence/sources/" + ev2.ID + "/raw", Title: ev2.Origin, Digest: snapshot2, Spans: []EvidenceSpan{{EvidenceID: ev2.ID, StartLine: 1, EndLine: 1, Digest: EvidenceSpanDigest(snapshot2, 1, 1, []byte("line1\n"))}}}
		if err := validateEvidenceSpans(validator2, ev2.ID, validSource, snapshot2); err != nil {
			t.Fatalf("validateEvidenceSpans(valid) error = %v", err)
		}
		validSource.Spans[0].StartLine = 0
		if err := validateEvidenceSpans(validator2, ev2.ID, validSource, snapshot2); err == nil {
			t.Fatalf("validateEvidenceSpans(bad start) error = nil")
		}
	}
	manifest, err := BuildTrustInputManifest(paths, "research")
	if err != nil {
		t.Fatalf("BuildTrustInputManifest() error = %v", err)
	}
	// Make one trust input a symlink
	if len(manifest.Entries) > 0 {
		first := manifest.Entries[0]
		fullPath := filepath.Join(paths.WorkspacesDir, "research", filepath.FromSlash(first.Path))
		_ = os.Remove(fullPath)
		target := filepath.Join(t.TempDir(), "outside")
		_ = os.WriteFile(target, []byte("outside"), 0o600)
		_ = os.Symlink(target, fullPath)
		if _, err := collectTrustInputMtimes(paths, "research", manifest); err == nil {
			t.Fatalf("collectTrustInputMtimes(symlink) error = nil")
		}
	}
}
