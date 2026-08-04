package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

const Version = "0.1.0-go"

type App struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	Paths  zruntime.Paths
	Now    func() time.Time
}

func New() (App, error) {
	paths, err := zruntime.ResolvePaths(zruntime.Options{})
	if err != nil {
		return App{}, err
	}
	return App{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
		Paths:  paths,
		Now:    time.Now,
	}, nil
}

func (app App) Run(args []string) error {
	if len(args) == 0 {
		app.printHelp()
		return nil
	}

	switch args[0] {
	case "--help", "-h", "help":
		app.printHelp()
		return nil
	case "--version", "version":
		_, err := fmt.Fprintln(app.Stdout, Version)
		return err
	case "setup":
		return app.runSetup(args[1:])
	case "workspace":
		return app.runWorkspace(args[1:])
	case "evidence":
		return app.runEvidence(args[1:])
	case "claim":
		return app.runClaim(args[1:])
	case "migrate":
		return app.runMigrate(args[1:])
	case "reindex":
		return app.runReindex(args[1:])
	case "ask":
		return app.runAsk(args[1:])
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func (app App) runSetup(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("setup accepts no arguments")
	}
	if err := os.MkdirAll(app.Paths.RuntimeDir, 0o755); err != nil {
		return err
	}
	created, err := zruntime.EnsureConfig(app.Paths.ConfigFile)
	if err != nil {
		return err
	}
	extracted, err := zruntime.ExtractBundledAssets(app.Paths)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(app.Stdout, "zbrain setup complete\nruntime: %s\nconfig_created: %t\nassets_copied: %d\nassets_skipped: %d\n", app.Paths.RuntimeDir, created, len(extracted.Copied), len(extracted.Skipped))
	return err
}

func (app App) runAsk(args []string) error {
	workspace, includes, rest, err := parseWorkspaceIncludeFlags(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return errors.New("usage: zbrain ask [--workspace <name>] [--include <name>]... <query>")
	}
	response, err := zruntime.TrustedQuery(app.Paths, zruntime.TrustedQueryOptions{Workspace: workspace, Includes: includes, Query: strings.Join(rest, " "), Limit: 10})
	if err != nil {
		return err
	}
	return writeJSON(app.Stdout, response)
}

func (app App) runReindex(args []string) error {
	workspace, rest, err := parseWorkspaceFlag(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return errors.New("usage: zbrain reindex [--workspace <name>]")
	}
	workspace, err = app.resolveWorkspace(workspace)
	if err != nil {
		return err
	}
	summary, err := (zruntime.IndexStore{Paths: app.Paths}).Rebuild(workspace)
	if err != nil {
		return err
	}
	return writeJSON(app.Stdout, struct {
		SchemaVersion int `json:"schema_version"`
		zruntime.IndexSummary
	}{SchemaVersion: 1, IndexSummary: summary})
}

func (app App) runEvidence(args []string) error {
	if len(args) == 0 || args[0] != "add" {
		return errors.New("evidence requires subcommand: add")
	}
	flags, rest := parseFlags(args[1:])
	if len(rest) > 0 {
		return errors.New("usage: zbrain evidence add --file <path> --origin <uri-or-path> [--media-type <type>] [--workspace <name>]")
	}
	file := flags.single("file")
	origin := flags.single("origin")
	if file == "" || origin == "" {
		return errors.New("evidence add requires --file and --origin")
	}
	workspace, err := app.resolveWorkspace(flags.single("workspace"))
	if err != nil {
		return err
	}
	evidence, err := (zruntime.EvidenceStore{Paths: app.Paths, Now: app.Now}).AddFile(workspace, file, origin, flags.single("media-type"))
	if err != nil {
		return err
	}
	return writeJSON(app.Stdout, struct {
		SchemaVersion int    `json:"schema_version"`
		Workspace     string `json:"workspace"`
		zruntime.Evidence
	}{SchemaVersion: 1, Workspace: workspace, Evidence: evidence})
}

func (app App) runClaim(args []string) error {
	if len(args) == 0 {
		return errors.New("claim requires a subcommand: draft, approve, supersede, or revoke")
	}
	switch args[0] {
	case "draft":
		return app.runClaimDraft(args[1:])
	case "approve":
		return app.runClaimApprove(args[1:])
	case "supersede":
		return app.runClaimSupersede(args[1:])
	case "revoke":
		return app.runClaimRevoke(args[1:])
	default:
		return fmt.Errorf("unknown claim subcommand: %s", args[0])
	}
}

func (app App) runClaimDraft(args []string) error {
	flags, rest := parseFlags(args)
	if len(rest) > 0 {
		return errors.New("usage: zbrain claim draft --tier <tier> --title <title> --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]")
	}
	workspace, err := app.resolveWorkspace(flags.single("workspace"))
	if err != nil {
		return err
	}
	body, err := io.ReadAll(app.Stdin)
	if err != nil {
		return err
	}
	id, err := zruntime.NewClaimID()
	if err != nil {
		return err
	}
	now := time.Now
	if app.Now != nil {
		now = app.Now
	}
	claim := zruntime.Claim{
		Type:               zruntime.OKFClaimType,
		ID:                 id,
		Tier:               flags.single("tier"),
		Status:             zruntime.ClaimStatusDraft,
		Title:              flags.single("title"),
		Basis:              zruntime.ClaimBasis(flags.single("basis")),
		CreatedAt:          now().UTC().Format(time.RFC3339),
		CreatedBy:          "owner",
		EvidenceIDs:        flags.values["evidence"],
		SupportingClaimIDs: flags.values["support"],
		ConflictsWith:      flags.values["conflicts-with"],
		Body:               string(body),
	}
	if err := (zruntime.IndexStore{Paths: app.Paths}).MarkDirty(workspace); err != nil {
		return err
	}
	created, err := (zruntime.ClaimStore{Paths: app.Paths, Now: app.Now}).WriteDraft(workspace, claim)
	if err != nil {
		return err
	}
	return app.writeClaimMutation(workspace, created)
}

func (app App) runClaimApprove(args []string) error {
	workspace, rest, err := parseWorkspaceFlag(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return errors.New("usage: zbrain claim approve <id> [--workspace <name>]")
	}
	workspace, err = app.resolveWorkspace(workspace)
	if err != nil {
		return err
	}
	if err := (zruntime.IndexStore{Paths: app.Paths}).MarkDirty(workspace); err != nil {
		return err
	}
	claim, err := (zruntime.ClaimStore{Paths: app.Paths, Now: app.Now}).Approve(workspace, rest[0])
	if err != nil {
		return err
	}
	return app.writeClaimMutation(workspace, claim)
}

func (app App) runClaimSupersede(args []string) error {
	flags, rest := parseFlags(args)
	if len(rest) != 1 {
		return errors.New("usage: zbrain claim supersede <id> --tier <tier> --title <title> --basis <owner|evidence|derived> ...")
	}
	workspace, err := app.resolveWorkspace(flags.single("workspace"))
	if err != nil {
		return err
	}
	body, err := io.ReadAll(app.Stdin)
	if err != nil {
		return err
	}
	id, err := zruntime.NewClaimID()
	if err != nil {
		return err
	}
	now := time.Now
	if app.Now != nil {
		now = app.Now
	}
	replacement := zruntime.Claim{Type: zruntime.OKFClaimType, ID: id, Tier: flags.single("tier"), Status: zruntime.ClaimStatusDraft, Title: flags.single("title"), Basis: zruntime.ClaimBasis(flags.single("basis")), CreatedAt: now().UTC().Format(time.RFC3339), CreatedBy: "owner", EvidenceIDs: flags.values["evidence"], SupportingClaimIDs: flags.values["support"], ConflictsWith: flags.values["conflicts-with"], Body: string(body)}
	if err := (zruntime.IndexStore{Paths: app.Paths}).MarkDirty(workspace); err != nil {
		return err
	}
	claim, err := (zruntime.ClaimStore{Paths: app.Paths, Now: app.Now}).WriteSupersedingDraft(workspace, rest[0], replacement)
	if err != nil {
		return err
	}
	return app.writeClaimMutation(workspace, claim)
}

func (app App) runClaimRevoke(args []string) error {
	flags, rest := parseFlags(args)
	if len(rest) != 1 || flags.single("reason") == "" {
		return errors.New("usage: zbrain claim revoke <id> --reason <text> [--workspace <name>]")
	}
	workspace, err := app.resolveWorkspace(flags.single("workspace"))
	if err != nil {
		return err
	}
	if err := (zruntime.IndexStore{Paths: app.Paths}).MarkDirty(workspace); err != nil {
		return err
	}
	claim, err := (zruntime.ClaimStore{Paths: app.Paths, Now: app.Now}).Revoke(workspace, rest[0], flags.single("reason"))
	if err != nil {
		return err
	}
	return app.writeClaimMutation(workspace, claim)
}

func (app App) runMigrate(args []string) error {
	if len(args) == 0 || args[0] != "okf" {
		return errors.New("migrate requires subcommand: okf")
	}
	workspace, rest, err := parseWorkspaceFlag(args[1:])
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return errors.New("usage: zbrain migrate okf [--workspace <name>]")
	}
	workspace, err = app.resolveWorkspace(workspace)
	if err != nil {
		return err
	}
	claimStore := zruntime.ClaimStore{Paths: app.Paths, Now: app.Now}
	scan, err := claimStore.ScanWorkspace(workspace)
	if err != nil {
		return err
	}
	needsMigration := false
	for _, claim := range scan.Claims {
		if claim.Schema == zruntime.ClaimSchemaVersion {
			needsMigration = true
			break
		}
	}
	if needsMigration {
		if err := (zruntime.IndexStore{Paths: app.Paths}).MarkDirty(workspace); err != nil {
			return err
		}
	}
	summary, err := claimStore.MigrateOKF(workspace)
	if err != nil {
		return err
	}
	return writeJSON(app.Stdout, struct {
		SchemaVersion int  `json:"schema_version"`
		IndexFresh    bool `json:"index_fresh"`
		zruntime.ClaimMigrationSummary
	}{SchemaVersion: 1, IndexFresh: summary.Migrated == 0, ClaimMigrationSummary: summary})
}

