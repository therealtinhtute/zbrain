package runtime

import (
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

func claimIDs(claims []QueryClaim) []string {
	ids := make([]string, 0, len(claims))
	for _, claim := range claims {
		ids = append(ids, claim.ID)
	}
	return ids
}
