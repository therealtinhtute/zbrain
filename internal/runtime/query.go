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
	for _, workspace := range workspaces {
		if err := idx.CheckFresh(workspace); err != nil {
			return TrustedQueryResponse{}, err
		}
		response.Index = append(response.Index, QueryIndexMetadata{Workspace: workspace, Fresh: true})
	}
	for _, workspace := range workspaces {
		approved, err := idx.Search(workspace, SearchOptions{Query: options.Query, Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: limit})
		if err != nil {
			return TrustedQueryResponse{}, err
		}
		for _, claim := range approved {
			if err := validateIndexedClaimBinding(paths, workspace, claim); err != nil {
				return TrustedQueryResponse{}, err
			}
			response.Claims = append(response.Claims, toQueryClaim(workspace, claim))
		}
		drafts, err := idx.Search(workspace, SearchOptions{Query: options.Query, Statuses: []ClaimStatus{ClaimStatusDraft}, Limit: limit})
		if err != nil {
			return TrustedQueryResponse{}, err
		}
		for _, claim := range drafts {
			if err := validateIndexedClaimBinding(paths, workspace, claim); err != nil {
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

func validateIndexedClaimBinding(paths Paths, workspace string, indexed IndexedClaim) error {
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
	return nil
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
		claim, err := store.Read(queryClaim.Workspace, queryClaim.ID)
		if err != nil {
			return nil, err
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