func (app App) writeClaimMutation(workspace string, claim zruntime.Claim) error {
	return writeJSON(app.Stdout, struct {
		SchemaVersion int                  `json:"schema_version"`
		Workspace     string               `json:"workspace"`
		ID            string               `json:"id"`
		Status        zruntime.ClaimStatus `json:"status"`
		Path          string               `json:"path"`
		IndexFresh    bool                 `json:"index_fresh"`
	}{SchemaVersion: 1, Workspace: workspace, ID: claim.ID, Status: claim.Status, Path: claim.Path, IndexFresh: false})
}

func (app App) runWorkspace(args []string) error {
	if len(args) == 0 {
		return errors.New("workspace requires a subcommand: create or current")
	}
	switch args[0] {
	case "create":
		if len(args) != 2 {
			return errors.New("usage: zbrain workspace create <name>")
		}
		if err := zruntime.CreateWorkspace(app.Paths, args[1], app.Now()); err != nil {
			return err
		}
		_, err := fmt.Fprintf(app.Stdout, "workspace created: %s\n", args[1])
		return err
	case "current":
		if len(args) > 1 {
			return errors.New("workspace current accepts no arguments")
		}
		current, err := zruntime.ResolveCurrentWorkspace(app.Paths)
		if err != nil {
			return err
		}
		encoded, err := zruntime.MarshalCurrent(current)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(app.Stdout, "%s\n", encoded)
		return err
	default:
		return fmt.Errorf("unknown workspace subcommand: %s", args[0])
	}
}

