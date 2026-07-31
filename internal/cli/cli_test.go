package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

func testApp(t *testing.T) (App, string) {
	t.Helper()
	tmp := t.TempDir()
	paths, err := zruntime.ResolvePaths(zruntime.Options{CWD: filepath.Join(tmp, "project"), HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	return App{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		Stdin:  strings.NewReader(""),
		Paths:  paths,
		Now:    func() time.Time { return time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC) },
	}, tmp
}

func stdout(app App) string {
	return app.Stdout.(*bytes.Buffer).String()
}

func TestRunSetup(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	out := stdout(app)
	if !strings.Contains(out, "zbrain setup complete") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunWorkspaceCreateCurrent(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"workspace", "current"}); err != nil {
		t.Fatalf("Run(workspace current) error = %v", err)
	}
	out := stdout(app)
	if !strings.Contains(out, `"workspace": "research"`) {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunEvidenceAddAndClaimLifecycleJSON(t *testing.T) {
	app, tmp := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	source := filepath.Join(tmp, "source.txt")
	if err := os.WriteFile(source, []byte("source bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"evidence", "add", "--file", source, "--origin", "file://source.txt", "--media-type", "text/plain"}); err != nil {
		t.Fatalf("Run(evidence add) error = %v", err)
	}
	var evidence struct {
		SchemaVersion int    `json:"schema_version"`
		ID            string `json:"id"`
		Workspace     string `json:"workspace"`
	}
	decodeJSON(t, stdout(app), &evidence)
	if evidence.SchemaVersion != 1 || evidence.Workspace != "research" || !strings.HasPrefix(evidence.ID, "evd_") {
		t.Fatalf("evidence output = %#v", evidence)
	}

	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("Claim body\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Evidence Claim", "--basis", "evidence", "--evidence", evidence.ID}); err != nil {
		t.Fatalf("Run(claim draft) error = %v", err)
	}
	var draft struct {
		SchemaVersion int    `json:"schema_version"`
		ID            string `json:"id"`
		Status        string `json:"status"`
		Path          string `json:"path"`
	}
	decodeJSON(t, stdout(app), &draft)
	if draft.SchemaVersion != 1 || draft.Status != "draft" || !strings.HasPrefix(draft.ID, "clm_") || draft.Path == "" {
		t.Fatalf("draft output = %#v", draft)
	}
	claimPath := filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects", draft.ID+".md")
	claimFile, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(claim) error = %v", err)
	}
	if !strings.Contains(string(claimFile), "type: zbrain.claim") || !strings.Contains(string(claimFile), "profile: zbrain.trusted-memory/v1") || strings.Contains(string(claimFile), "schema: zbrain.claim/v1") {
		t.Fatalf("claim draft did not write OKF profile file:\n%s", claimFile)
	}

	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"claim", "approve", draft.ID}); err != nil {
		t.Fatalf("Run(claim approve) error = %v", err)
	}
	var approved struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	decodeJSON(t, stdout(app), &approved)
	if approved.ID != draft.ID || approved.Status != "approved" {
		t.Fatalf("approved output = %#v", approved)
	}
}

func TestRunClaimApproveRejectsMissingEvidence(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("Claim body\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Needs Evidence", "--basis", "evidence"}); err != nil {
		t.Fatalf("Run(claim draft) error = %v", err)
	}
	var draft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &draft)
	if err := app.Run([]string{"claim", "approve", draft.ID}); err == nil {
		t.Fatalf("Run(claim approve missing evidence) error = nil")
	}
}

func TestRunMigrateOKFConvertsLegacyClaim(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	id := "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	claimPath := filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects", id+".md")
	legacy := []byte("---\nschema: zbrain.claim/v1\nid: " + id + "\nstatus: draft\ntitle: Legacy Claim\nbasis: owner\ncreated_at: 2026-07-30T09:00:00Z\ncreated_by: owner\n---\n\nLegacy body\n")
	if err := os.WriteFile(claimPath, legacy, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy claim) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"migrate", "okf"}); err != nil {
		t.Fatalf("Run(migrate okf) error = %v", err)
	}
	var summary struct {
		SchemaVersion int  `json:"schema_version"`
		Migrated      int  `json:"migrated"`
		IndexFresh    bool `json:"index_fresh"`
	}
	decodeJSON(t, stdout(app), &summary)
	if summary.SchemaVersion != 1 || summary.Migrated != 1 || summary.IndexFresh {
		t.Fatalf("migrate summary = %#v", summary)
	}
	migrated, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(migrated claim) error = %v", err)
	}
	if strings.Contains(string(migrated), "schema: zbrain.claim/v1") || !strings.Contains(string(migrated), "type: zbrain.claim") {
		t.Fatalf("claim was not migrated to OKF:\n%s", migrated)
	}
}

