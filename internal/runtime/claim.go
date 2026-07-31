package runtime

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const ClaimSchemaVersion = "zbrain.claim/v1"

type ClaimStatus string

const (
	ClaimStatusDraft      ClaimStatus = "draft"
	ClaimStatusApproved   ClaimStatus = "approved"
	ClaimStatusSuperseded ClaimStatus = "superseded"
	ClaimStatusRevoked    ClaimStatus = "revoked"
)

type ClaimBasis string

const (
	ClaimBasisOwner    ClaimBasis = "owner"
	ClaimBasisEvidence ClaimBasis = "evidence"
	ClaimBasisDerived  ClaimBasis = "derived"
)

type Claim struct {
	Schema             string
	ID                 string
	Tier               string
	Path               string
	Status             ClaimStatus
	Title              string
	Basis              ClaimBasis
	CreatedAt          string
	CreatedBy          string
	EvidenceIDs        []string
	SupportingClaimIDs []string
	Supersedes         []string
	ConflictsWith      []string
	Tags               []string
	Body               string
}

type claimFrontmatter struct {
	Schema             string      `yaml:"schema"`
	ID                 string      `yaml:"id"`
	Status             ClaimStatus `yaml:"status"`
	Title              string      `yaml:"title"`
	Basis              ClaimBasis  `yaml:"basis"`
	CreatedAt          string      `yaml:"created_at"`
	CreatedBy          string      `yaml:"created_by"`
	EvidenceIDs        []string    `yaml:"evidence_ids,omitempty"`
	SupportingClaimIDs []string    `yaml:"supporting_claim_ids,omitempty"`
	Supersedes         []string    `yaml:"supersedes,omitempty"`
	ConflictsWith      []string    `yaml:"conflicts_with,omitempty"`
	Tags               []string    `yaml:"tags,omitempty"`
}

var (
	claimIDPattern    = regexp.MustCompile(`^clm_[0-9a-f]{32}$`)
	evidenceIDPattern = regexp.MustCompile(`^evd_[0-9a-f]{32}$`)
)

func NewClaimID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "clm_" + hex.EncodeToString(buf), nil
}

func ParseClaimMarkdown(tier string, relPath string, contents []byte) (Claim, error) {
	metadata, body, err := splitClaimFrontmatter(contents)
	if err != nil {
		return Claim{}, err
	}
	pathTier := strings.Split(filepath.ToSlash(relPath), "/")[0]
	if tier == "" {
		tier = pathTier
	}
	if pathTier != "" && pathTier != "." && pathTier != tier {
		return Claim{}, fmt.Errorf("claim path tier %q does not match expected tier %q", pathTier, tier)
	}

	claim := Claim{
		Schema:             metadata.Schema,
		ID:                 metadata.ID,
		Tier:               tier,
		Path:               filepath.ToSlash(relPath),
		Status:             metadata.Status,
		Title:              metadata.Title,
		Basis:              metadata.Basis,
		CreatedAt:          metadata.CreatedAt,
		CreatedBy:          metadata.CreatedBy,
		EvidenceIDs:        append([]string(nil), metadata.EvidenceIDs...),
		SupportingClaimIDs: append([]string(nil), metadata.SupportingClaimIDs...),
		Supersedes:         append([]string(nil), metadata.Supersedes...),
		ConflictsWith:      append([]string(nil), metadata.ConflictsWith...),
		Tags:               append([]string(nil), metadata.Tags...),
		Body:               string(body),
	}
	if err := ValidateClaim(claim); err != nil {
		return Claim{}, err
	}
	return claim, nil
}

func RenderClaimMarkdown(claim Claim) ([]byte, error) {
	if err := ValidateClaim(claim); err != nil {
		return nil, err
	}
	metadata := claimFrontmatter{
		Schema:             claim.Schema,
		ID:                 claim.ID,
		Status:             claim.Status,
		Title:              claim.Title,
		Basis:              claim.Basis,
		CreatedAt:          claim.CreatedAt,
		CreatedBy:          claim.CreatedBy,
		EvidenceIDs:        append([]string(nil), claim.EvidenceIDs...),
		SupportingClaimIDs: append([]string(nil), claim.SupportingClaimIDs...),
		Supersedes:         append([]string(nil), claim.Supersedes...),
		ConflictsWith:      append([]string(nil), claim.ConflictsWith...),
		Tags:               append([]string(nil), claim.Tags...),
	}
	encoded, err := yaml.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	out := bytes.NewBufferString("---\n")
	out.Write(encoded)
	out.WriteString("---\n")
	out.WriteString(claim.Body)
	return out.Bytes(), nil
}

