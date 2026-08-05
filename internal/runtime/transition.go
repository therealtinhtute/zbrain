package runtime

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const pendingTransitionFileName = "pending-transition.json"

type PendingTransition struct {
	OperationID string                    `json:"operation_id"`
	Kind        ClaimTransitionKind       `json:"kind"`
	Workspace   string                    `json:"workspace"`
	Targets     []PendingTransitionTarget `json:"targets"`
}

type PendingTransitionTarget struct {
	Path           string `json:"path"`
	PreimageSHA256 string `json:"preimage_sha256"`
	TargetSHA256   string `json:"target_sha256"`
	TargetBytes    []byte `json:"target_bytes"`
}

func NewPendingTransitionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "txn_" + hex.EncodeToString(buf), nil
}

func PendingTransitionPath(paths Paths, workspace string) (string, error) {
	root, err := ValidateWorkspace(paths, workspace)
	if err != nil {
		return "", err
	}
	directory := filepath.Join(root, ".zbrain")
	if err := validatePendingTransitionDirectory(root, directory); err != nil {
		return "", err
	}
	return filepath.Join(directory, pendingTransitionFileName), nil
}

func WritePendingTransition(paths Paths, workspace string, pending PendingTransition) error {
	journalPath, err := PendingTransitionPath(paths, workspace)
	if err != nil {
		return err
	}
	normalized, err := normalizePendingTransition(workspace, pending)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pending transition: %w", err)
	}
	encoded = append(encoded, '\n')

	if err := os.MkdirAll(filepath.Dir(journalPath), 0o755); err != nil {
		return fmt.Errorf("create pending transition directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(journalPath), "."+pendingTransitionFileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("create pending transition temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set pending transition temporary permissions: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write pending transition journal: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync pending transition journal: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close pending transition journal: %w", err)
	}
	if err := os.Link(temporaryPath, journalPath); err != nil {
		return fmt.Errorf("publish pending transition journal: %w", err)
	}
	return nil
}

func ReadPendingTransition(paths Paths, workspace string) (PendingTransition, error) {
	journalPath, err := PendingTransitionPath(paths, workspace)
	if err != nil {
		return PendingTransition{}, err
	}
	contents, err := os.ReadFile(journalPath)
	if err != nil {
		return PendingTransition{}, err
	}
	var pending PendingTransition
	if err := json.Unmarshal(contents, &pending); err != nil {
		return PendingTransition{}, fmt.Errorf("decode pending transition journal: %w", err)
	}
	if err := validatePendingTransition(workspace, pending, true); err != nil {
		return PendingTransition{}, err
	}
	return pending, nil
}

// CheckPendingTransition is read-only and blocks trust while a journal exists.
func CheckPendingTransition(paths Paths, workspace string) error {
	pending, err := ReadPendingTransition(paths, workspace)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("workspace %q pending transition is invalid: %w", workspace, err)
	}
	return fmt.Errorf("workspace %q has pending transition %q; run zbrain reindex", workspace, pending.OperationID)
}

// RecoverPendingTransitionForMutation marks the index dirty before applying a pending journal.
func RecoverPendingTransitionForMutation(paths Paths, workspace string) error {
	pending, err := ReadPendingTransition(paths, workspace)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("workspace %q pending transition is invalid: %w", workspace, err)
	}
	if err := (IndexStore{Paths: paths}).MarkDirty(workspace); err != nil {
		return fmt.Errorf("mark workspace dirty before pending transition recovery: %w", err)
	}
	if err := RecoverPendingTransition(paths, workspace); err != nil {
		return fmt.Errorf("recover pending transition %q: %w", pending.OperationID, err)
	}
	return nil
}

