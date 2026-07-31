package runtime

import (
	"strings"
	"testing"
	"time"
)

func TestClaimIDFormat(t *testing.T) {
	id, err := NewClaimID()
	if err != nil {
		t.Fatalf("NewClaimID() error = %v", err)
	}
	if !claimIDPattern.MatchString(id) {
		t.Fatalf("NewClaimID() = %q, want clm_ plus 32 lowercase hex chars", id)
	}
}

func TestClaimRoundTripPreservesBodyAndMetadata(t *testing.T) {
	created := "2026-07-30T09:00:00Z"
	claim := Claim{
		Schema:             ClaimSchemaVersion,
		ID:                 "clm_0123456789abcdef0123456789abcdef",
		Tier:               "projects",
		Status:             ClaimStatusDraft,
		Title:              "Trusted Ask Contract",
		Basis:              ClaimBasisEvidence,
		CreatedAt:          created,
		CreatedBy:          "owner",
		EvidenceIDs:        []string{"evd_0123456789abcdef0123456789abcdef"},
		SupportingClaimIDs: []string{"clm_11111111111111111111111111111111"},
		Supersedes:         []string{"clm_22222222222222222222222222222222"},
		ConflictsWith:      []string{"clm_33333333333333333333333333333333"},
		Tags:               []string{"memory", "trust"},
		Body:               "# Body\n\nKeep this exact markdown body.\n\n```go\nfmt.Println(\"unchanged\")\n```\n",
	}

	rendered, err := RenderClaimMarkdown(claim)
	if err != nil {
		t.Fatalf("RenderClaimMarkdown() error = %v", err)
	}
	parsed, err := ParseClaimMarkdown("projects", "projects/trusted-ask.md", rendered)
	if err != nil {
		t.Fatalf("ParseClaimMarkdown() error = %v", err)
	}

	if parsed.Body != claim.Body {
		t.Fatalf("Body changed after round trip:\n got: %q\nwant: %q", parsed.Body, claim.Body)
	}
	if parsed.ID != claim.ID || parsed.Title != claim.Title || parsed.Status != claim.Status || parsed.Basis != claim.Basis || parsed.CreatedAt != created {
		t.Fatalf("parsed metadata mismatch: %#v", parsed)
	}
	if got := strings.Join(parsed.Tags, ","); got != "memory,trust" {
		t.Fatalf("Tags = %q", got)
	}

	renderedAgain, err := RenderClaimMarkdown(parsed)
	if err != nil {
		t.Fatalf("RenderClaimMarkdown(parsed) error = %v", err)
	}
	if string(renderedAgain) != string(rendered) {
		t.Fatalf("render is not deterministic:\nfirst:\n%s\nsecond:\n%s", rendered, renderedAgain)
	}
}

func TestClaimValidationRejectsTierMismatch(t *testing.T) {
	contents := []byte("---\nschema: zbrain.claim/v1\nid: clm_0123456789abcdef0123456789abcdef\nstatus: draft\ntitle: Tier mismatch\nbasis: owner\ncreated_at: 2026-07-30T09:00:00Z\ncreated_by: owner\n---\n\nBody\n")
	_, err := ParseClaimMarkdown("projects", "decisions/tier-mismatch.md", contents)
	if err == nil {
		t.Fatalf("ParseClaimMarkdown() error = nil, want tier mismatch")
	}
	if !strings.Contains(err.Error(), "tier") {
		t.Fatalf("ParseClaimMarkdown() error = %v, want tier error", err)
	}
}

func TestClaimValidationRejectsInvalidRelations(t *testing.T) {
	claim := validOwnerClaim()
	claim.ConflictsWith = []string{"not-a-claim-id"}
	if err := ValidateClaim(claim); err == nil {
		t.Fatalf("ValidateClaim() error = nil, want invalid relation error")
	}
}

func TestClaimValidationRejectsUnsupportedStatusAndBasis(t *testing.T) {
	claim := validOwnerClaim()
	claim.Status = "published"
	if err := ValidateClaim(claim); err == nil {
		t.Fatalf("ValidateClaim() status error = nil")
	}

	claim = validOwnerClaim()
	claim.Basis = "rumor"
	if err := ValidateClaim(claim); err == nil {
		t.Fatalf("ValidateClaim() basis error = nil")
	}
}

func TestClaimApprovalGuardsByBasis(t *testing.T) {
	owner := validOwnerClaim()
	owner.Basis = ClaimBasisOwner
	if err := ValidateClaimApproval(owner); err != nil {
		t.Fatalf("ValidateClaimApproval(owner) error = %v", err)
	}

	evidence := validOwnerClaim()
	evidence.Basis = ClaimBasisEvidence
	if err := ValidateClaimApproval(evidence); err == nil {
		t.Fatalf("ValidateClaimApproval(evidence without evidence) error = nil")
	}
	evidence.EvidenceIDs = []string{"evd_0123456789abcdef0123456789abcdef"}
	if err := ValidateClaimApproval(evidence); err != nil {
		t.Fatalf("ValidateClaimApproval(evidence) error = %v", err)
	}

	derived := validOwnerClaim()
	derived.Basis = ClaimBasisDerived
	if err := ValidateClaimApproval(derived); err == nil {
		t.Fatalf("ValidateClaimApproval(derived without support) error = nil")
	}
	derived.SupportingClaimIDs = []string{"clm_11111111111111111111111111111111"}
	if err := ValidateClaimApproval(derived); err != nil {
		t.Fatalf("ValidateClaimApproval(derived) error = %v", err)
	}
}

func TestClaimValidationRejectsBadCreatedAt(t *testing.T) {
	claim := validOwnerClaim()
	claim.CreatedAt = time.Now().Format(time.RFC1123)
	if err := ValidateClaim(claim); err == nil {
		t.Fatalf("ValidateClaim() error = nil, want RFC3339 validation error")
	}
}

func validOwnerClaim() Claim {
	return Claim{
		Schema:    ClaimSchemaVersion,
		ID:        "clm_0123456789abcdef0123456789abcdef",
		Tier:      "projects",
		Status:    ClaimStatusDraft,
		Title:     "Owner preference",
		Basis:     ClaimBasisOwner,
		CreatedAt: "2026-07-30T09:00:00Z",
		CreatedBy: "owner",
		Body:      "Body\n",
	}
}
