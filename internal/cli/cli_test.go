package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

func TestCLIHelpParity(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "root",
			args: []string{"--help"},
			want: []string{
				"ask [--workspace <name>] [--include <name>]... [--embed] <query>",
				"evidence add --file <path> --origin <uri-or-path>",
				"claim supersede <id>",
				"zbrain <command> --help",
			},
		},
		{name: "setup", args: []string{"setup", "--help"}, want: []string{"Usage: zbrain setup"}},
		{name: "version", args: []string{"version", "--help"}, want: []string{"Usage: zbrain version"}},
		{name: "workspace", args: []string{"workspace", "--help"}, want: []string{"zbrain workspace create <name>", "zbrain workspace current"}},
		{name: "evidence", args: []string{"evidence", "--help"}, want: []string{"--file <path>", "--origin <uri-or-path>", "--workspace <name>"}},
		{name: "claim", args: []string{"claim", "--help"}, want: []string{"claim draft --tier <tier>", "claim supersede <id>", "claim revoke <id> --reason <text>"}},
		{name: "workspace create", args: []string{"workspace", "create", "--help"}, want: []string{"Usage: zbrain workspace create <name>"}},
		{name: "workspace current", args: []string{"workspace", "current", "--help"}, want: []string{"Usage: zbrain workspace current"}},
		{name: "evidence", args: []string{"evidence", "add", "--help"}, want: []string{"--file <path>", "--origin <uri-or-path>", "--media-type <type>", "--workspace <name>"}},
		{name: "claim draft", args: []string{"claim", "draft", "--help"}, want: []string{"--tier <tier>", "--title <title>", "--basis <basis>", "--evidence <id>", "--support <id>", "--conflicts-with <id>", "--workspace <name>"}},
		{name: "claim approve", args: []string{"claim", "approve", "--help"}, want: []string{"Usage: zbrain claim approve <id> [--workspace <name>]"}},
		{name: "claim supersede", args: []string{"claim", "supersede", "--help"}, want: []string{"--tier <tier>", "--title <title>", "--basis <basis>"}},
		{name: "claim revoke", args: []string{"claim", "revoke", "--help"}, want: []string{"--reason <text>", "--workspace <name>"}},
		{name: "migrate", args: []string{"migrate", "okf", "--help"}, want: []string{"Usage: zbrain migrate okf [--workspace <name>]"}},
		{name: "reindex", args: []string{"reindex", "--help"}, want: []string{"Usage: zbrain reindex [--workspace <name>]"}},
		{name: "ask", args: []string{"ask", "--help"}, want: []string{"--workspace <name>", "--include <name>", "--embed"}},
		{name: "status", args: []string{"status", "--help"}, want: []string{"status"}},
		{name: "approval", args: []string{"approval", "--help"}, want: []string{"approval show <challenge-id>", "approval grant <challenge-id>"}},
		{name: "approval show", args: []string{"approval", "show", "--help"}, want: []string{"Usage: zbrain approval show <challenge-id>"}},
		{name: "approval grant", args: []string{"approval", "grant", "--help"}, want: []string{"Usage: zbrain approval grant <challenge-id>"}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			app, _ := testApp(t)
			if err := app.Run(test.args); err != nil {
				t.Fatalf("Run(%v) error = %v", test.args, err)
			}
			for _, want := range test.want {
				if !strings.Contains(stdout(app), want) {
					t.Fatalf("Run(%v) output %q does not contain %q", test.args, stdout(app), want)
				}
			}
		})
	}
}

func TestUnknownFlag(t *testing.T) {
	cases := [][]string{
		{"--bogus"},
		{"setup", "--bogus"},
		{"workspace", "create", "research", "--bogus", "value"},
		{"workspace", "current", "--bogus"},
		{"evidence", "add", "--file", "source", "--origin", "local", "--bogus", "value"},
		{"claim", "draft", "--tier", "projects", "--bogus", "value"},
		{"claim", "approve", "clm_example", "--bogus", "value"},
		{"claim", "supersede", "clm_example", "--tier", "projects", "--bogus", "value"},
		{"claim", "revoke", "clm_example", "--reason", "reason", "--bogus", "value"},
		{"migrate", "okf", "--bogus", "value"},
		{"reindex", "--bogus"},
		{"ask", "query", "--bogus", "value"},
		{"approval", "show", "--bogus", "value"},
		{"approval", "grant", "--bogus", "value"},
		{"version", "--bogus"},
	}

	for _, args := range cases {
		args := args
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			app, _ := testApp(t)
			err := app.Run(args)
			if err == nil || !strings.Contains(err.Error(), "unknown flag") {
				t.Fatalf("Run(%v) error = %v, want unknown flag error", args, err)
			}
		})
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

