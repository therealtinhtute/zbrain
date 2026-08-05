package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func TestWorkspaceGenerationCanonicalMutationPublishesBarrierBeforeWrite(t *testing.T) {
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Barrier Claim", ClaimBasisOwner)
	beforeWrite := make(chan struct{})
	releaseWrite := make(chan struct{})
	var releaseWriteOnce sync.Once
	release := func() { releaseWriteOnce.Do(func() { close(releaseWrite) }) }
	restoreHook := setWorkspaceGenerationTestHook(workspaceGenerationHookBeforeCanonicalWrite, func() {
		close(beforeWrite)
		<-releaseWrite
	})
	defer restoreHook()
	defer release()

	result := make(chan error, 1)
	go func() {
		_, err := store.WriteDraft("research", claim)
		result <- err
	}()
	<-beforeWrite

	idx := IndexStore{Paths: paths}
	dirtyPath, err := idx.DirtyPath("research")
	if err != nil {
		t.Fatalf("DirtyPath() error = %v", err)
	}
	if _, err := os.Stat(dirtyPath); err != nil {
		t.Fatalf("dirty marker error = %v", err)
	}
	generation, err := readWorkspaceGeneration(paths, "research")
	if err != nil {
		t.Fatalf("readWorkspaceGeneration() error = %v", err)
	}
	if generation.Current != 1 || generation.Published != 0 {
		t.Fatalf("generation = %#v, want current=1 published=0", generation)
	}
	claimPath, err := store.claimPath("research", claim)
	if err != nil {
		t.Fatalf("claimPath() error = %v", err)
	}
	if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
		t.Fatalf("canonical claim stat error = %v, want missing before hook release", err)
	}

	release()
	if err := <-result; err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
}

func TestWorkspaceGenerationMutationDuringScanRejectsStalePublication(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("initial Rebuild() error = %v", err)
	}
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "Scan Mutation", ClaimBasisOwner)
	mutationResult := make(chan error, 1)
	restoreHook := setWorkspaceGenerationTestHook(workspaceGenerationHookRebuildAfterScan, func() {
		_, err := store.WriteDraft("research", claim)
		mutationResult <- err
	})
	defer restoreHook()

	if _, err := idx.Rebuild("research"); err == nil || !strings.Contains(err.Error(), "during rebuild") {
		t.Fatalf("Rebuild() error = %v, want stale rebuild rejection", err)
	}
	if err := <-mutationResult; err != nil {
		t.Fatalf("mutation during scan error = %v", err)
	}
	assertWorkspaceGenerationUnpublished(t, paths, "research")
	if _, err := os.Stat(indexDatabasePath(t, idx, "research")); err != nil {
		t.Fatalf("published database error = %v", err)
	}
}

func TestWorkspaceGenerationMutationBeforePublicationRejectsStalePublication(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("initial Rebuild() error = %v", err)
	}
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_ffffffffffffffffffffffffffffffff", "Publication Mutation", ClaimBasisOwner)
	mutationResult := make(chan error, 1)
	restoreHook := setWorkspaceGenerationTestHook(workspaceGenerationHookRebuildBeforePublication, func() {
		_, err := store.WriteDraft("research", claim)
		mutationResult <- err
	})
	defer restoreHook()

	rebuildErr := error(nil)
	if _, err := idx.Rebuild("research"); err != nil {
		rebuildErr = err
	}
	mutationErr := <-mutationResult
	if mutationErr != nil {
		t.Fatalf("mutation before publication error = %v (rebuild error = %v)", mutationErr, rebuildErr)
	}
	if rebuildErr == nil || !strings.Contains(rebuildErr.Error(), "changed during rebuild") {
		t.Fatalf("Rebuild() error = %v, want stale publication rejection", rebuildErr)
	}
	assertWorkspaceGenerationUnpublished(t, paths, "research")
}

func TestWorkspaceGenerationTrustedQuerySharedLockBlocksWriter(t *testing.T) {
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_dddddddddddddddddddddddddddddddd", "Shared Lock Claim", ClaimBasisOwner)
	claim.Body = "shared lock generation query body\n"
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	queryLocked := make(chan struct{})
	releaseQuery := make(chan struct{})
	var releaseQueryOnce sync.Once
	releaseQueryLock := func() { releaseQueryOnce.Do(func() { close(releaseQuery) }) }
	restoreHook := setWorkspaceGenerationTestHook(workspaceGenerationHookTrustedQueryAfterLocking, func() {
		close(queryLocked)
		<-releaseQuery
	})
	defer restoreHook()
	defer releaseQueryLock()

	queryResult := make(chan error, 1)
	go func() {
		_, err := TrustedQuery(paths, TrustedQueryOptions{Query: "shared lock generation", Limit: 10})
		queryResult <- err
	}()
	<-queryLocked

	if err := tryWorkspaceExclusiveLock(paths, "research"); !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
		t.Fatalf("exclusive writer lock error = %v, want would-block", err)
	}
	releaseQueryLock()
	if err := <-queryResult; err != nil {
		t.Fatalf("TrustedQuery() error = %v", err)
	}
	writerClaim := indexClaim("clm_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "After Shared Lock", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", writerClaim); err != nil {
		t.Fatalf("WriteDraft(after query) error = %v", err)
	}
}

