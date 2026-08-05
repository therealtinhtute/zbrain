package runtime

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestTrustValidationChain(t *testing.T) {
	leaf := trustTestApprovedClaim(3)
	middle := trustTestApprovedClaim(2, leaf.ID)
	root := trustTestApprovedClaim(1, middle.ID)
	validator, err := NewTrustValidator([]Claim{root, leaf, middle})
	if err != nil {
		t.Fatalf("NewTrustValidator() error = %v", err)
	}
	if err := validator.ValidateClaim(root); err != nil {
		t.Fatalf("ValidateClaim() error = %v", err)
	}

	deep := make([]Claim, 0, 129)
	for number := 129; number >= 1; number-- {
		if number == 129 {
			deep = append(deep, trustTestApprovedClaim(number))
			continue
		}
		deep = append(deep, trustTestApprovedClaim(number, trustTestClaimID(number+1)))
	}
	deepValidator, err := NewTrustValidator(deep)
	if err != nil {
		t.Fatalf("NewTrustValidator(deep) error = %v", err)
	}
	if err := deepValidator.ValidateClaim(deep[len(deep)-1]); err != nil {
		t.Fatalf("ValidateClaim(deep) error = %v", err)
	}
}

func TestTrustValidationMissing(t *testing.T) {
	t.Run("missing node", func(t *testing.T) {
		root := trustTestApprovedClaim(1, trustTestClaimID(99))
		validator, err := NewTrustValidator([]Claim{root})
		if err != nil {
			t.Fatalf("NewTrustValidator() error = %v", err)
		}
		validationErr := requireTrustValidationError(t, validator.ValidateClaim(root))
		if !strings.Contains(validationErr.Reason, "missing supporting claim") {
			t.Fatalf("Reason = %q, want missing supporting claim", validationErr.Reason)
		}
		wantPath := []string{root.ID, trustTestClaimID(99)}
		if !reflect.DeepEqual(validationErr.Path, wantPath) {
			t.Fatalf("Path = %#v, want %#v", validationErr.Path, wantPath)
		}
	})

	t.Run("unsafe node", func(t *testing.T) {
		root := trustTestClaim(1, ClaimStatusApproved, "../outside")
		validator, err := NewTrustValidator([]Claim{root})
		if err != nil {
			t.Fatalf("NewTrustValidator() error = %v", err)
		}
		validationErr := requireTrustValidationError(t, validator.ValidateClaim(root))
		if !strings.Contains(validationErr.Reason, "unsafe") {
			t.Fatalf("Reason = %q, want unsafe", validationErr.Reason)
		}
	})

	t.Run("duplicate node", func(t *testing.T) {
		support := trustTestApprovedClaim(2)
		root := trustTestApprovedClaim(1, support.ID, support.ID)
		validator, err := NewTrustValidator([]Claim{root, support})
		if err != nil {
			t.Fatalf("NewTrustValidator() error = %v", err)
		}
		validationErr := requireTrustValidationError(t, validator.ValidateClaim(root))
		if !strings.Contains(validationErr.Reason, "duplicated") {
			t.Fatalf("Reason = %q, want duplicated", validationErr.Reason)
		}
	})
}

func TestTrustValidationStatus(t *testing.T) {
	for number, status := range map[int]ClaimStatus{
		2: ClaimStatusDraft,
		3: ClaimStatusRevoked,
		4: ClaimStatusSuperseded,
	} {
		t.Run(string(status), func(t *testing.T) {
			support := trustTestClaim(number, status)
			root := trustTestApprovedClaim(1, support.ID)
			validator, err := NewTrustValidator([]Claim{root, support})
			if err != nil {
				t.Fatalf("NewTrustValidator() error = %v", err)
			}
			validationErr := requireTrustValidationError(t, validator.ValidateClaim(root))
			if !strings.Contains(validationErr.Reason, string(status)) {
				t.Fatalf("Reason = %q, want status %q", validationErr.Reason, status)
			}
		})
	}
}

func TestTrustValidationDigest(t *testing.T) {
	support := trustTestApprovedClaim(2)
	support.Body = "tampered\n"
	root := trustTestApprovedClaim(1, support.ID)
	validator, err := NewTrustValidator([]Claim{root, support})
	if err != nil {
		t.Fatalf("NewTrustValidator() error = %v", err)
	}
	validationErr := requireTrustValidationError(t, validator.ValidateClaim(root))
	if !strings.Contains(validationErr.Reason, "verification digest mismatch") {
		t.Fatalf("Reason = %q, want verification digest mismatch", validationErr.Reason)
	}
}