func TestWorkspaceCurrentContract(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"workspace", "current"}); err != nil {
		t.Fatalf("Run(workspace current) error = %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout(app)), &decoded); err != nil {
		t.Fatalf("workspace current JSON is invalid: %v", err)
	}
	wantKeys := map[string]bool{
		"project_root":         true,
		"workspace":            true,
		"secondary_workspaces": true,
	}
	if len(decoded) != len(wantKeys) {
		t.Fatalf("workspace current keys = %#v, want exactly %#v", decoded, wantKeys)
	}
	for key := range wantKeys {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("workspace current missing %q", key)
		}
	}
	if _, ok := decoded["context_file"]; ok {
		t.Fatalf("workspace current must not advertise unsupported context_file storage")
	}
	var workspace string
	if err := json.Unmarshal(decoded["workspace"], &workspace); err != nil {
		t.Fatalf("workspace field is invalid: %v", err)
	}
	if workspace != "research" {
		t.Fatalf("workspace = %q, want research", workspace)
	}
	var secondary []string
	if err := json.Unmarshal(decoded["secondary_workspaces"], &secondary); err != nil {
		t.Fatalf("secondary_workspaces field is invalid: %v", err)
	}
	if len(secondary) != 0 {
		t.Fatalf("secondary_workspaces = %#v, want empty", secondary)
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

func TestRunClaimApproveSupersedeRevokeTransitions(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}

	originalBody := "Original body\n"
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader(originalBody)
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Original", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft) error = %v", err)
	}
	var draft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &draft)
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"claim", "approve", draft.ID}); err != nil {
		t.Fatalf("Run(claim approve) error = %v", err)
	}
	if err := app.Run([]string{"claim", "approve", draft.ID}); err == nil {
		t.Fatalf("Run(claim approve twice) error = nil")
	}

	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("Replacement body\n")
	if err := app.Run([]string{"claim", "supersede", draft.ID, "--tier", "projects", "--title", "Replacement", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim supersede) error = %v", err)
	}
	var replacement struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &replacement)
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"claim", "approve", replacement.ID}); err != nil {
		t.Fatalf("Run(claim approve replacement) error = %v", err)
	}

	store := zruntime.ClaimStore{Paths: app.Paths, Now: app.Now}
	old, err := store.Read("research", draft.ID)
	if err != nil {
		t.Fatalf("Read(old) error = %v", err)
	}
	if old.Status != zruntime.ClaimStatusSuperseded || old.Body != originalBody || len(old.Transitions) != 2 {
		t.Fatalf("old lifecycle state = %#v", old)
	}
	replacementBefore, err := store.Read("research", replacement.ID)
	if err != nil {
		t.Fatalf("Read(replacement) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"claim", "revoke", replacement.ID, "--reason", "withdrawn"}); err != nil {
		t.Fatalf("Run(claim revoke) error = %v", err)
	}
	revoked, err := store.Read("research", replacement.ID)
	if err != nil {
		t.Fatalf("Read(revoked) error = %v", err)
	}
	if revoked.Status != zruntime.ClaimStatusRevoked || revoked.Body != replacementBefore.Body || revoked.VerifiedDigest != replacementBefore.VerifiedDigest || len(revoked.Transitions) != 2 || revoked.Transitions[1].Kind != zruntime.ClaimTransitionRevoke {
		t.Fatalf("replacement lifecycle state = %#v", revoked)
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
		SchemaVersion      int  `json:"schema_version"`
		Migrated           int  `json:"migrated"`
		IndexFresh         bool `json:"index_fresh"`
		ReapprovalRequired int  `json:"reapproval_required"`
	}
	decodeJSON(t, stdout(app), &summary)
	if summary.SchemaVersion != 1 || summary.Migrated != 1 || summary.IndexFresh || summary.ReapprovalRequired != 0 {
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
	if err := app.Run([]string{"reindex"}); err != nil {
		t.Fatalf("Run(reindex before migrate no-op) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"migrate", "okf"}); err != nil {
		t.Fatalf("Run(migrate okf no-op) error = %v", err)
	}
	var summary struct {
		SchemaVersion   int    `json:"schema_version"`
		Migrated        int    `json:"migrated"`
		IndexFresh      bool   `json:"index_fresh"`
		IndexFreshError string `json:"index_fresh_error"`
	}
	decodeJSON(t, stdout(app), &summary)
	if summary.SchemaVersion != 1 || summary.Migrated != 0 || !summary.IndexFresh || summary.IndexFreshError != "" {
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

func TestMigrateOKFFreshnessReportsUnknownWithoutIndex(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"migrate", "okf"}); err != nil {
		t.Fatalf("Run(migrate okf without index) error = %v", err)
	}
	var summary struct {
		IndexFresh      bool   `json:"index_fresh"`
		IndexFreshError string `json:"index_fresh_error"`
	}
	decodeJSON(t, stdout(app), &summary)
	if summary.IndexFresh || !strings.Contains(summary.IndexFreshError, "does not exist") {
		t.Fatalf("migrate freshness summary = %#v, want explicit unknown result", summary)
	}
}

func TestMigrateOKFFreshnessReportsDirtyIndex(t *testing.T) {
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
	if err := (zruntime.IndexStore{Paths: app.Paths}).MarkDirty("research"); err != nil {
		t.Fatalf("MarkDirty() error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"migrate", "okf"}); err != nil {
		t.Fatalf("Run(migrate okf with dirty index) error = %v", err)
	}
	var summary struct {
		IndexFresh      bool   `json:"index_fresh"`
		IndexFreshError string `json:"index_fresh_error"`
	}
	decodeJSON(t, stdout(app), &summary)
	if summary.IndexFresh || !strings.Contains(summary.IndexFreshError, "dirty") {
		t.Fatalf("migrate freshness summary = %#v, want dirty index result", summary)
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

func TestRunAskEmbeddingOptIn(t *testing.T) {
	app, _ := testApp(t)
	setupResearchApp(t, &app)
	const (
		lexicalOnlyID = "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		vectorOnlyID  = "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	store := zruntime.ClaimStore{Paths: app.Paths, Now: app.Now}
	claims := []zruntime.Claim{
		{
			Type:      zruntime.OKFClaimType,
			ID:        lexicalOnlyID,
			Tier:      "projects",
			Status:    zruntime.ClaimStatusDraft,
			Title:     "Quantum Entanglement Observer",
			Basis:     zruntime.ClaimBasisOwner,
			CreatedAt: app.Now().UTC().Format(time.RFC3339),
			CreatedBy: "owner",
			Tags:      []string{"memory"},
			Body:      "quantum entanglement observer discovery\n",
		},
		{
			Type:      zruntime.OKFClaimType,
			ID:        vectorOnlyID,
			Tier:      "projects",
			Status:    zruntime.ClaimStatusDraft,
			Title:     "Entangled Observer",
			Basis:     zruntime.ClaimBasisOwner,
			CreatedAt: app.Now().UTC().Format(time.RFC3339),
			CreatedBy: "owner",
			Tags:      []string{"memory"},
			Body:      "entanglement observer phenomena\n",
		},
	}
	for _, claim := range claims {
		if _, err := store.WriteDraft("research", claim); err != nil {
			t.Fatalf("WriteDraft(%s) error = %v", claim.ID, err)
		}
		if _, err := store.Approve("research", claim.ID); err != nil {
			t.Fatalf("Approve(%s) error = %v", claim.ID, err)
		}
	}
	if err := app.Run([]string{"reindex", "--embed"}); err != nil {
		t.Fatalf("Run(reindex --embed) error = %v", err)
	}

	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "quantum", "entanglement", "observer"}); err != nil {
		t.Fatalf("Run(ask lexical default) error = %v", err)
	}
	var lexical struct {
		Claims []struct {
			ID string `json:"id"`
		} `json:"claims"`
	}
	decodeJSON(t, stdout(app), &lexical)
	if len(lexical.Claims) != 1 || lexical.Claims[0].ID != lexicalOnlyID {
		t.Fatalf("lexical default claims = %#v, want only %q", lexical.Claims, lexicalOnlyID)
	}

	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "--embed", "quantum", "entanglement", "observer"}); err != nil {
		t.Fatalf("Run(ask --embed) error = %v", err)
	}
	var hybrid struct {
		Claims []struct {
			ID string `json:"id"`
		} `json:"claims"`
	}
	decodeJSON(t, stdout(app), &hybrid)
	seen := map[string]bool{}
	for _, claim := range hybrid.Claims {
		seen[claim.ID] = true
	}
	if len(hybrid.Claims) < 2 || !seen[lexicalOnlyID] || !seen[vectorOnlyID] {
		t.Fatalf("hybrid claims = %#v, want both %q and %q", hybrid.Claims, lexicalOnlyID, vectorOnlyID)
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

func TestRunReindexDependencyInvalidation(t *testing.T) {
	app, _ := testApp(t)
	setupResearchApp(t, &app)
	store := zruntime.ClaimStore{Paths: app.Paths, Now: app.Now}
	makeClaim := func(id string, title string, basis zruntime.ClaimBasis) zruntime.Claim {
		return zruntime.Claim{
			Type:      zruntime.OKFClaimType,
			ID:        id,
			Tier:      "projects",
			Status:    zruntime.ClaimStatusDraft,
			Title:     title,
			Basis:     basis,
			CreatedAt: app.Now().UTC().Format(time.RFC3339),
			CreatedBy: "owner",
			Tags:      []string{"memory"},
			Body:      title + " trusted token\n",
		}
	}
	base := makeClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "CLI Base Support", zruntime.ClaimBasisOwner)
	middle := makeClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "CLI Middle Support", zruntime.ClaimBasisDerived)
	middle.SupportingClaimIDs = []string{base.ID}
	dependent := makeClaim("clm_cccccccccccccccccccccccccccccccc", "CLI Deep Dependent", zruntime.ClaimBasisDerived)
	dependent.SupportingClaimIDs = []string{middle.ID}
	unrelated := makeClaim("clm_dddddddddddddddddddddddddddddddd", "CLI Unrelated Trusted", zruntime.ClaimBasisOwner)
	for _, claim := range []zruntime.Claim{base, middle, dependent, unrelated} {
		if _, err := store.WriteDraft("research", claim); err != nil {
			t.Fatalf("WriteDraft(%s) error = %v", claim.ID, err)
		}
		if _, err := store.Approve("research", claim.ID); err != nil {
			t.Fatalf("Approve(%s) error = %v", claim.ID, err)
		}
	}
	middlePath := filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects", middle.ID+".md")
	dependentPath := filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects", dependent.ID+".md")
	middleBefore, err := os.ReadFile(middlePath)
	if err != nil {
		t.Fatalf("ReadFile(middle) error = %v", err)
	}
	dependentBefore, err := os.ReadFile(dependentPath)
	if err != nil {
		t.Fatalf("ReadFile(dependent) error = %v", err)
	}

	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"reindex"}); err != nil {
		t.Fatalf("Run(initial reindex) error = %v", err)
	}
	var initial struct {
		RebuildState string `json:"rebuild_state"`
		InvalidCount int    `json:"invalid_count"`
	}
	decodeJSON(t, stdout(app), &initial)
	if initial.RebuildState != "clean" || initial.InvalidCount != 0 {
		t.Fatalf("initial reindex = %#v, want clean", initial)
	}

	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"claim", "revoke", base.ID, "--reason", "support withdrawn"}); err != nil {
		t.Fatalf("Run(claim revoke) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"reindex"}); err != nil {
		t.Fatalf("Run(reindex) error = %v", err)
	}
	var summary struct {
		Approved      int    `json:"approved"`
		Invalid       int    `json:"invalid"`
		InvalidCount  int    `json:"invalid_count"`
		RebuildState  string `json:"rebuild_state"`
		InvalidClaims []struct {
			Path  string `json:"path"`
			Error string `json:"error"`
		} `json:"invalid_claims"`
	}
	decodeJSON(t, stdout(app), &summary)
	if summary.RebuildState != "rejected" || summary.Invalid != 2 || summary.InvalidCount != 2 || len(summary.InvalidClaims) != 2 {
		t.Fatalf("rejected reindex = %#v, want two dependent roots rejected", summary)
	}
	if summary.InvalidClaims[0].Path != "projects/"+middle.ID+".md" || summary.InvalidClaims[1].Path != "projects/"+dependent.ID+".md" {
		t.Fatalf("invalid claims = %#v, want deterministic paths", summary.InvalidClaims)
	}
	for _, invalid := range summary.InvalidClaims {
		if !strings.Contains(invalid.Error, base.ID) || !strings.Contains(invalid.Error, "revoked") {
			t.Fatalf("invalid claim = %#v, want revoked base path/reason", invalid)
		}
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "CLI", "Unrelated"}); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("Run(ask) error = %v, want rejected error", err)
	}
	if stdout(app) != "" {
		t.Fatalf("ask wrote output for rejected dependency index: %q", stdout(app))
	}
	middleAfter, err := os.ReadFile(middlePath)
	if err != nil {
		t.Fatalf("ReadFile(middle after) error = %v", err)
	}
	dependentAfter, err := os.ReadFile(dependentPath)
	if err != nil {
		t.Fatalf("ReadFile(dependent after) error = %v", err)
	}
	if !bytes.Equal(middleBefore, middleAfter) || !bytes.Equal(dependentBefore, dependentAfter) {
		t.Fatalf("reindex changed dependent canonical bytes")
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

func TestRunReindexRecoversPendingTransition(t *testing.T) {
	app, _ := testApp(t)
	setupResearchApp(t, &app)
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("Recovery claim\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Recovery claim", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft) error = %v", err)
	}
	var draft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &draft)
	claimPath := filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects", draft.ID+".md")
	before, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(draft) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"claim", "approve", draft.ID}); err != nil {
		t.Fatalf("Run(claim approve) error = %v", err)
	}
	target, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(approved) error = %v", err)
	}
	if err := os.WriteFile(claimPath, before, 0o644); err != nil {
		t.Fatalf("WriteFile(preimage) error = %v", err)
	}
	if err := zruntime.WritePendingTransition(app.Paths, "research", zruntime.PendingTransition{
		OperationID: "txn_cli_reindex",
		Kind:        zruntime.ClaimTransitionApprove,
		Workspace:   "research",
		Targets:     []zruntime.PendingTransitionTarget{makePendingTransitionTarget("wiki/projects/"+draft.ID+".md", before, target)},
	}); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"reindex"}); err != nil {
		t.Fatalf("Run(reindex) error = %v", err)
	}
	var summary struct {
		Approved     int    `json:"approved"`
		RebuildState string `json:"rebuild_state"`
		InvalidCount int    `json:"invalid_count"`
	}
	decodeJSON(t, stdout(app), &summary)
	if summary.Approved != 1 || summary.RebuildState != "clean" || summary.InvalidCount != 0 {
		t.Fatalf("reindex recovery summary = %#v", summary)
	}
	contents, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(recovered claim) error = %v", err)
	}
	if !bytes.Equal(contents, target) {
		t.Fatalf("reindex did not recover target bytes")
	}
	if _, err := zruntime.ReadPendingTransition(app.Paths, "research"); !os.IsNotExist(err) {
		t.Fatalf("pending journal after reindex = %v, want missing", err)
	}
}