func TestRunClaimDraftFailsBeforeWriteWhenDirtyMarkerCannotBeWritten(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	if err := os.WriteFile(app.Paths.IndexesDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(indexes dir blocker) error = %v", err)
	}

	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("Claim body\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Dirty Marker", "--basis", "owner"}); err == nil {
		t.Fatalf("Run(claim draft with unwritable dirty marker) error = nil")
	}
	if entries, err := os.ReadDir(filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects")); err != nil {
		t.Fatalf("ReadDir(projects) error = %v", err)
	} else if len(entries) != 0 {
		t.Fatalf("claim draft wrote files despite dirty marker failure: %v", entries)
	}
}

func TestRunReindexAndAskTrustedContext(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("local trusted answer\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Trusted Ask", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft) error = %v", err)
	}
	var draft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &draft)
	if err := app.Run([]string{"claim", "approve", draft.ID}); err != nil {
		t.Fatalf("Run(claim approve) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"reindex"}); err != nil {
		t.Fatalf("Run(reindex) error = %v", err)
	}
	var summary struct {
		SchemaVersion int `json:"schema_version"`
		Approved      int `json:"approved"`
	}
	decodeJSON(t, stdout(app), &summary)
	if summary.SchemaVersion != 1 || summary.Approved != 1 {
		t.Fatalf("reindex summary = %#v", summary)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "local", "trusted"}); err != nil {
		t.Fatalf("Run(ask) error = %v", err)
	}
	var response struct {
		SchemaVersion int    `json:"schema_version"`
		Status        string `json:"status"`
		Claims        []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"claims"`
	}
	decodeJSON(t, stdout(app), &response)
	if response.SchemaVersion != 1 || response.Status != "ready" || len(response.Claims) != 1 || response.Claims[0].Status != "approved" {
		t.Fatalf("ask response = %#v", response)
	}
}

func TestRunAskReportsGap(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	if err := app.Run([]string{"reindex"}); err != nil {
		t.Fatalf("Run(reindex) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "missing", "claim"}); err != nil {
		t.Fatalf("Run(ask gap) error = %v", err)
	}
	var response struct {
		Status string        `json:"status"`
		Gaps   []interface{} `json:"gaps"`
	}
	decodeJSON(t, stdout(app), &response)
	if response.Status != "gap" || len(response.Gaps) != 1 {
		t.Fatalf("gap response = %#v", response)
	}
}

func TestRunAskUsesOnlyExplicitIncludes(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create research) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "personal"}); err != nil {
		t.Fatalf("Run(workspace create personal) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("private explicit token\n")
	if err := app.Run([]string{"claim", "draft", "--workspace", "personal", "--tier", "projects", "--title", "Personal", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft personal) error = %v", err)
	}
	var draft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &draft)
	if err := app.Run([]string{"claim", "approve", "--workspace", "personal", draft.ID}); err != nil {
		t.Fatalf("Run(claim approve personal) error = %v", err)
	}
	if err := app.Run([]string{"reindex", "--workspace", "research"}); err != nil {
		t.Fatalf("Run(reindex research) error = %v", err)
	}
	if err := app.Run([]string{"reindex", "--workspace", "personal"}); err != nil {
		t.Fatalf("Run(reindex personal) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "private", "explicit"}); err != nil {
		t.Fatalf("Run(ask without include) error = %v", err)
	}
	var noInclude struct {
		Status string `json:"status"`
	}
	decodeJSON(t, stdout(app), &noInclude)
	if noInclude.Status != "gap" {
		t.Fatalf("no include status = %q", noInclude.Status)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "--include", "personal", "private", "explicit"}); err != nil {
		t.Fatalf("Run(ask include) error = %v", err)
	}
	var included struct {
		Status string `json:"status"`
		Claims []struct {
			Workspace string `json:"workspace"`
		} `json:"claims"`
	}
	decodeJSON(t, stdout(app), &included)
	if included.Status != "ready" || len(included.Claims) != 1 || included.Claims[0].Workspace != "personal" {
		t.Fatalf("included response = %#v", included)
	}
}

func decodeJSON(t *testing.T, input string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(input), target); err != nil {
		t.Fatalf("invalid JSON %q: %v", input, err)
	}
}
