package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransitionJournalPath(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	journalPath, err := PendingTransitionPath(paths, "research")
	if err != nil {
		t.Fatalf("PendingTransitionPath() error = %v", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(filepath.Join(paths.WorkspacesDir, "research"))
	if err != nil {
		t.Fatalf("EvalSymlinks(workspace) error = %v", err)
	}
	want := filepath.Join(canonicalRoot, ".zbrain", pendingTransitionFileName)
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
