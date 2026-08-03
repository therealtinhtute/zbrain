package runtime

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type IndexStore struct {
	Paths Paths
}

type IndexSummary struct {
	Workspace     string         `json:"workspace"`
	Approved      int            `json:"approved"`
	Draft         int            `json:"draft"`
	Invalid       int            `json:"invalid"`
	InvalidClaims []InvalidClaim `json:"invalid_claims,omitempty"`
	Legacy        int            `json:"legacy"`
}

type SearchOptions struct {
	Query    string
	Statuses []ClaimStatus
	Limit    int
}

type IndexedClaim struct {
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

func (store IndexStore) DatabasePath(workspace string) string {
	return filepath.Join(store.Paths.IndexesDir, workspace+".sqlite")
}

func (store IndexStore) DirtyPath(workspace string) string {
	return filepath.Join(store.Paths.IndexesDir, workspace+".dirty")
}

func (store IndexStore) AssertFTS5() error {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer db.Close()
	var enabled int
	if err := db.QueryRow("select sqlite_compileoption_used('ENABLE_FTS5')").Scan(&enabled); err != nil {
		return err
	}
	if enabled != 1 {
		return fmt.Errorf("sqlite ENABLE_FTS5 compile option is not available")
	}
	if _, err := db.Exec("create virtual table fts_probe using fts5(body)"); err != nil {
		return fmt.Errorf("create FTS5 probe table: %w", err)
	}
	return nil
}

func (store IndexStore) MarkDirty(workspace string) error {
	if err := os.MkdirAll(store.Paths.IndexesDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(store.DirtyPath(workspace), []byte("dirty\n"), 0o644)
}

func (store IndexStore) CheckFresh(workspace string) error {
	if _, err := os.Stat(store.DirtyPath(workspace)); err == nil {
		return fmt.Errorf("workspace %q index is dirty; run zbrain reindex", workspace)
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(store.DatabasePath(workspace)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("workspace %q index does not exist; run zbrain reindex", workspace)
		}
		return err
	}
	return nil
}

func (store IndexStore) Rebuild(workspace string) (IndexSummary, error) {
	if !IsSafeWorkspaceName(workspace) {
		return IndexSummary{}, fmt.Errorf("workspace name must use lowercase letters, numbers, or hyphens only")
	}
	if err := store.AssertFTS5(); err != nil {
		return IndexSummary{}, err
	}
	if err := os.MkdirAll(store.Paths.IndexesDir, 0o755); err != nil {
		return IndexSummary{}, err
	}
	if err := store.MarkDirty(workspace); err != nil {
		return IndexSummary{}, err
	}
	tmpPath := store.DatabasePath(workspace) + ".tmp"
	_ = os.Remove(tmpPath)
	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return IndexSummary{}, err
	}
	defer db.Close()
	if err := createIndexSchema(db); err != nil {
		return IndexSummary{}, err
	}

	scan, err := (ClaimStore{Paths: store.Paths}).ScanWorkspace(workspace)
	if err != nil {
		return IndexSummary{}, err
	}
	summary := IndexSummary{
		Workspace:     workspace,
		Invalid:       len(scan.Invalid),
		InvalidClaims: scan.Invalid,
		Legacy:        len(scan.LegacyUnindexed),
	}
	tx, err := db.Begin()
	if err != nil {
		return IndexSummary{}, err
	}
	for _, claim := range scan.Claims {
		if claim.Status == ClaimStatusApproved {
			summary.Approved++
		} else if claim.Status == ClaimStatusDraft {
			summary.Draft++
		}
		if err := insertIndexedClaim(tx, claim); err != nil {
			_ = tx.Rollback()
			return IndexSummary{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return IndexSummary{}, err
	}
	if err := integrityCheck(db); err != nil {
		return IndexSummary{}, err
	}
	if err := db.Close(); err != nil {
		return IndexSummary{}, err
	}
	if err := os.Rename(tmpPath, store.DatabasePath(workspace)); err != nil {
		return IndexSummary{}, err
	}
	if err := os.Remove(store.DirtyPath(workspace)); err != nil && !os.IsNotExist(err) {
		return IndexSummary{}, err
	}
	return summary, nil
}

func (store IndexStore) Search(workspace string, options SearchOptions) ([]IndexedClaim, error) {
	if options.Limit <= 0 {
		options.Limit = 10
	}
	if strings.TrimSpace(options.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if len(options.Statuses) == 0 {
		return nil, fmt.Errorf("at least one status filter is required")
	}
	if err := store.CheckFresh(workspace); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", store.DatabasePath(workspace))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := integrityCheck(db); err != nil {
		return nil, err
	}

	statusValues := make([]string, 0, len(options.Statuses))
	for _, status := range options.Statuses {
		if !isKnownClaimStatus(status) {
			return nil, fmt.Errorf("claim status %q is not supported", status)
		}
		statusValues = append(statusValues, string(status))
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(statusValues)), ",")
	matchQuery := fts5Query(options.Query)
	if matchQuery == "" {
		return nil, fmt.Errorf("query is required")
	}
	args := []any{matchQuery}
	for _, status := range statusValues {
		args = append(args, status)
	}
	args = append(args, options.Limit)
	rows, err := db.Query(`
select c.id, c.path, c.tier, c.type, c.status, c.title, c.description, c.stale_after, rank
from claims_fts
join claims c on c.rowid = claims_fts.rowid
where claims_fts match ? and c.status in (`+placeholders+`)
order by rank, c.path
limit ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []IndexedClaim{}
	for rows.Next() {
		var result IndexedClaim
		if err := rows.Scan(&result.ID, &result.Path, &result.Tier, &result.Type, &result.Status, &result.Title, &result.Description, &result.StaleAfter, &result.Score); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func fts5Query(query string) string {
	tokens := queryTokens(query)
	quoted := make([]string, 0, len(tokens))
	for _, token := range tokens {
		quoted = append(quoted, `"`+strings.ReplaceAll(token, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " ")
}

func createIndexSchema(db *sql.DB) error {
	_, err := db.Exec(`
create table claims (
  id text not null unique,
  path text not null unique,
  tier text not null,
  type text not null,
  status text not null,
  title text not null,
  description text not null,
  stale_after text not null,
  tags text not null,
  body text not null
);
create virtual table claims_fts using fts5(
  title,
  description,
  tags,
  body,
  content='claims',
  content_rowid='rowid',
  prefix='2 3'
);
create trigger claims_ai after insert on claims begin
  insert into claims_fts(rowid, title, description, tags, body) values (new.rowid, new.title, new.description, new.tags, new.body);
end;
create trigger claims_ad after delete on claims begin
  insert into claims_fts(claims_fts, rowid, title, description, tags, body) values ('delete', old.rowid, old.title, old.description, old.tags, old.body);
end;
create trigger claims_au after update on claims begin
  insert into claims_fts(claims_fts, rowid, title, description, tags, body) values ('delete', old.rowid, old.title, old.description, old.tags, old.body);
  insert into claims_fts(rowid, title, description, tags, body) values (new.rowid, new.title, new.description, new.tags, new.body);
end;`)
	return err
}

func insertIndexedClaim(tx *sql.Tx, claim Claim) error {
	_, err := tx.Exec(`insert into claims(id, path, tier, type, status, title, description, stale_after, tags, body) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		claim.ID,
		claim.Path,
		claim.Tier,
		OKFClaimType,
		string(claim.Status),
		claim.Title,
		claim.Description,
		claim.StaleAfter,
		strings.Join(claim.Tags, " "),
		claim.Body,
	)
	return err
}

func integrityCheck(db *sql.DB) error {
	var result string
	if err := db.QueryRow("pragma integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("sqlite integrity_check = %s", result)
	}
	if _, err := db.Exec("insert into claims_fts(claims_fts, rank) values ('integrity-check', 1)"); err != nil {
		return err
	}
	return nil
}