func (app App) resolveWorkspace(workspace string) (string, error) {
	if workspace == "" {
		current, err := zruntime.ResolveCurrentWorkspace(app.Paths)
		if err != nil {
			return "", err
		}
		workspace = current.Workspace
	}
	if _, err := zruntime.ValidateWorkspace(app.Paths, workspace); err != nil {
		return "", err
	}
	return workspace, nil
}

func writeJSON(writer io.Writer, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "%s\n", encoded)
	return err
}

type parsedFlags struct{ values map[string][]string }

func (flags parsedFlags) single(name string) string {
	values := flags.values[name]
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func parseFlags(args []string) (parsedFlags, []string) {
	flags := parsedFlags{values: map[string][]string{}}
	rest := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") && i+1 < len(args) {
			name := strings.TrimPrefix(arg, "--")
			flags.values[name] = append(flags.values[name], args[i+1])
			i++
			continue
		}
		rest = append(rest, arg)
	}
	return flags, rest
}

func parseWorkspaceFlag(args []string) (string, []string, error) {
	flags, rest := parseFlags(args)
	return flags.single("workspace"), rest, nil
}

func parseWorkspaceIncludeFlags(args []string) (string, []string, []string, error) {
	flags, rest := parseFlags(args)
	return flags.single("workspace"), flags.values["include"], rest, nil
}

func (app App) printHelp() {
	fmt.Fprint(app.Stdout, `zbrain - Go-native OKF trusted memory CLI

Usage:
  zbrain <command> [arguments]

Commands:
  setup                         Prepare the runtime directory
  workspace create <name>       Create a workspace
  workspace current             Print the active workspace as JSON
  evidence add                  Capture an immutable local evidence snapshot
  claim draft                   Write a draft OKF claim concept from stdin
  claim approve <id>            Promote a valid draft claim
  claim supersede <id>          Create a replacement draft for an approved claim
  claim revoke <id>             Revoke a claim with a reason
  migrate okf                   Convert legacy zbrain claims to OKF concepts
  reindex                       Rebuild the derived workspace index
  ask <query>                   Return trusted context JSON; does not call an LLM
  version                       Print version
`)
}
