package runtime

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHybridRetrieval covers the TrustedQuery embedding option:
// pure lexical fallback, merged lexical+vector results without
// duplicates, and fail-closed freshness on the hybrid path.
func TestHybridRetrieval(t *testing.T) {
	const (
		lexicalOnlyID = "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		vectorOnlyID  = "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)

	setup := func(t *testing.T, withEmbedding bool) (Paths, *IndexStore) {
		t.Helper()
		paths := queryTestPaths(t)
		store := ClaimStore{Paths: paths, Now: fixedQueryNow}

		lexicalOnly := queryClaim(lexicalOnlyID, "Quantum Entanglement Observer", ClaimBasisOwner)
		lexicalOnly.Body = "quantum entanglement observer discovery\n"

		vectorOnly := queryClaim(vectorOnlyID, "Entangled Observer", ClaimBasisOwner)
		vectorOnly.Body = "entanglement observer phenomena\n"

		for _, claim := range []Claim{lexicalOnly, vectorOnly} {
			if _, err := store.WriteDraft("research", claim); err != nil {
				t.Fatalf("WriteDraft(%s) error = %v", claim.ID, err)
			}
			if _, err := store.Approve("research", claim.ID); err != nil {
				t.Fatalf("Approve(%s) error = %v", claim.ID, err)
			}
		}

		idx := IndexStore{Paths: paths}
		if _, err := idx.RebuildWithOptions("research", RebuildOptions{Embedding: withEmbedding}); err != nil {
			t.Fatalf("RebuildWithOptions(embed=%t) error = %v", withEmbedding, err)
		}
		return paths, &idx
	}

	t.Run("no embedding database uses pure lexical", func(t *testing.T) {
		paths, _ := setup(t, false)

		withOption, err := TrustedQuery(paths, TrustedQueryOptions{Query: "quantum entanglement observer", Limit: 10, Embedding: true})
		if err != nil {
			t.Fatalf("TrustedQuery(embedding=true) error = %v", err)
		}
		withoutOption, err := TrustedQuery(paths, TrustedQueryOptions{Query: "quantum entanglement observer", Limit: 10})
		if err != nil {
			t.Fatalf("TrustedQuery(embedding=false) error = %v", err)
		}
		if withOption.Status != QueryStatusReady || withoutOption.Status != QueryStatusReady {
			t.Fatalf("status = %q / %q, want ready", withOption.Status, withoutOption.Status)
		}
		if len(withOption.Claims) != 1 || withOption.Claims[0].ID != lexicalOnlyID {
			t.Fatalf("embedding=true claims = %#v, want pure lexical only", claimIDs(withOption.Claims))
		}
		if len(withoutOption.Claims) != 1 || withoutOption.Claims[0].ID != lexicalOnlyID {
			t.Fatalf("embedding=false claims = %#v, want pure lexical only", claimIDs(withoutOption.Claims))
		}
	})

	t.Run("empty embedding sidecar uses pure lexical", func(t *testing.T) {
		paths, _ := setup(t, false)
		embeddingPath := (EmbeddingStore{Paths: paths}).DatabasePath("research")
		db, err := sql.Open("sqlite", embeddingPath)
		if err != nil {
			t.Fatalf("sql.Open(empty sidecar) error = %v", err)
		}
		if _, err := db.Exec(`create table embeddings (
			claim_id text not null primary key,
			vector blob not null,
			dimension integer not null
		)`); err != nil {
			_ = db.Close()
			t.Fatalf("create empty sidecar schema error = %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close(empty sidecar) error = %v", err)
		}

		response, err := TrustedQuery(paths, TrustedQueryOptions{Query: "quantum entanglement observer", Limit: 10, Embedding: true})
		if err != nil {
			t.Fatalf("TrustedQuery(empty sidecar) error = %v", err)
		}
		if ids := claimIDs(response.Claims); len(ids) != 1 || ids[0] != lexicalOnlyID {
			t.Fatalf("empty sidecar claims = %v, want pure lexical result %q", ids, lexicalOnlyID)
		}
	})

	t.Run("embeddings merge lexical and vector matches without duplicates", func(t *testing.T) {
		paths, _ := setup(t, true)

		response, err := TrustedQuery(paths, TrustedQueryOptions{Query: "quantum entanglement observer", Limit: 10, Embedding: true})
		if err != nil {
			t.Fatalf("TrustedQuery() error = %v", err)
		}
		if response.Status != QueryStatusReady {
			t.Fatalf("Status = %q, want ready", response.Status)
		}
		ids := claimIDs(response.Claims)
		// Pure lexical returns 1 hit (quantum Entanglement Observer title).
		// Vector adds the second claim (Entangled Observer) due to token overlap.
		if len(ids) < 2 {
			t.Fatalf("claims = %v, want at least 2 hybrid matches (lexical + vector)", ids)
		}
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				t.Fatalf("duplicate claim ID %q in hybrid results", id)
			}
			seen[id] = true
		}
		// Lexical match must be first.
		if ids[0] != lexicalOnlyID {
			t.Fatalf("first claim = %q, want lexical match first", ids[0])
		}
	})

	t.Run("embeddings without option keep lexical only", func(t *testing.T) {
		paths, _ := setup(t, true)

		response, err := TrustedQuery(paths, TrustedQueryOptions{Query: "quantum entanglement observer", Limit: 10})
		if err != nil {
			t.Fatalf("TrustedQuery() error = %v", err)
		}
		ids := claimIDs(response.Claims)
		if len(ids) != 1 || ids[0] != lexicalOnlyID {
			t.Fatalf("claims = %v, want lexical only when embedding not requested", ids)
		}
	})

	t.Run("stale index fails closed on hybrid path", func(t *testing.T) {
		paths, _ := setup(t, true)

		claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", lexicalOnlyID+".md")
		contents, err := os.ReadFile(claimPath)
		if err != nil {
			t.Fatalf("ReadFile(claim) error = %v", err)
		}
		contents = []byte(strings.Replace(string(contents), "quantum entanglement observer discovery", "changed quantum entanglement observer discovery", 1))
		if err := os.WriteFile(claimPath, contents, 0o644); err != nil {
			t.Fatalf("WriteFile(changed claim) error = %v", err)
		}

		if _, err := TrustedQuery(paths, TrustedQueryOptions{Query: "quantum entanglement observer", Limit: 10, Embedding: true}); err == nil || !strings.Contains(err.Error(), "stale") {
			t.Fatalf("TrustedQuery() error = %v, want explicit stale error", err)
		}
	})
}

