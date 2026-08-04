package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"sync"
)

const (
	TrustInputKindClaim            = "claim"
	TrustInputKindEvidenceMetadata = "evidence_metadata"
	TrustInputKindEvidenceRaw      = "evidence_raw"
)

type TrustInput struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	ByteLength int64  `json:"byte_length"`
	SHA256     string `json:"sha256"`
}

type TrustInputManifest struct {
	Entries []TrustInput `json:"entries"`
	Digest  string       `json:"digest"`
}

// BuildTrustInputManifest hashes the canonical files that can affect trust.
func BuildTrustInputManifest(paths Paths, workspace string) (TrustInputManifest, error) {
	root, err := ValidateWorkspace(paths, workspace)
	if err != nil {
		return TrustInputManifest{}, err
	}

	type manifestInput struct {
		path     string
		relative string
		kind     string
	}
	inputs := make([]manifestInput, 0)
	add := func(path string, kind string) error {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if !pathWithin(root, absolute) {
			return fmt.Errorf("trust input path %q is outside workspace", path)
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		inputs = append(inputs, manifestInput{path: absolute, relative: relative, kind: kind})
		return nil
	}

	for _, tier := range WikiTiers {
		tierRoot := filepath.Join(root, "wiki", tier)
		if err := validateTrustInputDirectory(root, tierRoot); err != nil {
			return TrustInputManifest{}, fmt.Errorf("validate wiki tier %q: %w", tier, err)
		}
		if err := walkTrustInputClaims(tierRoot, add); err != nil {
			return TrustInputManifest{}, err
		}
	}

	sourcesRoot := filepath.Join(root, "evidence", "sources")
	if err := walkTrustInputEvidence(sourcesRoot, add); err != nil {
		return TrustInputManifest{}, err
	}

	entries := make([]TrustInput, len(inputs))
	if len(inputs) > 0 {
		workerCount := goruntime.GOMAXPROCS(0) * 2
		if workerCount < 1 {
			workerCount = 1
		}
		if workerCount > 32 {
			workerCount = 32
		}
		if workerCount > len(inputs) {
			workerCount = len(inputs)
		}
		jobs := make(chan int)
		var firstErr error
		var errOnce sync.Once
		var workers sync.WaitGroup
		workers.Add(workerCount)
		for i := 0; i < workerCount; i++ {
			go func() {
				defer workers.Done()
				for index := range jobs {
					input := inputs[index]
					byteLength, digest, err := hashTrustInputFile(input.path)
					if err != nil {
						errOnce.Do(func() {
							firstErr = fmt.Errorf("hash trust input %q: %w", input.relative, err)
						})
						continue
					}
					entries[index] = TrustInput{
						Path:       input.relative,
						Kind:       input.kind,
						ByteLength: byteLength,
						SHA256:     digest,
					}
				}
			}()
		}
		for index := range inputs {
			jobs <- index
		}
		close(jobs)
		workers.Wait()
		if firstErr != nil {
			return TrustInputManifest{}, firstErr
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return TrustInputManifest{
		Entries: entries,
		Digest:  trustInputManifestDigest(entries),
	}, nil
}

func walkTrustInputClaims(root string, add func(string, string) error) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("trust input %q must not be a symlink", path)
		}
		if entry.IsDir() {
			if filepath.Ext(path) == ".md" {
				return fmt.Errorf("trust input %q is not a regular file", path)
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		if entry.Type() != 0 {
			return fmt.Errorf("trust input %q is not a regular file", path)
		}
		return add(path, TrustInputKindClaim)
	})
}

func walkTrustInputEvidence(root string, add func(string, string) error) error {
	if err := validateTrustInputDirectory(root, root); err != nil {
		return fmt.Errorf("validate evidence sources: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("evidence source %q must not be a symlink", entry.Name())
		}
		if !entry.IsDir() {
			continue
		}
		if err := validateTrustInputDirectory(root, path); err != nil {
			return err
		}
		for _, input := range []struct {
			name string
			kind string
		}{
			{name: "source.yaml", kind: TrustInputKindEvidenceMetadata},
			{name: "raw", kind: TrustInputKindEvidenceRaw},
		} {
			path := filepath.Join(root, entry.Name(), input.name)
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return fmt.Errorf("trust input %q is not a regular file", path)
			}
			if err := add(path, input.kind); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateTrustInputDirectory(root string, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q must not be a symlink", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return err
	}
	if !pathWithin(root, resolved) {
		return fmt.Errorf("resolved path escapes workspace")
	}
	if resolved != absolute {
		return fmt.Errorf("%q contains a symlink", path)
	}
	return nil
}

func hashTrustInputFile(path string) (int64, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	hash := sha256.New()
	byteLength, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return 0, "", copyErr
	}
	if closeErr != nil {
		return 0, "", closeErr
	}
	return byteLength, hex.EncodeToString(hash.Sum(nil)), nil
}

func trustInputManifestDigest(entries []TrustInput) string {
	encoded, err := json.Marshal(entries)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
