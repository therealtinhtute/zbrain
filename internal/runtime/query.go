package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type QueryStatus string

const (
	QueryStatusReady   QueryStatus = "ready"
	QueryStatusGap     QueryStatus = "gap"
	QueryStatusBlocked QueryStatus = "blocked"
)

type QueryScopeOptions struct {
	Workspace string
	Includes  []string
}

type QueryScopes struct {
	Primary  string   `json:"primary"`
	Includes []string `json:"includes"`
}

type TrustedQueryOptions struct {
	Workspace string
	Includes  []string
	Query     string
	Limit     int
}

type TrustedQueryResponse struct {
	SchemaVersion       int                  `json:"schema_version"`
	Status              QueryStatus          `json:"status"`
	Query               string               `json:"query"`
	Scopes              QueryScopes          `json:"scopes"`
	Claims              []QueryClaim         `json:"claims"`
	Conflicts           []QueryConflict      `json:"conflicts"`
	Gaps                []QueryGap           `json:"gaps"`
	PromotionCandidates []QueryClaim         `json:"promotion_candidates"`
	Index               []QueryIndexMetadata `json:"index"`
}

type QueryClaim struct {
	Workspace   string      `json:"workspace"`
	ID          string      `json:"id"`
	Path        string      `json:"path"`
	Tier        string      `json:"tier"`
	Type        string      `json:"type"`
	Status      ClaimStatus `json:"status"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	StaleAfter  string      `json:"stale_after,omitempty"`
	Score       float64     `json:"score"`
}

type QueryConflict struct {
	Workspace string   `json:"workspace"`
	ClaimIDs  []string `json:"claim_ids"`
}

type QueryGap struct {
	Workspace string `json:"workspace"`
	Reason    string `json:"reason"`
}

type QueryIndexMetadata struct {
	Workspace string `json:"workspace"`
	Fresh     bool   `json:"fresh"`
}

func ResolveQueryScopes(paths Paths, options QueryScopeOptions) (QueryScopes, error) {
	primary := options.Workspace
	if primary == "" {
		current, err := ResolveCurrentWorkspace(paths)
		if err != nil {
			return QueryScopes{}, err
		}
		primary = current.Workspace
	}
	if err := validateWorkspaceExists(paths, primary); err != nil {
		return QueryScopes{}, err
	}
	seen := map[string]bool{primary: true}
	includes := []string{}
	for _, include := range options.Includes {
		if seen[include] {
			continue
		}
		if err := validateWorkspaceExists(paths, include); err != nil {
			return QueryScopes{}, err
		}
		seen[include] = true
		includes = append(includes, include)
	}
	return QueryScopes{Primary: primary, Includes: includes}, nil
}

func TrustedQuery(paths Paths, options TrustedQueryOptions) (TrustedQueryResponse, error) {
	scopes, err := ResolveQueryScopes(paths, QueryScopeOptions{Workspace: options.Workspace, Includes: options.Includes})
	if err != nil {
		return TrustedQueryResponse{}, err
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 10
	}
	response := TrustedQueryResponse{
		SchemaVersion: 1,
		Status:        QueryStatusReady,
		Query:         options.Query,
		Scopes:        scopes,
	}
	idx := IndexStore{Paths: paths}
	workspaces := append([]string{scopes.Primary}, scopes.Includes...)
	lockWorkspaces := append([]string(nil), workspaces...)
	sort.Strings(lockWorkspaces)
	locks := make([]*workspaceLock, 0, len(lockWorkspaces))
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			_ = locks[i].Close()
		}
	}()
	for _, workspace := range lockWorkspaces {
		lock, err := acquireWorkspaceLock(paths, workspace, false)
		if err != nil {
			return TrustedQueryResponse{}, err
		}
		locks = append(locks, lock)
	}
	runWorkspaceGenerationTestHook(workspaceGenerationHookTrustedQueryAfterLocking)
	manifests := make(map[string]TrustInputManifest, len(workspaces))
	for _, workspace := range workspaces {
		var manifest TrustInputManifest
		if err := idx.checkFreshUnlockedOutput(workspace, &manifest); err != nil {
			return TrustedQueryResponse{}, err
		}
		manifests[workspace] = manifest
		response.Index = append(response.Index, QueryIndexMetadata{Workspace: workspace, Fresh: true})
	}
	for _, workspace := range workspaces {
		manifest := manifests[workspace]
		approved, err := idx.searchUnlockedInternal(workspace, SearchOptions{Query: options.Query, Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: limit}, false, true)
		if err != nil {
			return TrustedQueryResponse{}, err
		}
		for _, claim := range approved {
			if err := validateIndexedClaimBindingInternal(paths, workspace, claim, &manifest, nil, nil, false); err != nil {
				return TrustedQueryResponse{}, err
			}
			response.Claims = append(response.Claims, toQueryClaim(workspace, claim))
		}
		drafts, err := idx.searchUnlockedInternal(workspace, SearchOptions{Query: options.Query, Statuses: []ClaimStatus{ClaimStatusDraft}, Limit: limit}, false, false)
		if err != nil {
			return TrustedQueryResponse{}, err
		}
		for _, claim := range drafts {
			if err := validateIndexedClaimBindingInternal(paths, workspace, claim, &manifest, nil, nil, false); err != nil {
				return TrustedQueryResponse{}, err
			}
			response.PromotionCandidates = append(response.PromotionCandidates, toQueryClaim(workspace, claim))
		}
	}
	sortQueryClaims(response.Claims)
	sortQueryClaims(response.PromotionCandidates)
	conflicts, err := findQueryConflicts(paths, response.Claims)
	if err != nil {
		return TrustedQueryResponse{}, err
	}
	response.Conflicts = conflicts
	if len(conflicts) > 0 {
		response.Status = QueryStatusBlocked
		return response, nil
	}
	if len(response.Claims) == 0 {
		response.Status = QueryStatusGap
		response.Gaps = []QueryGap{{Workspace: scopes.Primary, Reason: "no approved claims matched the query in resolved scopes"}}
		return response, nil
	}
	return response, nil
}

func validateIndexedClaimBinding(paths Paths, workspace string, indexed IndexedClaim, manifest *TrustInputManifest, evidenceValidator *EvidenceValidator, trustValidator *TrustValidator) error {
	return validateIndexedClaimBindingInternal(paths, workspace, indexed, manifest, evidenceValidator, trustValidator, true)
}

func validateIndexedClaimBindingInternal(paths Paths, workspace string, indexed IndexedClaim, manifest *TrustInputManifest, evidenceValidator *EvidenceValidator, trustValidator *TrustValidator, checkCanonicalSet bool) error {
	canonicalRelative := filepath.ToSlash(filepath.Join("wiki", filepath.FromSlash(indexed.Path)))
	canonicalPath, err := ResolveWorkspacePath(paths, workspace, canonicalRelative)
	if err != nil {
		return fmt.Errorf("load canonical claim %q: %w", indexed.ID, err)
	}
	contents, err := os.ReadFile(canonicalPath)
	if err != nil {
		return fmt.Errorf("read canonical claim %q: %w", indexed.ID, err)
	}
	canonical, err := ParseClaimMarkdown(indexed.Tier, indexed.Path, contents)
	if err != nil {
		return fmt.Errorf("parse canonical claim %q: %w", indexed.ID, err)
	}
	if manifest != nil {
		canonicalPaths := canonicalClaimPathsFromManifest(*manifest, indexed.ID)
		if len(canonicalPaths) > 1 {
			return duplicateCanonicalClaimPathsError(indexed.ID, canonicalPaths)
		}
		if checkCanonicalSet {
			canonicalClaims, err := (ClaimStore{Paths: paths}).readCanonicalClaimsByID(workspace, indexed.ID)
			if err != nil {
				return fmt.Errorf("load canonical claim set %q: %w", indexed.ID, err)
			}
			if len(canonicalClaims) > 1 {
				return duplicateCanonicalClaimError(indexed.ID, canonicalClaims)
			}
		}
	} else {
		canonicalClaims, err := (ClaimStore{Paths: paths}).readCanonicalClaimsByID(workspace, indexed.ID)
		if err != nil {
			return fmt.Errorf("load canonical claim set %q: %w", indexed.ID, err)
		}
		if len(canonicalClaims) > 1 {
			return duplicateCanonicalClaimError(indexed.ID, canonicalClaims)
		}
	}
	if manifest != nil && !trustInputManifestContainsClaim(*manifest, canonicalRelative) {
		return fmt.Errorf("indexed claim %q is not present in published trust manifest; run zbrain reindex", indexed.ID)
	}

	fields := []struct {
		name      string
		canonical string
		indexed   string
	}{
		{name: "id", canonical: canonical.ID, indexed: indexed.ID},
		{name: "path", canonical: canonical.Path, indexed: indexed.Path},
		{name: "tier", canonical: canonical.Tier, indexed: indexed.Tier},
		{name: "type", canonical: canonical.Type, indexed: indexed.Type},
		{name: "status", canonical: string(canonical.Status), indexed: string(indexed.Status)},
		{name: "title", canonical: canonical.Title, indexed: indexed.Title},
		{name: "description", canonical: canonical.Description, indexed: indexed.Description},
		{name: "stale_after", canonical: canonical.StaleAfter, indexed: indexed.StaleAfter},
		{name: "tags", canonical: strings.Join(canonical.Tags, " "), indexed: indexed.indexedTags},
		{name: "body", canonical: canonical.Body, indexed: indexed.indexedBody},
		{name: "verification_digest", canonical: canonical.VerifiedDigest, indexed: indexed.verificationDigest},
	}
	for _, field := range fields {
		if field.canonical != field.indexed {
			return fmt.Errorf("indexed claim %q %s does not match canonical claim", indexed.ID, field.name)
		}
	}
	if canonical.Status != ClaimStatusApproved {
		return nil
	}
	if err := VerifyClaimDigest(canonical); err != nil {
		return fmt.Errorf("approved claim %q verification failed: %w", indexed.ID, err)
	}
	if len(canonical.EvidenceIDs) == 0 && len(canonical.SupportingClaimIDs) == 0 {
		return nil
	}
	if evidenceValidator == nil {
		var err error
		evidenceValidator, err = NewEvidenceValidator(EvidenceStore{Paths: paths}, workspace)
		if err != nil {
			return fmt.Errorf("approved claim %q evidence validator is unavailable: %w", indexed.ID, err)
		}
	}
	if len(canonical.SupportingClaimIDs) > 0 && trustValidator == nil {
		var err error
		trustValidator, err = NewTrustValidatorFromStore(ClaimStore{Paths: paths}, workspace)
		if err != nil {
			return fmt.Errorf("approved claim %q trust validator is unavailable: %w", indexed.ID, err)
		}
		trustValidator.validateSupporting = func(support Claim) error {
			return validateClaimEvidence(evidenceValidator, support)
		}
	}
	if len(canonical.EvidenceIDs) > 0 {
		if evidenceValidator == nil {
			return fmt.Errorf("approved claim %q evidence validator is unavailable", indexed.ID)
		}
		if err := validateClaimEvidence(evidenceValidator, canonical); err != nil {
			return fmt.Errorf("approved claim %q evidence validation failed: %w", indexed.ID, err)
		}
	}
	if len(canonical.SupportingClaimIDs) > 0 {
		if trustValidator == nil {
			return fmt.Errorf("approved claim %q trust validator is unavailable", indexed.ID)
		}
		if err := trustValidator.ValidateClaim(canonical); err != nil {
			return fmt.Errorf("approved claim %q supporting-claim validation failed: %w", indexed.ID, err)
		}
	}
	return nil
}

func canonicalClaimPathsFromManifest(manifest TrustInputManifest, id string) []string {
	suffix := "/" + id + ".md"
	paths := make([]string, 0)
	for _, entry := range manifest.Entries {
		if entry.Kind != TrustInputKindClaim || !strings.HasPrefix(entry.Path, "wiki/") || !strings.HasSuffix(entry.Path, suffix) {
			continue
		}
		paths = append(paths, strings.TrimPrefix(entry.Path, "wiki/"))
	}
	sort.Strings(paths)
	return paths
}

func trustInputManifestContainsClaim(manifest TrustInputManifest, path string) bool {
	index := sort.Search(len(manifest.Entries), func(index int) bool {
		return manifest.Entries[index].Path >= path
	})
	return index < len(manifest.Entries) &&
		manifest.Entries[index].Path == path &&
		manifest.Entries[index].Kind == TrustInputKindClaim
}

func toQueryClaim(workspace string, claim IndexedClaim) QueryClaim {
	return QueryClaim{
		Workspace:   workspace,
		ID:          claim.ID,
		Path:        claim.Path,
		Tier:        claim.Tier,
		Type:        claim.Type,
		Status:      claim.Status,
		Title:       claim.Title,
		Description: claim.Description,
		StaleAfter:  claim.StaleAfter,
		Score:       claim.Score,
	}
}

func sortQueryClaims(claims []QueryClaim) {
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].Score != claims[j].Score {
			return claims[i].Score < claims[j].Score
		}
		if claims[i].Workspace != claims[j].Workspace {
			return claims[i].Workspace < claims[j].Workspace
		}
		return claims[i].Path < claims[j].Path
	})
}

func findQueryConflicts(paths Paths, claims []QueryClaim) ([]QueryConflict, error) {
	byWorkspace := map[string]map[string]bool{}
	for _, claim := range claims {
		if byWorkspace[claim.Workspace] == nil {
			byWorkspace[claim.Workspace] = map[string]bool{}
		}
		byWorkspace[claim.Workspace][claim.ID] = true
	}
	store := ClaimStore{Paths: paths}
	conflicts := []QueryConflict{}
	seen := map[string]bool{}
	for _, queryClaim := range claims {
		claim, err := store.readClaimPath(queryClaim.Workspace, queryClaim.Path)
		if err != nil {
			return nil, err
		}
		if claim.ID != queryClaim.ID {
			return nil, fmt.Errorf("query claim %q canonical path %q contains claim %q", queryClaim.ID, queryClaim.Path, claim.ID)
		}
		for _, other := range claim.ConflictsWith {
			if !byWorkspace[queryClaim.Workspace][other] {
				continue
			}
			ids := []string{queryClaim.ID, other}
			sort.Strings(ids)
			key := queryClaim.Workspace + ":" + ids[0] + ":" + ids[1]
			if seen[key] {
				continue
			}
			seen[key] = true
			conflicts = append(conflicts, QueryConflict{Workspace: queryClaim.Workspace, ClaimIDs: ids})
		}
	}
	return conflicts, nil
}

func validateWorkspaceExists(paths Paths, workspace string) error {
	_, err := ValidateWorkspace(paths, workspace)
	return err
}
