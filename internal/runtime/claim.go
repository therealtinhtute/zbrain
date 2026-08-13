package runtime

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const ClaimSchemaVersion = "zbrain.claim/v1"
const OKFClaimType = "zbrain.claim"
const ZbrainTrustedMemoryProfile = "zbrain.trusted-memory/v1"

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
	Type               string
	ID                 string
	Tier               string
	Path               string
	Status             ClaimStatus
	Title              string
	Description        string
	Resource           string
	Basis              ClaimBasis
	CreatedAt          string
	CreatedBy          string
	VerifiedAt         string
	VerifiedBy         string
	VerifiedDigest     string
	StaleAfter         string
	Sources            []ClaimSource
	EvidenceIDs        []string
	SupportingClaimIDs []string
	Supersedes         []string
	ConflictsWith      []string
	Tags               []string
	Transitions        []ClaimTransition
	Body               string
}

type ClaimTransitionKind string

const (
	ClaimTransitionApprove   ClaimTransitionKind = "approve"
	ClaimTransitionSupersede ClaimTransitionKind = "supersede"
	ClaimTransitionRevoke    ClaimTransitionKind = "revoke"
)

type ClaimTransition struct {
	Kind                    ClaimTransitionKind `yaml:"kind" json:"kind"`
	At                      string              `yaml:"at" json:"at"`
	By                      string              `yaml:"by" json:"by"`
	Reason                  string              `yaml:"reason,omitempty" json:"reason,omitempty"`
	RelatedClaimIDs         []string            `yaml:"related_claim_ids,omitempty" json:"related_claim_ids,omitempty"`
	PriorVerificationDigest string              `yaml:"prior_verification_digest,omitempty" json:"prior_verification_digest,omitempty"`
}

type ClaimSource struct {
	ID       string         `yaml:"id" json:"id"`
	Resource string         `yaml:"resource" json:"resource"`
	Title    string         `yaml:"title,omitempty" json:"title,omitempty"`
	Digest   string         `yaml:"digest" json:"digest"`
	Spans    []EvidenceSpan `yaml:"spans,omitempty" json:"spans,omitempty"`
}

type EvidenceSpan struct {
	EvidenceID string `yaml:"evidence_id" json:"evidence_id"`
	StartLine  int    `yaml:"start_line" json:"start_line"`
	EndLine    int    `yaml:"end_line" json:"end_line"`
	Digest     string `yaml:"digest" json:"digest"`
}

type ClaimGenerated struct {
	At string `yaml:"at"`
	By string `yaml:"by"`
}

type ClaimVerified struct {
	At     string `yaml:"at"`
	By     string `yaml:"by"`
	Digest string `yaml:"digest"`
}

type claimFrontmatter struct {
	Type        string          `yaml:"type"`
	Title       string          `yaml:"title"`
	Description string          `yaml:"description,omitempty"`
	Resource    string          `yaml:"resource,omitempty"`
	Tags        []string        `yaml:"tags,omitempty"`
	Sources     []ClaimSource   `yaml:"sources,omitempty"`
	Generated   *ClaimGenerated `yaml:"generated,omitempty"`
	Verified    *ClaimVerified  `yaml:"verified,omitempty"`
	Status      ClaimStatus     `yaml:"status"`
	StaleAfter  string          `yaml:"stale_after,omitempty"`
	Zbrain      zbrainProfile   `yaml:"zbrain"`
}

type zbrainProfile struct {
	Profile            string            `yaml:"profile"`
	ID                 string            `yaml:"id"`
	Tier               string            `yaml:"tier"`
	Basis              ClaimBasis        `yaml:"basis"`
	EvidenceIDs        []string          `yaml:"evidence_ids,omitempty"`
	SupportingClaimIDs []string          `yaml:"supporting_claim_ids,omitempty"`
	Supersedes         []string          `yaml:"supersedes,omitempty"`
	ConflictsWith      []string          `yaml:"conflicts_with,omitempty"`
	Transitions        []ClaimTransition `yaml:"transitions,omitempty"`
}