func RecoverPendingTransition(paths Paths, workspace string) error {
	pending, err := ReadPendingTransition(paths, workspace)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	journalPath, err := PendingTransitionPath(paths, workspace)
	if err != nil {
		return err
	}

	type targetState struct {
		path    string
		current string
		apply   bool
	}
	states := make([]targetState, len(pending.Targets))
	for i, target := range pending.Targets {
		path, err := ResolveWorkspacePath(paths, workspace, target.Path)
		if err != nil {
			return fmt.Errorf("resolve pending transition target %q: %w", target.Path, err)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("pending transition target %q is missing; expected preimage %s", target.Path, target.PreimageSHA256)
			}
			return fmt.Errorf("read pending transition target %q: %w", target.Path, err)
		}
		current := transitionSHA256(contents)
		switch current {
		case target.TargetSHA256:
			states[i] = targetState{path: path, current: current}
		case target.PreimageSHA256:
			states[i] = targetState{path: path, current: current, apply: true}
		default:
			return fmt.Errorf("pending transition target %q preimage mismatch: got %s, want %s or %s", target.Path, current, target.PreimageSHA256, target.TargetSHA256)
		}
	}

	for i, target := range pending.Targets {
		if !states[i].apply {
			continue
		}
		if err := writeTransitionBytesAtomic(states[i].path, target.TargetBytes); err != nil {
			return fmt.Errorf("apply pending transition target %q: %w", target.Path, err)
		}
	}
	for i, target := range pending.Targets {
		contents, err := os.ReadFile(states[i].path)
		if err != nil {
			return fmt.Errorf("verify pending transition target %q: %w", target.Path, err)
		}
		if got := transitionSHA256(contents); got != target.TargetSHA256 {
			return fmt.Errorf("pending transition target %q did not reach target hash: got %s, want %s", target.Path, got, target.TargetSHA256)
		}
	}
	if err := os.Remove(journalPath); err != nil {
		return fmt.Errorf("remove completed pending transition journal: %w", err)
	}
	return nil
}

func ValidatePendingTransition(workspace string, pending PendingTransition) error {
	return validatePendingTransition(workspace, pending, false)
}

func normalizePendingTransition(workspace string, pending PendingTransition) (PendingTransition, error) {
	if err := validatePendingTransition(workspace, pending, false); err != nil {
		return PendingTransition{}, err
	}
	normalized := pending
	normalized.Targets = append([]PendingTransitionTarget(nil), pending.Targets...)
	sort.Slice(normalized.Targets, func(i, j int) bool {
		return normalized.Targets[i].Path < normalized.Targets[j].Path
	})
	return normalized, nil
}

func validatePendingTransition(workspace string, pending PendingTransition, requireSorted bool) error {
	if !IsSafeWorkspaceName(workspace) {
		return fmt.Errorf("pending transition workspace is not safe")
	}
	if pending.Workspace != workspace {
		return fmt.Errorf("pending transition workspace %q does not match %q", pending.Workspace, workspace)
	}
	if strings.TrimSpace(pending.OperationID) == "" {
		return fmt.Errorf("pending transition operation_id is required")
	}
	switch pending.Kind {
	case ClaimTransitionApprove, ClaimTransitionSupersede, ClaimTransitionRevoke:
	default:
		return fmt.Errorf("pending transition kind %q is not supported", pending.Kind)
	}
	if len(pending.Targets) == 0 {
		return fmt.Errorf("pending transition targets are required")
	}
	previous := ""
	for _, target := range pending.Targets {
		if _, err := safeRelativePath(target.Path); err != nil {
			return fmt.Errorf("pending transition target %q is unsafe: %w", target.Path, err)
		}
		if filepath.ToSlash(filepath.Clean(filepath.FromSlash(target.Path))) != target.Path {
			return fmt.Errorf("pending transition target %q is not slash-normalized", target.Path)
		}
		if !isTransitionSHA256(target.PreimageSHA256) {
			return fmt.Errorf("pending transition target %q has invalid preimage hash", target.Path)
		}
		if !isTransitionSHA256(target.TargetSHA256) {
			return fmt.Errorf("pending transition target %q has invalid target hash", target.Path)
		}
		if got := transitionSHA256(target.TargetBytes); got != target.TargetSHA256 {
			return fmt.Errorf("pending transition target %q target hash does not match target bytes", target.Path)
		}
		if previous != "" && target.Path <= previous {
			if target.Path == previous {
				return fmt.Errorf("pending transition target %q is duplicated", target.Path)
			}
			if requireSorted {
				return fmt.Errorf("pending transition targets are not sorted at %q", target.Path)
			}
		}
		previous = target.Path
	}
	return nil
}

func validatePendingTransitionDirectory(root string, directory string) error {
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("pending transition directory must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("pending transition directory is not a directory")
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
		return fmt.Errorf("pending transition directory resolves outside workspace")
	}
	return nil
}

func isTransitionSHA256(value string) bool {
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(decoded) == sha256.Size
}

func transitionSHA256(contents []byte) string {
	sum := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func writeTransitionBytesAtomic(path string, contents []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(contents); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
