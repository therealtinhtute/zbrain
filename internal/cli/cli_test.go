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

func TestRunMigrateOKFNoopLeavesIndexFresh(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"migrate", "okf"}); err != nil {
		t.Fatalf("Run(migrate okf no-op) error = %v", err)
	}
	var summary struct {
		SchemaVersion int  `json:"schema_version"`
		Migrated      int  `json:"migrated"`
		IndexFresh    bool `json:"index_fresh"`
	}
	decodeJSON(t, stdout(app), &summary)
	if summary.SchemaVersion != 1 || summary.Migrated != 0 || !summary.IndexFresh {
		t.Fatalf("migrate no-op summary = %#v", summary)
	}
	dirtyPath, err := (zruntime.IndexStore{Paths: app.Paths}).DirtyPath("research")
	if err != nil {
		t.Fatalf("DirtyPath() error = %v", err)
	}
	if _, err := os.Stat(dirtyPath); !os.IsNotExist(err) {
		t.Fatalf("no-op migration created dirty marker: err = %v", err)
	}
}

func TestRunMigrateOKFFailsBeforeWriteWhenDirtyMarkerCannotBeWritten(t *testing.T) {
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
	if err := os.WriteFile(app.Paths.IndexesDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(indexes dir blocker) error = %v", err)
	}

	if err := app.Run([]string{"migrate", "okf"}); err == nil {
		t.Fatalf("Run(migrate okf with unwritable dirty marker) error = nil")
	}
	migrated, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(legacy claim after failed migration) error = %v", err)
	}
	if !bytes.Equal(migrated, legacy) {
		t.Fatalf("migration rewrote claim despite dirty marker failure:\n%s", migrated)
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

func TestRunMutationsRejectTraversalAndNonexistentWorkspaceBeforeMutation(t *testing.T) {
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

	workspaces := []string{"../../outside/pwn", "missing"}
	for _, workspace := range workspaces {
		workspace := workspace
		t.Run(strings.ReplaceAll(workspace, "/", "_"), func(t *testing.T) {
			mutations := []struct {
				name  string
				args  []string
				stdin string
			}{
				{
					name: "evidence add",
					args: []string{"evidence", "add", "--workspace", workspace, "--file", source, "--origin", "file://source.txt"},
				},
				{
					name:  "claim draft",
					args:  []string{"claim", "draft", "--workspace", workspace, "--tier", "projects", "--title", "Invalid workspace", "--basis", "owner"},
					stdin: "Claim body\n",
				},
				{
					name: "claim approve",
					args: []string{"claim", "approve", "--workspace", workspace, "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				},
				{
					name:  "claim supersede",
					args:  []string{"claim", "supersede", "--workspace", workspace, "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--tier", "projects", "--title", "Invalid workspace", "--basis", "owner"},
					stdin: "Replacement body\n",
				},
				{
					name: "claim revoke",
					args: []string{"claim", "revoke", "--workspace", workspace, "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--reason", "invalid workspace"},
				},
			}
			for _, mutation := range mutations {
				t.Run(mutation.name, func(t *testing.T) {
					app.Stdout = &bytes.Buffer{}
					app.Stdin = strings.NewReader(mutation.stdin)
					if err := app.Run(mutation.args); err == nil {
						t.Fatalf("Run(%s) error = nil", mutation.name)
					}
				})
			}
		})
	}

	if _, err := os.Stat(filepath.Join(tmp, "outside")); !os.IsNotExist(err) {
		t.Fatalf("unsafe workspace created external path: err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(app.Paths.WorkspacesDir, "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing workspace was created: err = %v", err)
	}
	if _, err := os.Stat(app.Paths.IndexesDir); !os.IsNotExist(err) {
		t.Fatalf("invalid workspace created indexes directory: err = %v", err)
	}
}

func TestRunClaimMutationRejectsUnsafeCurrentWorkspaceBeforeMutation(t *testing.T) {
	app, tmp := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	outside := filepath.Join(tmp, "outside", "pwn")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("MkdirAll(outside) error = %v", err)
	}
	if err := os.WriteFile(app.Paths.ConfigFile, []byte("default_workspace: ../../outside/pwn\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	app.Stdin = strings.NewReader("Claim body\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Unsafe current", "--basis", "owner"}); err == nil {
		t.Fatalf("Run(claim draft with unsafe current workspace) error = nil")
	}
	if _, err := os.Stat(filepath.Join(tmp, "outside", "pwn.dirty")); !os.IsNotExist(err) {
		t.Fatalf("unsafe current workspace created external dirty marker: err = %v", err)
	}
	if _, err := os.Stat(app.Paths.IndexesDir); !os.IsNotExist(err) {
		t.Fatalf("unsafe current workspace created indexes directory: err = %v", err)
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
		SchemaVersion  int    `json:"schema_version"`
		Approved       int    `json:"approved"`
		RebuildState   string `json:"rebuild_state"`
		InvalidCount   int    `json:"invalid_count"`
		ManifestDigest string `json:"manifest_digest"`
		RebuiltAt      string `json:"rebuilt_at"`
	}
	decodeJSON(t, stdout(app), &summary)
	if summary.SchemaVersion != 1 || summary.Approved != 1 || summary.RebuildState != "clean" || summary.InvalidCount != 0 || len(summary.ManifestDigest) != 64 || summary.RebuiltAt == "" {
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

func TestRunReindexReportsTamperedApprovedClaim(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("trusted canonical answer\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Trusted Claim", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft) error = %v", err)
	}
	var draft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &draft)
	if err := app.Run([]string{"claim", "approve", draft.ID}); err != nil {
		t.Fatalf("Run(claim approve) error = %v", err)
	}

	claimPath := filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects", draft.ID+".md")
	contents, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(claim) error = %v", err)
	}
	contents = []byte(strings.Replace(string(contents), "trusted canonical answer", "tampered canonical answer", 1))
	if err := os.WriteFile(claimPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(tampered claim) error = %v", err)
	}

	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"reindex"}); err != nil {
		t.Fatalf("Run(reindex) error = %v", err)
	}
	var summary struct {
		Approved       int    `json:"approved"`
		Invalid        int    `json:"invalid"`
		InvalidCount   int    `json:"invalid_count"`
		RebuildState   string `json:"rebuild_state"`
		ManifestDigest string `json:"manifest_digest"`
		InvalidClaims  []struct {
			Path  string `json:"path"`
			Error string `json:"error"`
		} `json:"invalid_claims"`
	}
	decodeJSON(t, stdout(app), &summary)
	if summary.Approved != 0 || summary.Invalid != 1 || summary.InvalidCount != 1 || summary.RebuildState != "rejected" || len(summary.ManifestDigest) != 64 || len(summary.InvalidClaims) != 1 {
		t.Fatalf("reindex summary = %#v", summary)
	}
	if summary.InvalidClaims[0].Path != "projects/"+draft.ID+".md" || !strings.Contains(summary.InvalidClaims[0].Error, "verification digest mismatch") {
		t.Fatalf("invalid claim = %#v", summary.InvalidClaims[0])
	}

	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "trusted", "canonical"}); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("Run(ask) error = %v, want rejected error", err)
	}
	if stdout(app) != "" {
		t.Fatalf("ask wrote output for rejected index: %q", stdout(app))
	}
}

