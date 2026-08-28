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
	"unicode"

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
	Contradicts        []Contradiction
	Tags               []string
	Transitions        []ClaimTransition
	Body               string
}

type ContradictionHeuristic string

const (
	ContradictionNegation     ContradictionHeuristic = "negation"
	ContradictionValueSwap    ContradictionHeuristic = "value_swap"
	ContradictionStatusChange ContradictionHeuristic = "status_change"
)

// Contradiction records one advisory, rule-based contradiction hit between a
// draft and an approved claim. It never changes claim status, blocks the
// draft, or alters lifecycle promotion.
type Contradiction struct {
	ClaimID   string                 `yaml:"claim_id" json:"claim_id"`
	Heuristic ContradictionHeuristic `yaml:"heuristic" json:"heuristic"`
}

type ClaimTransitionKind string

const (
	ClaimTransitionApprove   ClaimTransitionKind = "approve"
	ClaimTransitionSupersede ClaimTransitionKind = "supersede"
	ClaimTransitionRevoke    ClaimTransitionKind = "revoke"
)

// ClaimTransitionAuthorization records the owner-pinned authorization
// metadata for a transition performed by a later caller such as MCP. It is
// optional so existing owner-authored YAML and JSON remain byte-compatible.
type ClaimTransitionAuthorization struct {
	ChallengeID string `yaml:"challenge_id,omitempty" json:"challenge_id,omitempty"`
	Method      string `yaml:"method,omitempty" json:"method,omitempty"`
	MCPClient   string `yaml:"mcp_client,omitempty" json:"mcp_client,omitempty"`
}

// TransitionAuthorization is kept as a concise alias for callers that model
// the metadata independently from ClaimTransition.
type TransitionAuthorization = ClaimTransitionAuthorization