func TestRunAskBlocksPendingTransition(t *testing.T) {
	app, _ := testApp(t)
	setupResearchApp(t, &app)
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("Pending ask claim\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Pending ask claim", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft) error = %v", err)
	}
	var draft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &draft)
	if err := app.Run([]string{"claim", "approve", draft.ID}); err != nil {
		t.Fatalf("Run(claim approve) error = %v", err)
	}
	if err := app.Run([]string{"reindex"}); err != nil {
		t.Fatalf("Run(reindex) error = %v", err)
	}
	claimPath := filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects", draft.ID+".md")
	before, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(claim) error = %v", err)
	}
	target := append(append([]byte(nil), before...), []byte("pending\n")...)
	if err := zruntime.WritePendingTransition(app.Paths, "research", zruntime.PendingTransition{
		OperationID: "txn_cli_ask",
		Kind:        zruntime.ClaimTransitionSupersede,
		Workspace:   "research",
		Targets:     []zruntime.PendingTransitionTarget{makePendingTransitionTarget("wiki/projects/"+draft.ID+".md", before, target)},
	}); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "pending", "ask"}); err == nil || !strings.Contains(err.Error(), "pending transition") {
		t.Fatalf("Run(ask) error = %v, want pending transition error", err)
	}
	if stdout(app) != "" {
		t.Fatalf("ask wrote output while transition was pending: %q", stdout(app))
	}
	if _, err := zruntime.ReadPendingTransition(app.Paths, "research"); err != nil {
		t.Fatalf("ReadPendingTransition() error = %v, want journal preserved", err)
	}
}