func TestRunAskReportsFreshnessErrors(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		app, _ := testApp(t)
		setupResearchApp(t, &app)
		app.Stdout = &bytes.Buffer{}
		err := app.Run([]string{"ask", "anything"})
		if err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("Run(ask) error = %v, want missing error", err)
		}
	})

	t.Run("dirty", func(t *testing.T) {
		app, _ := testApp(t)
		setupResearchApp(t, &app)
		if err := app.Run([]string{"reindex"}); err != nil {
			t.Fatalf("Run(reindex) error = %v", err)
		}
		if err := (zruntime.IndexStore{Paths: app.Paths}).MarkDirty("research"); err != nil {
			t.Fatalf("MarkDirty() error = %v", err)
		}
		app.Stdout = &bytes.Buffer{}
		err := app.Run([]string{"ask", "anything"})
		if err == nil || !strings.Contains(err.Error(), "dirty") {
			t.Fatalf("Run(ask) error = %v, want dirty error", err)
		}
	})

	t.Run("stale", func(t *testing.T) {
		app, _ := testApp(t)
		setupResearchApp(t, &app)
		if err := app.Run([]string{"reindex"}); err != nil {
			t.Fatalf("Run(reindex) error = %v", err)
		}
		stalePath := filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects", "outside-edit.md")
		if err := os.WriteFile(stalePath, []byte("outside edit\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(stale input) error = %v", err)
		}
		app.Stdout = &bytes.Buffer{}
		err := app.Run([]string{"ask", "anything"})
		if err == nil || !strings.Contains(err.Error(), "stale") || !strings.Contains(err.Error(), stalePath) || !strings.Contains(err.Error(), "run zbrain reindex") {
			t.Fatalf("Run(ask) error = %v, want stale error naming %q and reindex recovery", err, stalePath)
		}
	})

	t.Run("rejected", func(t *testing.T) {
		app, _ := testApp(t)
		setupResearchApp(t, &app)
		legacyPath := filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects", "legacy.md")
		if err := os.WriteFile(legacyPath, []byte("legacy input\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(legacy) error = %v", err)
		}
		if err := app.Run([]string{"reindex"}); err != nil {
			t.Fatalf("Run(reindex) error = %v", err)
		}
		app.Stdout = &bytes.Buffer{}
		err := app.Run([]string{"ask", "anything"})
		if err == nil || !strings.Contains(err.Error(), "rejected") {
			t.Fatalf("Run(ask) error = %v, want rejected error", err)
		}
	})
}

func setupResearchApp(t *testing.T, app *App) {
	t.Helper()
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
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