func TestWorkspaceGenerationPendingRecoveryRemainsBlockedAndDirty(t *testing.T) {
	paths := indexTestPaths(t)
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "recovery.md")
	before := []byte("before\n")
	target := []byte("after\n")
	corrupt := []byte("changed\n")
	if err := os.WriteFile(claimPath, corrupt, 0o644); err != nil {
		t.Fatalf("WriteFile(corrupt) error = %v", err)
	}
	if err := WritePendingTransition(paths, "research", PendingTransition{
		OperationID: "txn_generation_blocked",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets:     []PendingTransitionTarget{pendingTransitionTarget("wiki/projects/recovery.md", before, target)},
	}); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}

	if err := RecoverPendingTransitionForMutation(paths, "research"); err == nil || !strings.Contains(err.Error(), "preimage mismatch") {
		t.Fatalf("RecoverPendingTransitionForMutation() error = %v, want preimage mismatch", err)
	}
	assertWorkspaceGenerationUnpublished(t, paths, "research")
	if _, err := ReadPendingTransition(paths, "research"); err != nil {
		t.Fatalf("ReadPendingTransition() error = %v, want journal preserved", err)
	}
	if err := (IndexStore{Paths: paths}).CheckFresh("research"); err == nil || !strings.Contains(err.Error(), "pending transition") {
		t.Fatalf("CheckFresh() error = %v, want pending transition block", err)
	}
}

func TestWorkspaceGenerationMissingAndMalformedStateFailsClosed(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	generationPath, err := idx.GenerationPath("research")
	if err != nil {
		t.Fatalf("GenerationPath() error = %v", err)
	}
	backupPath := generationPath + ".backup"
	if err := os.Rename(generationPath, backupPath); err != nil {
		t.Fatalf("Rename(generation) error = %v", err)
	}
	if err := idx.CheckFresh("research"); err == nil || !strings.Contains(err.Error(), "generation state is malformed or missing") {
		t.Fatalf("CheckFresh(missing generation) error = %v", err)
	}
	if err := os.Rename(backupPath, generationPath); err != nil {
		t.Fatalf("Restore(generation) error = %v", err)
	}
	if err := os.WriteFile(generationPath, []byte(`{"current":"not-a-number","published":0}`), 0o600); err != nil {
		t.Fatalf("WriteFile(malformed generation) error = %v", err)
	}
	if err := idx.CheckFresh("research"); err == nil || !strings.Contains(err.Error(), "generation state is malformed or missing") {
		t.Fatalf("CheckFresh(malformed generation) error = %v", err)
	}
}

func TestWorkspaceGenerationMarkDirtyDoesNotAdvance(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	before, err := readWorkspaceGeneration(paths, "research")
	if err != nil {
		t.Fatalf("readWorkspaceGeneration(before) error = %v", err)
	}
	if err := idx.MarkDirty("research"); err != nil {
		t.Fatalf("MarkDirty() error = %v", err)
	}
	after, err := readWorkspaceGeneration(paths, "research")
	if err != nil {
		t.Fatalf("readWorkspaceGeneration(after) error = %v", err)
	}
	if after != before {
		t.Fatalf("generation changed after MarkDirty: before=%#v after=%#v", before, after)
	}
}

func assertWorkspaceGenerationUnpublished(t *testing.T, paths Paths, workspace string) {
	t.Helper()
	generation, err := readWorkspaceGeneration(paths, workspace)
	if err != nil {
		t.Fatalf("readWorkspaceGeneration() error = %v", err)
	}
	if generation.Current == generation.Published {
		t.Fatalf("generation = %#v, want unpublished current", generation)
	}
	idx := IndexStore{Paths: paths}
	dirtyPath, err := idx.DirtyPath(workspace)
	if err != nil {
		t.Fatalf("DirtyPath() error = %v", err)
	}
	if _, err := os.Stat(dirtyPath); err != nil {
		t.Fatalf("dirty marker error = %v", err)
	}
}

func tryWorkspaceExclusiveLock(paths Paths, workspace string) error {
	lockPath, err := CoordinationLockPath(paths, workspace)
	if err != nil {
		return err
	}
	fd, err := unix.Open(lockPath, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
}
