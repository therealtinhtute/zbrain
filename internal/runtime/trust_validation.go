package runtime

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

type TrustValidationError struct {
	RootID string
	Path   []string
	Reason string
}

func (err *TrustValidationError) Error() string {
	return fmt.Sprintf("trust dependency validation failed for %s: %s (path: %s)", err.RootID, err.Reason, strings.Join(err.Path, " -> "))
}

type trustVisitState uint8

const (
	trustUnvisited trustVisitState = iota
	trustVisiting
	trustValid
	trustInvalid
)

type trustFailure struct {
	path   []string
	reason string
}

type TrustValidator struct {
	claims             map[string]Claim
	load               func(string) (Claim, error)
	states             map[string]trustVisitState
	failures           map[string]trustFailure
	visitCount         map[string]int
	activeRoots        map[string]bool
	validateSupporting func(Claim) error
}

func NewTrustValidator(claims []Claim) (*TrustValidator, error) {
	validator := &TrustValidator{
		claims:      make(map[string]Claim, len(claims)),
		states:      make(map[string]trustVisitState, len(claims)),
		failures:    make(map[string]trustFailure),
		visitCount:  make(map[string]int, len(claims)),
		activeRoots: make(map[string]bool),
	}
	ordered := append([]Claim(nil), claims...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	for _, claim := range ordered {
		if !claimIDPattern.MatchString(claim.ID) {
			return nil, fmt.Errorf("trust validator claim id %q is unsafe", claim.ID)
		}
		if _, exists := validator.claims[claim.ID]; exists {
			return nil, fmt.Errorf("trust validator claim %q is duplicated", claim.ID)
		}
		validator.claims[claim.ID] = claim
	}
	return validator, nil
}

func NewTrustValidatorFromStore(store ClaimStore, workspace string) (*TrustValidator, error) {
	if _, err := ValidateWorkspace(store.Paths, workspace); err != nil {
		return nil, err
	}
	validator, err := NewTrustValidator(nil)
	if err != nil {
		return nil, err
	}
	validator.load = func(id string) (Claim, error) {
		return store.Read(workspace, id)
	}
	return validator, nil
}

func (validator *TrustValidator) ValidateClaim(root Claim) error {
	if !claimIDPattern.MatchString(root.ID) {
		return &TrustValidationError{
			RootID: root.ID,
			Path:   []string{root.ID},
			Reason: fmt.Sprintf("root claim id %q is unsafe", root.ID),
		}
	}
	validator.activeRoots[root.ID] = true
	defer delete(validator.activeRoots, root.ID)
	ids := append([]string(nil), root.SupportingClaimIDs...)
	sort.Strings(ids)
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		path := []string{root.ID, id}
		if !claimIDPattern.MatchString(id) {
			return &TrustValidationError{RootID: root.ID, Path: path, Reason: fmt.Sprintf("supporting claim id %q is unsafe", id)}
		}
		if _, exists := seen[id]; exists {
			return &TrustValidationError{RootID: root.ID, Path: path, Reason: fmt.Sprintf("supporting claim id %q is duplicated", id)}
		}
		seen[id] = struct{}{}
		if err := validator.visit(root.ID, id, path); err != nil {
			return err
		}
	}
	return nil
}