func TestRunClaimMutationRecoversPendingTransition(t *testing.T) {
	app, _ := testApp(t)
	setupResearchApp(t, &app)
	recoveryPath := filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects", "recovery.md")
	before := []byte("before claim mutation\n")
	target := []byte("recovered before claim mutation\n")
	if err := os.WriteFile(recoveryPath, before, 0o644); err != nil {
		t.Fatalf("WriteFile(before) error = %v", err)
	}
	if err := zruntime.WritePendingTransition(app.Paths, "research", zruntime.PendingTransition{
		OperationID: "txn_cli_claim",
		Kind:        zruntime.ClaimTransitionSupersede,
		Workspace:   "research",
		Targets:     []zruntime.PendingTransitionTarget{makePendingTransitionTarget("wiki/projects/recovery.md", before, target)},
	}); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("Claim after recovery\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Claim after recovery", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft) error = %v", err)
	}
	recovered, err := os.ReadFile(recoveryPath)
	if err != nil {
		t.Fatalf("ReadFile(recovered) error = %v", err)
	}
	if !bytes.Equal(recovered, target) {
		t.Fatalf("claim mutation did not recover target bytes")
	}
	if _, err := zruntime.ReadPendingTransition(app.Paths, "research"); !os.IsNotExist(err) {
		t.Fatalf("pending journal after claim mutation = %v, want missing", err)
	}
}