type legacyClaimFrontmatter struct {
	Schema             string            `yaml:"schema"`
	ID                 string            `yaml:"id"`
	Status             ClaimStatus       `yaml:"status"`
	Title              string            `yaml:"title"`
	Description        string            `yaml:"description,omitempty"`
	Resource           string            `yaml:"resource,omitempty"`
	Basis              ClaimBasis        `yaml:"basis"`
	CreatedAt          string            `yaml:"created_at"`
	CreatedBy          string            `yaml:"created_by"`
	Verified           *ClaimVerified    `yaml:"verified,omitempty"`
	StaleAfter         string            `yaml:"stale_after,omitempty"`
	Sources            []ClaimSource     `yaml:"sources,omitempty"`
	EvidenceIDs        []string          `yaml:"evidence_ids,omitempty"`
	SupportingClaimIDs []string          `yaml:"supporting_claim_ids,omitempty"`
	Supersedes         []string          `yaml:"supersedes,omitempty"`
	ConflictsWith      []string          `yaml:"conflicts_with,omitempty"`
	Tags               []string          `yaml:"tags,omitempty"`
	Transitions        []ClaimTransition `yaml:"transitions,omitempty"`
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
	frontmatter, body, err := splitMarkdownFrontmatter(contents)
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

	var probe struct {
		Schema string `yaml:"schema"`
		Type   string `yaml:"type"`
	}
	if err := yaml.Unmarshal(frontmatter, &probe); err != nil {
		return Claim{}, err
	}

	var claim Claim
	switch {
	case probe.Schema == ClaimSchemaVersion:
		claim, err = parseLegacyClaimFrontmatter(frontmatter, tier, relPath, body)
	case probe.Type == OKFClaimType:
		claim, err = parseOKFClaimFrontmatter(frontmatter, tier, relPath, body)
	default:
		return Claim{}, fmt.Errorf("claim type must be %q or legacy schema %q", OKFClaimType, ClaimSchemaVersion)
	}
	if err != nil {
		return Claim{}, err
	}
	if err := validateClaimPathID(relPath, claim.ID); err != nil {
		return Claim{}, err
	}
	return claim, nil
}