func (validator *TrustValidator) visit(rootID string, id string, path []string) error {
	if validator.activeRoots[rootID] && id == rootID {
		return validator.cycle(rootID, id, path)
	}
	switch validator.states[id] {
	case trustValid:
		return nil
	case trustInvalid:
		failure := validator.failures[id]
		return newTrustValidationError(rootID, combineTrustPath(path, failure.path), failure.reason)
	case trustVisiting:
		return validator.cycle(rootID, id, path)
	}

	claim, exists := validator.claims[id]
	if !exists && validator.load != nil {
		loaded, loadErr := validator.load(id)
		if loadErr == nil {
			claim = loaded
			exists = true
			validator.claims[id] = claim
		} else {
			reason := fmt.Sprintf("supporting claim %s cannot be parsed: %v", id, loadErr)
			if errors.Is(loadErr, os.ErrNotExist) {
				reason = fmt.Sprintf("missing supporting claim %s", id)
			}
			failure := trustFailure{path: []string{id}, reason: reason}
			validator.markInvalid(id, failure)
			return newTrustValidationError(rootID, path, failure.reason)
		}
	}
	if !exists {
		failure := trustFailure{path: []string{id}, reason: fmt.Sprintf("missing supporting claim %s", id)}
		validator.markInvalid(id, failure)
		return newTrustValidationError(rootID, path, failure.reason)
	}
	validator.states[id] = trustVisiting
	validator.visitCount[id]++
	if claim.Status != ClaimStatusApproved {
		failure := trustFailure{path: []string{id}, reason: fmt.Sprintf("supporting claim %s is %s; only approved claims are trusted", id, claim.Status)}
		validator.markInvalid(id, failure)
		return newTrustValidationError(rootID, path, failure.reason)
	}
	if err := VerifyClaimDigest(claim); err != nil {
		failure := trustFailure{path: []string{id}, reason: fmt.Sprintf("supporting claim %s: %v", id, err)}
		validator.markInvalid(id, failure)
		return newTrustValidationError(rootID, path, failure.reason)
	}
	if validator.validateSupporting != nil {
		if err := validator.validateSupporting(claim); err != nil {
			failure := trustFailure{path: []string{id}, reason: fmt.Sprintf("supporting claim %s: %v", id, err)}
			validator.markInvalid(id, failure)
			return newTrustValidationError(rootID, path, failure.reason)
		}
	}

	ids := append([]string(nil), claim.SupportingClaimIDs...)
	sort.Strings(ids)
	seen := make(map[string]struct{}, len(ids))
	for _, supportID := range ids {
		if !claimIDPattern.MatchString(supportID) {
			failure := trustFailure{path: []string{id, supportID}, reason: fmt.Sprintf("supporting claim id %q is unsafe", supportID)}
			validator.markInvalid(id, failure)
			return newTrustValidationError(rootID, combineTrustPath(path, failure.path), failure.reason)
		}
		if _, exists := seen[supportID]; exists {
			failure := trustFailure{path: []string{id, supportID}, reason: fmt.Sprintf("supporting claim id %q is duplicated", supportID)}
			validator.markInvalid(id, failure)
			return newTrustValidationError(rootID, combineTrustPath(path, failure.path), failure.reason)
		}
		seen[supportID] = struct{}{}
		childPath := append(append([]string(nil), path...), supportID)
		if err := validator.visit(rootID, supportID, childPath); err != nil {
			if validator.states[id] != trustInvalid {
				if failure, exists := validator.failures[supportID]; exists {
					validator.markInvalid(id, trustFailure{path: append([]string{id}, failure.path...), reason: failure.reason})
				} else if validationErr, ok := err.(*TrustValidationError); ok {
					validator.markInvalid(id, trustFailure{path: []string{id, supportID}, reason: validationErr.Reason})
				}
			}
			return err
		}
	}
	validator.states[id] = trustValid
	return nil
}

func (validator *TrustValidator) cycle(rootID string, id string, path []string) error {
	start := 0
	for index, candidate := range path {
		if candidate == id {
			start = index
			break
		}
	}
	cyclePath := append([]string(nil), path[start:]...)
	nodes := append([]string(nil), cyclePath[:len(cyclePath)-1]...)
	for index, node := range nodes {
		rotated := append([]string(nil), nodes[index:]...)
		rotated = append(rotated, nodes[:index]...)
		rotated = append(rotated, node)
		validator.markInvalid(node, trustFailure{path: rotated, reason: "dependency cycle detected"})
	}
	return newTrustValidationError(rootID, cyclePath, "dependency cycle detected")
}

func (validator *TrustValidator) markInvalid(id string, failure trustFailure) {
	if validator.states[id] == trustInvalid {
		return
	}
	validator.states[id] = trustInvalid
	failure.path = append([]string(nil), failure.path...)
	validator.failures[id] = failure
}

func newTrustValidationError(rootID string, path []string, reason string) error {
	return &TrustValidationError{RootID: rootID, Path: append([]string(nil), path...), Reason: reason}
}

func combineTrustPath(current []string, suffix []string) []string {
	if len(current) == 0 {
		return append([]string(nil), suffix...)
	}
	path := append([]string(nil), current[:len(current)-1]...)
	return append(path, suffix...)
}