func makePendingTransitionTarget(path string, before []byte, target []byte) zruntime.PendingTransitionTarget {
	hash := func(contents []byte) string {
		sum := sha256.Sum256(contents)
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	return zruntime.PendingTransitionTarget{
		Path:           path,
		PreimageSHA256: hash(before),
		TargetSHA256:   hash(target),
		TargetBytes:    target,
	}
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

func TestApprovalCeremony(t *testing.T) {
	app, _ := testApp(t)
	setupResearchApp(t, &app)
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("Ceremony claim\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Ceremony Claim", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft) error = %v", err)
	}
	var draft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &draft)

	claimStore := zruntime.ClaimStore{Paths: app.Paths, Now: app.Now}
	prepared, err := claimStore.PrepareChallenge("research", zruntime.ChallengePrepare{
		Workspace: "research",
		Operation: zruntime.ChallengeOperationApprove,
		ClaimID:   draft.ID,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	challengeID := prepared.Challenge.ID
	suffix := actionDigestSuffix(prepared.Challenge.ActionDigest)
	challengeStore := zruntime.ChallengeStore{Paths: app.Paths, Now: app.Now}

	// Show prints the challenge summary as JSON.
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"approval", "show", challengeID}); err != nil {
		t.Fatalf("Run(approval show) error = %v", err)
	}
	var shown struct {
		SchemaVersion      int    `json:"schema_version"`
		ChallengeID        string `json:"challenge_id"`
		ActionDigestSuffix string `json:"action_digest_suffix"`
		Operation          string `json:"operation"`
		ClaimID            string `json:"claim_id"`
		Workspace          string `json:"workspace"`
		ExpiresAt          string `json:"expires_at"`
		TokenExpiresAt     string `json:"token_expires_at"`
	}
	decodeJSON(t, stdout(app), &shown)
	if shown.SchemaVersion != 1 || shown.ChallengeID != challengeID || shown.ActionDigestSuffix != suffix || shown.Operation != "approve" || shown.ClaimID != draft.ID || shown.Workspace != "research" || shown.ExpiresAt != prepared.Challenge.ExpiresAt || shown.TokenExpiresAt != "" {
		t.Fatalf("show output = %#v", shown)
	}

	if _, err := claimStore.ApplyChallenge("research", challengeID, "not-issued", zruntime.ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "has not been owner-granted") {
		t.Fatalf("ApplyChallenge(before grant) error = %v, want owner-grant rejection", err)
	}
	unchanged, err := claimStore.Read("research", draft.ID)
	if err != nil {
		t.Fatalf("Read(after ungranted apply) error = %v", err)
	}
	if unchanged.Status != zruntime.ClaimStatusDraft || len(unchanged.Transitions) != 0 {
		t.Fatalf("claim changed after ungranted apply: %#v", unchanged)
	}
	ungranted, err := challengeStore.Read("research", challengeID)
	if err != nil {
		t.Fatalf("Read(challenge after ungranted apply) error = %v", err)
	}
	if ungranted.Granted || ungranted.Consumed {
		t.Fatalf("challenge changed after ungranted apply: %#v", ungranted)
	}

	// Grant with the correct suffix issues a fresh token without consuming it.
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader(suffix + "\n")
	if err := app.Run([]string{"approval", "grant", challengeID}); err != nil {
		t.Fatalf("Run(approval grant) error = %v", err)
	}
	var granted struct {
		SchemaVersion int    `json:"schema_version"`
		ChallengeID   string `json:"challenge_id"`
		Token         string `json:"token"`
	}
	decodeJSON(t, stdout(app), &granted)
	if granted.SchemaVersion != 1 || granted.ChallengeID != challengeID || granted.Token == "" {
		t.Fatalf("grant output = %#v", granted)
	}
	if strings.Contains(app.Stderr.(*bytes.Buffer).String(), granted.Token) {
		t.Fatalf("grant logged plaintext token: %q", granted.Token)
	}
	token := granted.Token
	unconsumed, err := challengeStore.Read("research", challengeID)
	if err != nil {
		t.Fatalf("Read(after grant) error = %v", err)
	}
	if !unconsumed.Granted || unconsumed.GrantedAt == "" {
		t.Fatalf("grant did not persist owner approval: %#v", unconsumed)
	}
	if unconsumed.Consumed {
		t.Fatal("grant consumed the challenge before apply")
	}

	// The verified token can later be consumed by the lifecycle apply exactly once.
	approved, err := claimStore.ApplyChallenge("research", challengeID, token, zruntime.ClaimMutationOptions{})
	if err != nil {
		t.Fatalf("ApplyChallenge(after grant) error = %v", err)
	}
	if approved.ID != draft.ID || approved.Status != zruntime.ClaimStatusApproved {
		t.Fatalf("ApplyChallenge(after grant) claim = %#v", approved)
	}
	if _, err := claimStore.ApplyChallenge("research", challengeID, token, zruntime.ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("ApplyChallenge(replay) error = %v, want already consumed", err)
	}

	// An incorrect suffix fails before a token is issued.
	second, err := challengeStore.Prepare("research", zruntime.ChallengePrepare{
		Workspace: "research",
		Operation: zruntime.ChallengeOperationApprove,
		ClaimID:   draft.ID,
	})
	if err != nil {
		t.Fatalf("Prepare(second) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("0000000000000000\n")
	if err := app.Run([]string{"approval", "grant", second.Challenge.ID}); err == nil || !strings.Contains(err.Error(), "suffix") {
		t.Fatalf("Run(approval grant wrong suffix) error = %v, want suffix mismatch", err)
	}
	if secondGranted, err := challengeStore.Read("research", second.Challenge.ID); err != nil {
		t.Fatalf("Read(second challenge) error = %v", err)
	} else if secondGranted.Granted || secondGranted.TokenSHA256 != "" {
		t.Fatalf("wrong suffix changed challenge: %#v", secondGranted)
	}

	third, err := challengeStore.Prepare("research", zruntime.ChallengePrepare{
		Workspace: "research",
		Operation: zruntime.ChallengeOperationApprove,
		ClaimID:   draft.ID,
	})
	if err != nil {
		t.Fatalf("Prepare(third) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader(actionDigestSuffix(third.Challenge.ActionDigest) + "\n")
	if err := app.Run([]string{"approval", "grant", third.Challenge.ID}); err != nil {
		t.Fatalf("Run(approval grant third) error = %v", err)
	}
	var thirdGranted struct {
		Token string `json:"token"`
	}
	decodeJSON(t, stdout(app), &thirdGranted)
	if thirdGranted.Token == "" {
		t.Fatal("third grant returned an empty token")
	}
	if _, err := challengeStore.Verify("research", third.Challenge.ID, thirdGranted.Token); err != nil {
		t.Fatalf("Verify(third after grant) error = %v", err)
	}
	if _, err := claimStore.ApplyChallenge("research", third.Challenge.ID, "wrong-token", zruntime.ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "token mismatch") {
		t.Fatalf("ApplyChallenge(wrong token) error = %v, want token mismatch", err)
	}
}

func TestApprovalGrantRequiresTTY(t *testing.T) {
	app, _ := testApp(t)
	setupResearchApp(t, &app)
	store := zruntime.ChallengeStore{Paths: app.Paths, Now: app.Now}
	prepared, err := store.Prepare("research", zruntime.ChallengePrepare{
		Workspace: "research",
		Operation: zruntime.ChallengeOperationApprove,
		ClaimID:   "clm_0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("Open(os.DevNull) error = %v", err)
	}
	defer func() { _ = devNull.Close() }()
	app.Stdin = devNull
	if err := app.Run([]string{"approval", "grant", prepared.Challenge.ID}); err == nil || !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("Run(approval grant non-TTY) error = %v, want interactive terminal error", err)
	}
}

func TestExitCodes(t *testing.T) {
	exitCode := func(err error) int {
		if err == nil {
			return 0
		}
		if c, ok := err.(interface{ ExitCode() int }); ok {
			return c.ExitCode()
		}
		return 1
	}

	cases := []struct {
		name    string
		args    []string
		want    int
		prepare func(*App)
	}{
		{name: "valid help", args: []string{"--help"}, want: 0},
		{name: "valid version", args: []string{"version"}, want: 0},
		{name: "valid setup", args: []string{"setup"}, want: 0},
		{name: "valid doctor healthy", args: []string{"doctor"}, want: 0, prepare: func(app *App) {
			setupResearchApp(t, app)
			if err := app.Run([]string{"reindex"}); err != nil {
				t.Fatalf("prepare reindex error = %v", err)
			}
		}},
		{name: "valid ask gap is success", args: []string{"ask", "missing"}, want: 0, prepare: func(app *App) {
			setupResearchApp(t, app)
			if err := app.Run([]string{"reindex"}); err != nil {
				t.Fatalf("prepare reindex error = %v", err)
			}
		}},
		{name: "unknown command", args: []string{"bogus"}, want: 1},
		{name: "unknown command with dash still flag", args: []string{"--bogus"}, want: 2},
		{name: "unknown flag subcommand", args: []string{"setup", "--bogus"}, want: 2},
		{name: "unknown flag ask", args: []string{"ask", "--bogus", "value"}, want: 2},
		{name: "flag requires value", args: []string{"ask", "--workspace"}, want: 2},
		{name: "help with extra arg is usage", args: []string{"--help", "extra"}, want: 2},
		{name: "version with extra arg is usage", args: []string{"version", "extra"}, want: 2},
		{name: "missing arg workspace create", args: []string{"workspace", "create"}, want: 2},
		{name: "missing arg claim approve", args: []string{"claim", "approve"}, want: 2},
		{name: "missing arg claim draft extra positional", args: []string{"claim", "draft", "extra"}, want: 2, prepare: func(app *App) { setupResearchApp(t, app) }},
		{name: "missing arg ask no query", args: []string{"ask"}, want: 2},
		{name: "missing arg evidence add", args: []string{"evidence", "add", "--file", "a"}, want: 2, prepare: func(app *App) { setupResearchApp(t, app) }},
		{name: "usage reindex extra arg", args: []string{"reindex", "extra"}, want: 2, prepare: func(app *App) { setupResearchApp(t, app) }},
		{name: "unknown claim subcommand", args: []string{"claim", "bogus"}, want: 1},
		{name: "unknown workspace subcommand", args: []string{"workspace", "bogus"}, want: 1},
		{name: "unknown approval subcommand", args: []string{"approval", "bogus"}, want: 1},
		{name: "mcp missing subcommand is usage", args: []string{"mcp"}, want: 2},
		{name: "mcp unknown subcommand is usage", args: []string{"mcp", "bogus"}, want: 2},
		{name: "runtime error approve nonexistent", args: []string{"claim", "approve", "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, want: 1, prepare: func(app *App) { setupResearchApp(t, app) }},
		{name: "runtime error resolve workspace missing", args: []string{"status"}, want: 1},
		{name: "doctor with findings dirty", args: []string{"doctor"}, want: 2, prepare: func(app *App) {
			setupResearchApp(t, app)
			if err := app.Run([]string{"reindex"}); err != nil {
				t.Fatalf("prepare reindex error = %v", err)
			}
			if err := (zruntime.IndexStore{Paths: app.Paths}).MarkDirty("research"); err != nil {
				t.Fatalf("MarkDirty error = %v", err)
			}
		}},
		{name: "doctor with findings probe-embedder no embedding", args: []string{"doctor", "--probe-embedder"}, want: 2, prepare: func(app *App) {
			setupResearchApp(t, app)
			if err := app.Run([]string{"reindex"}); err != nil {
				t.Fatalf("prepare reindex error = %v", err)
			}
		}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			app, _ := testApp(t)
			if tc.prepare != nil {
				tc.prepare(&app)
				// reset output buffers before actual run so prepare output does not interfere
				app.Stdout = &bytes.Buffer{}
				app.Stderr = &bytes.Buffer{}
				// ensure stdin is empty for commands that read body
				if app.Stdin == nil {
					app.Stdin = strings.NewReader("")
				}
			}
			err := app.Run(tc.args)
			got := exitCode(err)
			if got != tc.want {
				t.Fatalf("Run(%v) exit code = %d, want %d (err=%v)", tc.args, got, tc.want, err)
			}
		})
	}
}

func decodeJSON(t *testing.T, input string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(input), target); err != nil {
		t.Fatalf("invalid JSON %q: %v", input, err)
	}
}

func TestSurface(t *testing.T) {
	repoRoot := findRepoRoot(t)
	snapshotPath := filepath.Join(repoRoot, "docs", "proofs", "surface.txt")

	cases := []struct {
		name string
		args []string
	}{
		{name: "zbrain --help", args: []string{"--help"}},
		{name: "zbrain setup --help", args: []string{"setup", "--help"}},
		{name: "zbrain workspace --help", args: []string{"workspace", "--help"}},
		{name: "zbrain evidence --help", args: []string{"evidence", "--help"}},
		{name: "zbrain claim --help", args: []string{"claim", "--help"}},
		{name: "zbrain migrate --help", args: []string{"migrate", "--help"}},
		{name: "zbrain reindex --help", args: []string{"reindex", "--help"}},
		{name: "zbrain ask --help", args: []string{"ask", "--help"}},
		{name: "zbrain status --help", args: []string{"status", "--help"}},
		{name: "zbrain doctor --help", args: []string{"doctor", "--help"}},
		{name: "zbrain mcp --help", args: []string{"mcp", "--help"}},
		{name: "zbrain view --help", args: []string{"view", "--help"}},
		{name: "zbrain approval --help", args: []string{"approval", "--help"}},
	}

	var captured bytes.Buffer
	for _, tc := range cases {
		output := runZbrainHelp(t, repoRoot, tc.args)
		if !strings.Contains(output, "Usage:") {
			t.Fatalf("surface %q missing Usage: in output %q", tc.name, output)
		}
		if !strings.Contains(output, "zbrain") {
			t.Fatalf("surface %q missing zbrain example in output %q", tc.name, output)
		}
		fmt.Fprintf(&captured, "=== %s ===\n%s", tc.name, output)
		if !strings.HasSuffix(output, "\n") {
			captured.WriteString("\n")
		}
		captured.WriteString("\n")
	}
	if os.Getenv("UPDATE_EXPECT") == "1" {
		if err := os.WriteFile(snapshotPath, captured.Bytes(), 0o644); err != nil {
			t.Fatalf("write snapshot %s: %v", snapshotPath, err)
		}
		return
	}

	want, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot %s: %v (regenerate with: go run ./cmd/zbrain --help and 12 sub-helps > docs/proofs/surface.txt)", snapshotPath, err)
	}
	got := captured.Bytes()
	if !bytes.Equal(got, want) {
		t.Fatalf("surface snapshot mismatch at %s: got %d bytes want %d bytes\nrun the following to regenerate:\n  go run ./cmd/zbrain --help > /tmp/surface.txt and append sub-helps, or run TestSurface with UPDATE_EXPECT=1\n--- got (first 500 bytes) ---\n%s\n--- want (first 500 bytes) ---\n%s", snapshotPath, len(got), len(want), truncateForDiff(got), truncateForDiff(want))
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root (no go.mod) starting from %s", dir)
		}
		dir = parent
	}
}

func runZbrainHelp(t *testing.T, repoRoot string, args []string) string {
	t.Helper()
	cmdArgs := append([]string{"run", "./cmd/zbrain"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run ./cmd/zbrain %v failed: %v\noutput: %s", args, err, string(out))
	}
	return string(out)
}

func truncateForDiff(b []byte) string {
	const limit = 500
	if len(b) <= limit {
		return string(b)
	}
	return string(b[:limit]) + "...[truncated]"
}

func TestRunAskSurfacesConflictCandidate(t *testing.T) {
	app, _ := testApp(t)
	setupResearchApp(t, &app)

	// Draft and approve approved claim
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("zbrain uses SQLite for indexes\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "zbrain uses SQLite for indexes", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft approved) error = %v", err)
	}
	var approvedDraft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &approvedDraft)
	if err := app.Run([]string{"claim", "approve", approvedDraft.ID}); err != nil {
		t.Fatalf("Run(claim approve) error = %v", err)
	}

	// Draft contradicting claim (value swap on title)
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("zbrain uses BoltDB for indexes\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "zbrain uses BoltDB for indexes", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft conflicting) error = %v", err)
	}
	var conflictingDraft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &conflictingDraft)

	// Reindex
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"reindex"}); err != nil {
		t.Fatalf("Run(reindex) error = %v", err)
	}

	// Ask matching both
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "uses", "indexes"}); err != nil {
		t.Fatalf("Run(ask) error = %v", err)
	}
	var response struct {
		SchemaVersion int    `json:"schema_version"`
		Status        string `json:"status"`
		Claims        []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"claims"`
		PromotionCandidates []struct {
			ID          string `json:"id"`
			Status      string `json:"status"`
			Contradicts []struct {
				ClaimID   string `json:"claim_id"`
				Heuristic string `json:"heuristic"`
			} `json:"contradicts"`
		} `json:"promotion_candidates"`
	}
	decodeJSON(t, stdout(app), &response)
	if response.SchemaVersion != 1 || response.Status != "ready" {
		t.Fatalf("response status = %q, want ready", response.Status)
	}
	if len(response.Claims) != 1 || response.Claims[0].ID != approvedDraft.ID || response.Claims[0].Status != "approved" {
		t.Fatalf("response.Claims = %#v, want 1 approved claim", response.Claims)
	}
	if len(response.PromotionCandidates) != 1 {
		t.Fatalf("response.PromotionCandidates = %#v, want 1 candidate", response.PromotionCandidates)
	}
	candidate := response.PromotionCandidates[0]
	if candidate.ID != conflictingDraft.ID || candidate.Status != "conflict" {
		t.Fatalf("candidate = %#v, want status conflict", candidate)
	}
	if len(candidate.Contradicts) != 1 || candidate.Contradicts[0].ClaimID != approvedDraft.ID || candidate.Contradicts[0].Heuristic != "value_swap" {
		t.Fatalf("candidate.Contradicts = %#v, want value_swap against %s", candidate.Contradicts, approvedDraft.ID)
	}
}

func TestRunAskTemporalFlags(t *testing.T) {
	app, _ := testApp(t)
	setupResearchApp(t, &app)

	// 1. Create Early claim verified at 2026-08-01
	app.Now = func() time.Time { return time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC) }
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("temporal early decision body\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Early Decision", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft early) error = %v", err)
	}
	var earlyDraft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &earlyDraft)
	if err := app.Run([]string{"claim", "approve", earlyDraft.ID}); err != nil {
		t.Fatalf("Run(claim approve early) error = %v", err)
	}

	// 2. Create Late claim verified at 2026-08-28
	app.Now = func() time.Time { return time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC) }
	app.Stdout = &bytes.Buffer{}
	app.Stdin = strings.NewReader("temporal late decision body\n")
	if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", "Late Decision", "--basis", "owner"}); err != nil {
		t.Fatalf("Run(claim draft late) error = %v", err)
	}
	var lateDraft struct {
		ID string `json:"id"`
	}
	decodeJSON(t, stdout(app), &lateDraft)
	if err := app.Run([]string{"claim", "approve", lateDraft.ID}); err != nil {
		t.Fatalf("Run(claim approve late) error = %v", err)
	}

	// Reindex
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"reindex"}); err != nil {
		t.Fatalf("Run(reindex) error = %v", err)
	}

	type askResponse struct {
		Status string `json:"status"`
		Claims []struct {
			ID string `json:"id"`
		} `json:"claims"`
	}

	// Test --after 2026-08-10 returns only Late
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "--after", "2026-08-10T00:00:00Z", "temporal", "decision"}); err != nil {
		t.Fatalf("Run(ask --after) error = %v", err)
	}
	var afterRes askResponse
	decodeJSON(t, stdout(app), &afterRes)
	if afterRes.Status != "ready" || len(afterRes.Claims) != 1 || afterRes.Claims[0].ID != lateDraft.ID {
		t.Fatalf("ask --after response = %#v, want only late claim", afterRes)
	}

	// Test --before 2026-08-10 returns only Early
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "--before", "2026-08-10T00:00:00Z", "temporal", "decision"}); err != nil {
		t.Fatalf("Run(ask --before) error = %v", err)
	}
	var beforeRes askResponse
	decodeJSON(t, stdout(app), &beforeRes)
	if beforeRes.Status != "ready" || len(beforeRes.Claims) != 1 || beforeRes.Claims[0].ID != earlyDraft.ID {
		t.Fatalf("ask --before response = %#v, want only early claim", beforeRes)
	}

	// Test --as-of 2026-08-05 returns only Early
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "--as-of", "2026-08-05T00:00:00Z", "temporal", "decision"}); err != nil {
		t.Fatalf("Run(ask --as-of) error = %v", err)
	}
	var asOfRes askResponse
	decodeJSON(t, stdout(app), &asOfRes)
	if asOfRes.Status != "ready" || len(asOfRes.Claims) != 1 || asOfRes.Claims[0].ID != earlyDraft.ID {
		t.Fatalf("ask --as-of response = %#v, want only early claim", asOfRes)
	}

	// Test invalid timestamp returns error
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "--after", "bad-date", "temporal"}); err == nil {
		t.Fatalf("Run(ask invalid timestamp) error = nil, want error")
	}
}