func RenderClaimMarkdown(claim Claim) ([]byte, error) {
	if err := ValidateClaim(claim); err != nil {
		return nil, err
	}
	metadata := claimFrontmatter{
		Type:        OKFClaimType,
		Title:       claim.Title,
		Description: claim.Description,
		Resource:    claim.Resource,
		Tags:        append([]string(nil), claim.Tags...),
		Sources:     append([]ClaimSource(nil), claim.Sources...),
		Status:      claim.Status,
		StaleAfter:  claim.StaleAfter,
		Zbrain: zbrainProfile{
			Profile:            ZbrainTrustedMemoryProfile,
			ID:                 claim.ID,
			Tier:               claim.Tier,
			Basis:              claim.Basis,
			EvidenceIDs:        append([]string(nil), claim.EvidenceIDs...),
			SupportingClaimIDs: append([]string(nil), claim.SupportingClaimIDs...),
			Supersedes:         append([]string(nil), claim.Supersedes...),
			ConflictsWith:      append([]string(nil), claim.ConflictsWith...),
			Transitions:        append([]ClaimTransition(nil), claim.Transitions...),
		},
	}
	if claim.CreatedAt != "" || claim.CreatedBy != "" {
		metadata.Generated = &ClaimGenerated{At: claim.CreatedAt, By: claim.CreatedBy}
	}
	if claim.VerifiedAt != "" || claim.VerifiedBy != "" || claim.VerifiedDigest != "" {
		metadata.Verified = &ClaimVerified{At: claim.VerifiedAt, By: claim.VerifiedBy, Digest: claim.VerifiedDigest}
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
	if claim.Schema != "" && claim.Schema != ClaimSchemaVersion {
		return fmt.Errorf("claim schema must be %q when present", ClaimSchemaVersion)
	}
	if claim.Type != "" && claim.Type != OKFClaimType {
		return fmt.Errorf("claim type must be %q", OKFClaimType)
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
	for _, source := range claim.Sources {
		for _, span := range source.Spans {
			if !evidenceIDPattern.MatchString(span.EvidenceID) || span.StartLine < 1 || span.EndLine < span.StartLine {
				return fmt.Errorf("claim evidence span has invalid evidence or line range")
			}
			if !strings.HasPrefix(span.Digest, "sha256:span-v1:") {
				return fmt.Errorf("claim evidence span digest must use sha256:span-v1:<hex>")
			}
		}
	}
	if _, err := time.Parse(time.RFC3339, claim.CreatedAt); err != nil {
		return fmt.Errorf("claim generated.at must be RFC3339: %w", err)
	}
	if strings.TrimSpace(claim.CreatedBy) == "" {
		return fmt.Errorf("claim generated.by is required")
	}
	if claim.VerifiedAt != "" || claim.VerifiedBy != "" || claim.VerifiedDigest != "" {
		if _, err := time.Parse(time.RFC3339, claim.VerifiedAt); err != nil {
			return fmt.Errorf("claim verified.at must be RFC3339: %w", err)
		}
		if strings.TrimSpace(claim.VerifiedBy) == "" {
			return fmt.Errorf("claim verified.by is required")
		}
		if !strings.HasPrefix(claim.VerifiedDigest, "sha256:") {
			return fmt.Errorf("claim verified.digest must use sha256:<hex>")
		}
	}
	if claim.StaleAfter != "" {
		if _, err := time.Parse(time.RFC3339, claim.StaleAfter); err != nil {
			return fmt.Errorf("claim stale_after must be RFC3339: %w", err)
		}
	}
	for _, id := range claim.EvidenceIDs {
		if !evidenceIDPattern.MatchString(id) {
			return fmt.Errorf("evidence id %q must match evd_<32 lowercase hex chars>", id)
		}
	}
	for _, source := range claim.Sources {
		if source.ID != "" && !evidenceIDPattern.MatchString(source.ID) {
			return fmt.Errorf("source id %q must match evd_<32 lowercase hex chars>", source.ID)
		}
		if source.Digest != "" && !strings.HasPrefix(source.Digest, "sha256:") {
			return fmt.Errorf("source digest must use sha256:<hex>")
		}
	}
	for _, ids := range [][]string{claim.SupportingClaimIDs, claim.Supersedes, claim.ConflictsWith} {
		for _, id := range ids {
			if !claimIDPattern.MatchString(id) {
				return fmt.Errorf("related claim id %q must match clm_<32 lowercase hex chars>", id)
			}
		}
	}
	if err := ValidateClaimTransitions(claim.Transitions); err != nil {
		return err
	}
	return nil
}

func ValidateClaimTransitions(transitions []ClaimTransition) error {
	for index, transition := range transitions {
		if err := ValidateClaimTransition(transition); err != nil {
			return fmt.Errorf("claim transition %d: %w", index, err)
		}
	}
	return nil
}

func ValidateClaimTransition(transition ClaimTransition) error {
	switch transition.Kind {
	case ClaimTransitionApprove, ClaimTransitionSupersede, ClaimTransitionRevoke:
	default:
		return fmt.Errorf("claim transition kind %q is not supported", transition.Kind)
	}
	if _, err := time.Parse(time.RFC3339, transition.At); err != nil {
		return fmt.Errorf("claim transition at must be RFC3339: %w", err)
	}
	if strings.TrimSpace(transition.By) == "" {
		return fmt.Errorf("claim transition by is required")
	}
	if transition.PriorVerificationDigest != "" && !strings.HasPrefix(transition.PriorVerificationDigest, "sha256:") {
		return fmt.Errorf("claim transition prior_verification_digest must use sha256:<hex>")
	}
	seen := make(map[string]struct{}, len(transition.RelatedClaimIDs))
	for _, id := range transition.RelatedClaimIDs {
		if !claimIDPattern.MatchString(id) {
			return fmt.Errorf("claim transition related claim id %q must match clm_<32 lowercase hex chars>", id)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("claim transition related claim id %q is duplicated", id)
		}
		seen[id] = struct{}{}
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

func ClaimVerificationDigest(claim Claim) (string, error) {
	unsigned := claim
	unsigned.VerifiedAt = ""
	unsigned.VerifiedBy = ""
	unsigned.VerifiedDigest = ""
	rendered, err := RenderClaimMarkdown(unsigned)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(rendered)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func VerifyClaimDigest(claim Claim) error {
	if claim.Status != ClaimStatusApproved {
		return nil
	}
	if strings.TrimSpace(claim.VerifiedDigest) == "" {
		return fmt.Errorf("approved claim is missing verification digest")
	}
	digest, err := ClaimVerificationDigest(claim)
	if err != nil {
		return fmt.Errorf("compute verification digest: %w", err)
	}
	if digest != claim.VerifiedDigest {
		return fmt.Errorf("verification digest mismatch")
	}
	return nil
}

func splitMarkdownFrontmatter(contents []byte) ([]byte, []byte, error) {
	if !bytes.HasPrefix(contents, []byte("---\n")) {
		return nil, nil, fmt.Errorf("claim frontmatter is required")
	}
	closing := bytes.Index(contents[4:], []byte("\n---\n"))
	if closing == -1 {
		return nil, nil, fmt.Errorf("claim frontmatter closing marker is required")
	}
	frontmatter := contents[4 : 4+closing]
	body := contents[4+closing+5:]
	return frontmatter, body, nil
}

func parseLegacyClaimFrontmatter(frontmatter []byte, tier string, relPath string, body []byte) (Claim, error) {
	var metadata legacyClaimFrontmatter
	if err := yaml.Unmarshal(frontmatter, &metadata); err != nil {
		return Claim{}, err
	}
	var verifiedAt, verifiedBy, verifiedDigest string
	if metadata.Verified != nil {
		verifiedAt = metadata.Verified.At
		verifiedBy = metadata.Verified.By
		verifiedDigest = metadata.Verified.Digest
	}
	claim := Claim{
		Schema:             metadata.Schema,
		Type:               OKFClaimType,
		ID:                 metadata.ID,
		Tier:               tier,
		Path:               filepath.ToSlash(relPath),
		Status:             metadata.Status,
		Title:              metadata.Title,
		Description:        metadata.Description,
		Resource:           metadata.Resource,
		Basis:              metadata.Basis,
		CreatedAt:          metadata.CreatedAt,
		CreatedBy:          metadata.CreatedBy,
		VerifiedAt:         verifiedAt,
		VerifiedBy:         verifiedBy,
		VerifiedDigest:     verifiedDigest,
		StaleAfter:         metadata.StaleAfter,
		Sources:            append([]ClaimSource(nil), metadata.Sources...),
		EvidenceIDs:        append([]string(nil), metadata.EvidenceIDs...),
		SupportingClaimIDs: append([]string(nil), metadata.SupportingClaimIDs...),
		Supersedes:         append([]string(nil), metadata.Supersedes...),
		ConflictsWith:      append([]string(nil), metadata.ConflictsWith...),
		Tags:               append([]string(nil), metadata.Tags...),
		Transitions:        append([]ClaimTransition(nil), metadata.Transitions...),
		Body:               string(body),
	}
	if err := ValidateClaim(claim); err != nil {
		if claim.Status != ClaimStatusApproved || !strings.Contains(err.Error(), "claim verified.digest") {
			return Claim{}, err
		}
		unsigned := claim
		unsigned.VerifiedAt = ""
		unsigned.VerifiedBy = ""
		unsigned.VerifiedDigest = ""
		if unsignedErr := ValidateClaim(unsigned); unsignedErr != nil {
			return Claim{}, err
		}
	}
	return claim, nil
}

func parseOKFClaimFrontmatter(frontmatter []byte, tier string, relPath string, body []byte) (Claim, error) {
	var metadata claimFrontmatter
	if err := yaml.Unmarshal(frontmatter, &metadata); err != nil {
		return Claim{}, err
	}
	claim := Claim{
		Type:               metadata.Type,
		ID:                 metadata.Zbrain.ID,
		Tier:               metadata.Zbrain.Tier,
		Path:               filepath.ToSlash(relPath),
		Status:             metadata.Status,
		Title:              metadata.Title,
		Description:        metadata.Description,
		Resource:           metadata.Resource,
		Basis:              metadata.Zbrain.Basis,
		StaleAfter:         metadata.StaleAfter,
		Sources:            append([]ClaimSource(nil), metadata.Sources...),
		EvidenceIDs:        append([]string(nil), metadata.Zbrain.EvidenceIDs...),
		SupportingClaimIDs: append([]string(nil), metadata.Zbrain.SupportingClaimIDs...),
		Supersedes:         append([]string(nil), metadata.Zbrain.Supersedes...),
		ConflictsWith:      append([]string(nil), metadata.Zbrain.ConflictsWith...),
		Tags:               append([]string(nil), metadata.Tags...),
		Transitions:        append([]ClaimTransition(nil), metadata.Zbrain.Transitions...),
		Body:               string(body),
	}
	if claim.Tier == "" {
		claim.Tier = tier
	}
	if metadata.Generated != nil {
		claim.CreatedAt = metadata.Generated.At
		claim.CreatedBy = metadata.Generated.By
	}
	if metadata.Verified != nil {
		claim.VerifiedAt = metadata.Verified.At
		claim.VerifiedBy = metadata.Verified.By
		claim.VerifiedDigest = metadata.Verified.Digest
	}
	if metadata.Zbrain.Profile != ZbrainTrustedMemoryProfile {
		return Claim{}, fmt.Errorf("zbrain profile must be %q", ZbrainTrustedMemoryProfile)
	}
	if claim.Tier != tier {
		return Claim{}, fmt.Errorf("claim zbrain tier %q does not match path tier %q", claim.Tier, tier)
	}
	if err := ValidateClaim(claim); err != nil {
		return Claim{}, err
	}
	return claim, nil
}

func validateClaimPathID(relPath string, id string) error {
	base := strings.TrimSuffix(filepath.Base(filepath.ToSlash(relPath)), ".md")
	if claimIDPattern.MatchString(base) && base != id {
		return fmt.Errorf("claim path id %q does not match frontmatter id %q", base, id)
	}
	return nil
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