func TestTrustValidationCycle(t *testing.T) {
	first := trustTestApprovedClaim(1, trustTestClaimID(2))
	second := trustTestApprovedClaim(2, first.ID)
	validator, err := NewTrustValidator([]Claim{second, first})
	if err != nil {
		t.Fatalf("NewTrustValidator() error = %v", err)
	}
	validationErr := requireTrustValidationError(t, validator.ValidateClaim(first))
	if validationErr.Reason != "dependency cycle detected" {
		t.Fatalf("Reason = %q, want dependency cycle detected", validationErr.Reason)
	}
	wantPath := []string{first.ID, second.ID, first.ID}
	if !reflect.DeepEqual(validationErr.Path, wantPath) {
		t.Fatalf("Path = %#v, want %#v", validationErr.Path, wantPath)
	}
}

func TestTrustValidationDeterministic(t *testing.T) {
	missingFirst := trustTestClaimID(10)
	missingSecond := trustTestClaimID(11)
	rootA := trustTestApprovedClaim(1, missingSecond, missingFirst)
	rootB := trustTestApprovedClaim(1, missingFirst, missingSecond)

	firstValidator, err := NewTrustValidator([]Claim{rootA})
	if err != nil {
		t.Fatalf("NewTrustValidator(first) error = %v", err)
	}
	secondValidator, err := NewTrustValidator([]Claim{rootB})
	if err != nil {
		t.Fatalf("NewTrustValidator(second) error = %v", err)
	}
	firstErr := requireTrustValidationError(t, firstValidator.ValidateClaim(rootA))
	secondErr := requireTrustValidationError(t, secondValidator.ValidateClaim(rootB))
	if firstErr.Error() != secondErr.Error() {
		t.Fatalf("validation errors differ:\nfirst: %s\nsecond: %s", firstErr, secondErr)
	}
}

func TestTrustValidationMemoized(t *testing.T) {
	shared := trustTestApprovedClaim(3)
	first := trustTestApprovedClaim(1, shared.ID)
	second := trustTestApprovedClaim(2, shared.ID)
	validator, err := NewTrustValidator([]Claim{second, shared, first})
	if err != nil {
		t.Fatalf("NewTrustValidator() error = %v", err)
	}
	if err := validator.ValidateClaim(first); err != nil {
		t.Fatalf("ValidateClaim(first) error = %v", err)
	}
	if err := validator.ValidateClaim(second); err != nil {
		t.Fatalf("ValidateClaim(second) error = %v", err)
	}
	if got := validator.visitCount[shared.ID]; got != 1 {
		t.Fatalf("shared visit count = %d, want 1", got)
	}
}

func trustTestApprovedClaim(number int, supporting ...string) Claim {
	claim := trustTestClaim(number, ClaimStatusApproved, supporting...)
	claim.VerifiedAt = "2026-01-01T00:00:00Z"
	claim.VerifiedBy = "test"
	digest, err := ClaimVerificationDigest(claim)
	if err != nil {
		panic(fmt.Sprintf("trust test claim digest: %v", err))
	}
	claim.VerifiedDigest = digest
	return claim
}

func trustTestClaim(number int, status ClaimStatus, supporting ...string) Claim {
	return Claim{
		ID:                 trustTestClaimID(number),
		Tier:               "projects",
		Status:             status,
		Title:              fmt.Sprintf("claim %d", number),
		Basis:              ClaimBasisDerived,
		CreatedAt:          "2026-01-01T00:00:00Z",
		CreatedBy:          "test",
		SupportingClaimIDs: append([]string(nil), supporting...),
		Body:               fmt.Sprintf("claim body %d\n", number),
	}
}

func trustTestClaimID(number int) string {
	return fmt.Sprintf("clm_%032x", number)
}

func requireTrustValidationError(t *testing.T, err error) *TrustValidationError {
	t.Helper()
	if err == nil {
		t.Fatal("ValidateClaim() error = nil")
	}
	var validationErr *TrustValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("ValidateClaim() error = %T %v, want *TrustValidationError", err, err)
	}
	return validationErr
}