type ClaimTransition struct {
	Kind                    ClaimTransitionKind           `yaml:"kind" json:"kind"`
	At                      string                        `yaml:"at" json:"at"`
	By                      string                        `yaml:"by" json:"by"`
	Reason                  string                        `yaml:"reason,omitempty" json:"reason,omitempty"`
	RelatedClaimIDs         []string                      `yaml:"related_claim_ids,omitempty" json:"related_claim_ids,omitempty"`
	PriorVerificationDigest string                        `yaml:"prior_verification_digest,omitempty" json:"prior_verification_digest,omitempty"`
	Authorization           *ClaimTransitionAuthorization `yaml:"authorization,omitempty" json:"authorization,omitempty"`
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
	Contradicts        []Contradiction   `yaml:"contradicts,omitempty"`
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
			Contradicts:        append([]Contradiction(nil), claim.Contradicts...),
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
	for _, contradiction := range claim.Contradicts {
		if !claimIDPattern.MatchString(contradiction.ClaimID) {
			return fmt.Errorf("contradiction claim id %q must match clm_<32 lowercase hex chars>", contradiction.ClaimID)
		}
		switch contradiction.Heuristic {
		case ContradictionNegation, ContradictionValueSwap, ContradictionStatusChange:
		default:
			return fmt.Errorf("contradiction heuristic %q is not supported", contradiction.Heuristic)
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
	if err := ValidateClaimTransitionAuthorization(transition.Authorization); err != nil {
		return err
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

// ValidateClaimTransitionAuthorization validates optional authorization
// metadata without changing the four-state claim lifecycle contract.
func ValidateClaimTransitionAuthorization(authorization *ClaimTransitionAuthorization) error {
	if authorization == nil {
		return nil
	}
	if !challengeIDPattern.MatchString(authorization.ChallengeID) {
		return fmt.Errorf("claim transition authorization challenge id must match chg_<32 lowercase hex chars>")
	}
	if strings.TrimSpace(authorization.Method) == "" {
		return fmt.Errorf("claim transition authorization method is required")
	}
	if strings.TrimSpace(authorization.MCPClient) == "" {
		return fmt.Errorf("claim transition authorization MCP client is required")
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

// ClaimCanonicalDigest hashes the canonical serialized claim bytes used by a
// challenge. Callers should compute it from the current claim immediately
// before preparing a challenge and compare it again before applying one.
func ClaimCanonicalDigest(claim Claim) (string, error) {
	rendered, err := RenderClaimMarkdown(claim)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(rendered)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
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
		Contradicts:        append([]Contradiction(nil), metadata.Zbrain.Contradicts...),
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

// DetectContradictions applies deterministic, rule-based heuristics between a
// draft and approved claims. It only compares normalized text (lowercase,
// whitespace-collapsed, punctuation-stripped) of matching fields — title to
// title and body to body — and never touches the index, network, or a model.
// Hits are advisory: callers record them as draft metadata.
func DetectContradictions(draft Claim, approvedClaims []Claim) []Contradiction {
	contradictions := make([]Contradiction, 0)
	seen := make(map[string]bool)
	for _, approved := range approvedClaims {
		if approved.ID == draft.ID {
			continue
		}
		heuristics := make(map[ContradictionHeuristic]bool)
		for _, pair := range [][2]string{
			{draft.Title, approved.Title},
			{draft.Body, approved.Body},
		} {
			if heuristic, ok := classifyContradiction(pair[0], pair[1]); ok {
				heuristics[heuristic] = true
			}
		}
		for _, heuristic := range []ContradictionHeuristic{ContradictionStatusChange, ContradictionNegation, ContradictionValueSwap} {
			if !heuristics[heuristic] || seen[approved.ID+"/"+string(heuristic)] {
				continue
			}
			seen[approved.ID+"/"+string(heuristic)] = true
			contradictions = append(contradictions, Contradiction{ClaimID: approved.ID, Heuristic: heuristic})
		}
	}
	return contradictions
}

// classifyContradiction reports the most specific heuristic firing between a
// draft text and an approved text, if any. Status change outranks negation,
// which outranks value swap, so "X is deprecated" vs "X is recommended"
// records status_change rather than the weaker value_swap.
func classifyContradiction(draftText string, approvedText string) (ContradictionHeuristic, bool) {
	if detectStatusChange(draftText, approvedText) {
		return ContradictionStatusChange, true
	}
	if detectNegationFlip(draftText, approvedText) {
		return ContradictionNegation, true
	}
	if detectValueSwap(draftText, approvedText) {
		return ContradictionValueSwap, true
	}
	return "", false
}

var contradictionNegators = map[string]bool{
	"not": true, "no": true, "never": true, "cannot": true, "cant": true,
	"dont": true, "doesnt": true, "isnt": true, "arent": true, "wont": true,
	"without": true,
}

var contradictionPredicates = map[string]bool{
	"is": true, "are": true, "uses": true, "requires": true, "supports": true,
	"has": true, "equals": true, "defaults": true,
}

var contradictionStatusWords = map[string]bool{
	"deprecated": true, "obsolete": true, "active": true, "current": true,
	"recommended": true, "rejected": true, "approved": true, "stable": true,
	"experimental": true, "required": true, "optional": true, "enabled": true,
	"disabled": true,
}

func contradictionTokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func equalTokenSlices(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

var contradictionAuxiliaries = map[string]bool{
	"do": true, "does": true, "did": true, "can": true, "will": true,
	"would": true, "may": true, "might": true, "with": true, "without": true,
	"also": true, "even": true, "still": true,
}

// stemContradictionToken collapses a trailing plural "s" so "supports" and
// "support" compare equal. Claims keep the crudeness deliberately: the
// heuristics are tuned only through tests.
func stemContradictionToken(token string) string {
	if len(token) > 3 && strings.HasSuffix(token, "s") && !strings.HasSuffix(token, "ss") {
		return token[:len(token)-1]
	}
	return token
}

// stripNegators removes negator tokens, reporting whether any were present.
func stripNegators(tokens []string) ([]string, bool) {
	stripped := make([]string, 0, len(tokens))
	found := false
	for _, token := range tokens {
		if contradictionNegators[token] {
			found = true
			continue
		}
		stripped = append(stripped, token)
	}
	return stripped, found
}

// normalizeContradictionCore strips negator and auxiliary tokens and stems
// plurals, returning the comparable core plus whether the text was negated.
func normalizeContradictionCore(tokens []string) ([]string, bool) {
	stripped, negated := stripNegators(tokens)
	core := make([]string, 0, len(stripped))
	for _, token := range stripped {
		if contradictionAuxiliaries[token] {
			continue
		}
		core = append(core, stemContradictionToken(token))
	}
	if len(core) == 0 {
		return nil, negated
	}
	return core, negated
}

// detectNegationFlip fires when the two texts share the same core tokens but
// exactly one side carries a negator: "zbrain is not deployed on edge nodes"
// vs "zbrain is deployed on edge nodes", or "runs without network" vs "runs
// with network".
func detectNegationFlip(draftText string, approvedText string) bool {
	draftTokens := contradictionTokens(draftText)
	approvedTokens := contradictionTokens(approvedText)
	if len(draftTokens) == 0 || len(approvedTokens) == 0 {
		return false
	}
	draftCore, draftNegated := normalizeContradictionCore(draftTokens)
	approvedCore, approvedNegated := normalizeContradictionCore(approvedTokens)
	if draftNegated == approvedNegated {
		return false
	}
	if draftCore == nil || approvedCore == nil {
		return false
	}
	return equalTokenSlices(draftCore, approvedCore)
}

// splitContradictionClause extracts "<subject> <verb> <object>" from the
// first predicate token. Clauses whose object starts with a negator are
// rejected so negation flips stay in the negation heuristic.
type contradictionClause struct {
	subject []string
	verb    string
	object  []string
}

func splitContradictionClause(text string) (contradictionClause, bool) {
	tokens := contradictionTokens(text)
	for index, token := range tokens {
		if !contradictionPredicates[token] {
			continue
		}
		subject := tokens[:index]
		object := tokens[index+1:]
		if len(subject) == 0 || len(object) == 0 || contradictionNegators[object[0]] {
			return contradictionClause{}, false
		}
		return contradictionClause{subject: subject, verb: token, object: object}, true
	}
	return contradictionClause{}, false
}

// detectValueSwap fires when both texts assert the same subject with the
// same predicate but a different exact object: "X uses Postgres 15" vs
// "X uses Postgres 16".
func detectValueSwap(draftText string, approvedText string) bool {
	draftClause, ok := splitContradictionClause(draftText)
	if !ok {
		return false
	}
	approvedClause, ok := splitContradictionClause(approvedText)
	if !ok {
		return false
	}
	if draftClause.verb != approvedClause.verb {
		return false
	}
	if !equalTokenSlices(draftClause.subject, approvedClause.subject) {
		return false
	}
	return !equalTokenSlices(draftClause.object, approvedClause.object)
}

// detectStatusChange fires when both texts assert the same subject with a
// single-word status object and the status words differ: "X is deprecated"
// vs "X is recommended".
func detectStatusChange(draftText string, approvedText string) bool {
	draftClause, ok := splitContradictionClause(draftText)
	if !ok {
		return false
	}
	approvedClause, ok := splitContradictionClause(approvedText)
	if !ok {
		return false
	}
	if len(draftClause.object) != 1 || len(approvedClause.object) != 1 {
		return false
	}
	draftStatus := draftClause.object[0]
	approvedStatus := approvedClause.object[0]
	if !contradictionStatusWords[draftStatus] || !contradictionStatusWords[approvedStatus] {
		return false
	}
	return equalTokenSlices(draftClause.subject, approvedClause.subject) && draftStatus != approvedStatus
}