func TestInterleaveClaimsDeduplicatesByID(t *testing.T) {
	lexical := []IndexedClaim{
		{ID: "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Score: 1},
		{ID: "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Score: 2},
	}
	vector := []IndexedClaim{
		{ID: "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Score: 1},
		{ID: "clm_cccccccccccccccccccccccccccccccc", Score: 2},
	}

	merged := interleaveClaims(lexical, vector, 10)
	ids := make([]string, 0, len(merged))
	seen := map[string]bool{}
	for _, claim := range merged {
		if seen[claim.ID] {
			t.Fatalf("duplicate claim ID %q in merged results", claim.ID)
		}
		seen[claim.ID] = true
		ids = append(ids, claim.ID)
	}
	// RRF k=60: b appears in both lists (rank2 lex + rank1 vec) => highest rrf,
	// a rank1 lex-only => 1/61, c rank2 vec-only => 1/62. Order must be b,a,c.
	want := []string{
		"clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"clm_cccccccccccccccccccccccccccccccc",
	}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("merged IDs = %v, want %v", ids, want)
	}
	// Scores must be ascending to preserve sortQueryClaims order.
	for i := 1; i < len(merged); i++ {
		if merged[i].Score <= merged[i-1].Score {
			t.Fatalf("scores not ascending: %v", merged)
		}
	}
	if merged[0].Score != 0 || merged[1].Score != 1 || merged[2].Score != 2 {
		t.Fatalf("scores = %v, want [0 1 2]", []float64{merged[0].Score, merged[1].Score, merged[2].Score})
	}
}

func TestInterleaveRRF(t *testing.T) {
	// lex=[a,b] vec=[b,c] => b appears in both => highest RRF, then a (rank1 lex),
	// then c (rank2 vec).
	lexical := []IndexedClaim{
		{ID: "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Score: 9},
		{ID: "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Score: 9},
	}
	vector := []IndexedClaim{
		{ID: "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Score: 9},
		{ID: "clm_cccccccccccccccccccccccccccccccc", Score: 9},
	}
	merged := interleaveClaims(lexical, vector, 10)
	if len(merged) != 3 {
		t.Fatalf("merged len = %d, want 3", len(merged))
	}
	ids := []string{merged[0].ID, merged[1].ID, merged[2].ID}
	want := []string{
		"clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"clm_cccccccccccccccccccccccccccccccc",
	}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("RRF order = %v, want %v (b highest rrf, then a rank1 lex, then c rank2 vec)", ids, want)
	}
	// Verify b has highest RRF by checking it is first and scores preserve RRF order.
	if merged[0].ID != "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("first claim = %q, want b (highest RRF due to presence in both lists)", merged[0].ID)
	}
	// Scores must be 0,1,2 ascending so sortQueryClaims preserves RRF order.
	for i, c := range merged {
		if c.Score != float64(i) {
			t.Fatalf("merged[%d].Score = %v, want %d", i, c.Score, i)
		}
	}
	// Verify sortQueryClaims would keep RRF order (ascending Score).
	claims := []QueryClaim{
		{ID: merged[0].ID, Score: merged[0].Score},
		{ID: merged[1].ID, Score: merged[1].Score},
		{ID: merged[2].ID, Score: merged[2].Score},
	}
	// Shuffle and sort to ensure stability.
	shuffled := []QueryClaim{claims[2], claims[0], claims[1]}
	sortQueryClaims(shuffled)
	if shuffled[0].ID != want[0] || shuffled[1].ID != want[1] || shuffled[2].ID != want[2] {
		t.Fatalf("sortQueryClaims order = %v, want %v", []string{shuffled[0].ID, shuffled[1].ID, shuffled[2].ID}, want)
	}
	// Limit truncation: top 2 should be b and a.
	truncated := interleaveClaims(lexical, vector, 2)
	if len(truncated) != 2 || truncated[0].ID != want[0] || truncated[1].ID != want[1] {
		t.Fatalf("truncated RRF = %v, want %v", []string{truncated[0].ID, truncated[1].ID}, want[:2])
	}
}

func TestHybridRetrievalRespectsWorkspaceIsolation(t *testing.T) {
	paths := queryTestPaths(t)
	if err := CreateWorkspace(paths, "personal", fixedQueryNow()); err != nil {
		t.Fatalf("CreateWorkspace(personal) error = %v", err)
	}
	store := ClaimStore{Paths: paths, Now: fixedQueryNow}
	researchClaim := queryClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Research Memory", ClaimBasisOwner)
	researchClaim.Body = "research-only context marker\n"
	personalClaim := queryClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "Personal Memory", ClaimBasisOwner)
	personalClaim.Body = "workspace-private hybrid marker\n"
	for _, entry := range []struct {
		workspace string
		claim     Claim
	}{
		{workspace: "research", claim: researchClaim},
		{workspace: "personal", claim: personalClaim},
	} {
		if _, err := store.WriteDraft(entry.workspace, entry.claim); err != nil {
			t.Fatalf("WriteDraft(%s) error = %v", entry.workspace, err)
		}
		if _, err := store.Approve(entry.workspace, entry.claim.ID); err != nil {
			t.Fatalf("Approve(%s) error = %v", entry.workspace, err)
		}
	}
	idx := IndexStore{Paths: paths}
	for _, workspace := range []string{"research", "personal"} {
		if _, err := idx.RebuildWithOptions(workspace, RebuildOptions{Embedding: true}); err != nil {
			t.Fatalf("RebuildWithOptions(%s) error = %v", workspace, err)
		}
	}

	withoutInclude, err := TrustedQuery(paths, TrustedQueryOptions{
		Workspace: "research",
		Query:     "workspace-private hybrid marker",
		Limit:     10,
		Embedding: true,
	})
	if err != nil {
		t.Fatalf("TrustedQuery(without include) error = %v", err)
	}
	if withoutInclude.Scopes.Primary != "research" || len(withoutInclude.Scopes.Includes) != 0 {
		t.Fatalf("withoutInclude scopes = %#v, want research only", withoutInclude.Scopes)
	}
	for _, claim := range withoutInclude.Claims {
		if claim.Workspace != "research" || claim.ID == personalClaim.ID {
			t.Fatalf("withoutInclude claim = %#v, leaked personal workspace", claim)
		}
	}

	withInclude, err := TrustedQuery(paths, TrustedQueryOptions{
		Workspace: "research",
		Includes:  []string{"personal"},
		Query:     "workspace-private hybrid marker",
		Limit:     10,
		Embedding: true,
	})
	if err != nil {
		t.Fatalf("TrustedQuery(with include) error = %v", err)
	}
	foundPersonal := false
	for _, claim := range withInclude.Claims {
		if claim.Workspace == "personal" && claim.ID == personalClaim.ID {
			foundPersonal = true
		}
		if claim.Workspace != "research" && claim.Workspace != "personal" {
			t.Fatalf("withInclude claim = %#v, outside resolved scopes", claim)
		}
	}
	if !foundPersonal {
		t.Fatalf("withInclude claims = %#v, want personal claim only when explicitly included", withInclude.Claims)
	}
}

func claimIDs(claims []QueryClaim) []string {
	ids := make([]string, 0, len(claims))
	for _, claim := range claims {
		ids = append(ids, claim.ID)
	}
	return ids
}