func ValidateClaim(claim Claim) error {
	if claim.Schema != ClaimSchemaVersion {
		return fmt.Errorf("claim schema must be %q", ClaimSchemaVersion)
	}
	if !claimIDPattern.MatchString(claim.ID) {
		return fmt.Errorf("claim id must match clm_<32 lowercase hex chars>")
	}
	if !isKnownWikiTier(claim.Tier) {
		return fmt.Errorf("claim tier %q is not supported", claim.Tier)
	}
	if !isKnownClaimStatus(claim.Status) {
		return fmt.Errorf("claim status %q is not supported", claim.Status)
	}
	if strings.TrimSpace(claim.Title) == "" {
		return fmt.Errorf("claim title is required")
	}
	if !isKnownClaimBasis(claim.Basis) {
		return fmt.Errorf("claim basis %q is not supported", claim.Basis)
	}
	if _, err := time.Parse(time.RFC3339, claim.CreatedAt); err != nil {
		return fmt.Errorf("claim created_at must be RFC3339: %w", err)
	}
	if strings.TrimSpace(claim.CreatedBy) == "" {
		return fmt.Errorf("claim created_by is required")
	}
	for _, id := range claim.EvidenceIDs {
		if !evidenceIDPattern.MatchString(id) {
			return fmt.Errorf("evidence id %q must match evd_<32 lowercase hex chars>", id)
		}
	}
	for _, ids := range [][]string{claim.SupportingClaimIDs, claim.Supersedes, claim.ConflictsWith} {
		for _, id := range ids {
			if !claimIDPattern.MatchString(id) {
				return fmt.Errorf("related claim id %q must match clm_<32 lowercase hex chars>", id)
			}
		}
	}
	return nil
}

func ValidateClaimApproval(claim Claim) error {
	if err := ValidateClaim(claim); err != nil {
		return err
	}
	switch claim.Basis {
	case ClaimBasisOwner:
		return nil
	case ClaimBasisEvidence:
		if len(claim.EvidenceIDs) == 0 {
			return fmt.Errorf("evidence-based claims require at least one evidence id before approval")
		}
		return nil
	case ClaimBasisDerived:
		if len(claim.SupportingClaimIDs) == 0 && len(claim.EvidenceIDs) == 0 {
			return fmt.Errorf("derived claims require supporting claim or evidence ids before approval")
		}
		return nil
	default:
		return fmt.Errorf("claim basis %q is not supported", claim.Basis)
	}
}

func splitClaimFrontmatter(contents []byte) (claimFrontmatter, []byte, error) {
	if !bytes.HasPrefix(contents, []byte("---\n")) {
		return claimFrontmatter{}, nil, fmt.Errorf("claim frontmatter is required")
	}
	closing := bytes.Index(contents[4:], []byte("\n---\n"))
	if closing == -1 {
		return claimFrontmatter{}, nil, fmt.Errorf("claim frontmatter closing marker is required")
	}
	frontmatter := contents[4 : 4+closing]
	body := contents[4+closing+5:]
	var metadata claimFrontmatter
	if err := yaml.Unmarshal(frontmatter, &metadata); err != nil {
		return claimFrontmatter{}, nil, err
	}
	return metadata, body, nil
}

func isKnownWikiTier(tier string) bool {
	for _, known := range WikiTiers {
		if tier == known {
			return true
		}
	}
	return false
}

func isKnownClaimStatus(status ClaimStatus) bool {
	switch status {
	case ClaimStatusDraft, ClaimStatusApproved, ClaimStatusSuperseded, ClaimStatusRevoked:
		return true
	default:
		return false
	}
}

func isKnownClaimBasis(basis ClaimBasis) bool {
	switch basis {
	case ClaimBasisOwner, ClaimBasisEvidence, ClaimBasisDerived:
		return true
	default:
		return false
	}
}
