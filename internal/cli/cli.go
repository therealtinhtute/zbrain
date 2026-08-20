package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	zmcp "github.com/therealtinhtute/zbrain/internal/mcp"
	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
	"github.com/therealtinhtute/zbrain/internal/view"
)

const Version = "0.1.1"

var (
	noFlags  = map[string]struct{}{}
	askFlags = map[string]struct{}{
		"workspace": {},
		"include":   {},
	}
	evidenceAddFlags = map[string]struct{}{
		"file":       {},
		"origin":     {},
		"media-type": {},
		"workspace":  {},
	}
	claimDraftFlags = map[string]struct{}{
		"tier":           {},
		"title":          {},
		"basis":          {},
		"evidence":       {},
		"support":        {},
		"conflicts-with": {},
		"workspace":      {},
	}
	claimSupersedeFlags = map[string]struct{}{
		"tier":           {},
		"title":          {},
		"basis":          {},
		"evidence":       {},
		"support":        {},
		"conflicts-with": {},
		"workspace":      {},
	}
	claimRevokeFlags = map[string]struct{}{
		"reason":    {},
		"workspace": {},
	}
	workspaceFlags = map[string]struct{}{
		"workspace": {},
	}
)

type App struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	Paths  zruntime.Paths
	Now    func() time.Time
}

type commandExitError struct {
	Code    int
	Message string
}

func (e commandExitError) Error() string { return e.Message }
func (e commandExitError) ExitCode() int { return e.Code }
func exitCodeError(code int, message string) error {
	return commandExitError{Code: code, Message: message}
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
		if len(args) != 1 {
			return errors.New("usage: zbrain --help")
		}
		app.printHelp()
		return nil
	case "--version", "version":
		if helpRequested(args[1:]) {
			app.printVersionHelp()
			return nil
		}
		if len(args) > 1 {
			if strings.HasPrefix(args[1], "-") {
				return unknownFlag(args[1])
			}
			return errors.New("version accepts no arguments")
		}
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
	case "status":
		return app.runStatus(args[1:])
	case "doctor":
		return app.runDoctor(args[1:])
	case "mcp":
		return app.runMCP(args[1:])
	case "view":
		return app.runView(args[1:])
	default:
		if strings.HasPrefix(args[0], "-") {
			return unknownFlag(args[0])
		}
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func (app App) runStatus(args []string) error {
	if helpRequested(args) {
		fmt.Fprintln(app.Stdout, "Usage: zbrain status [--workspace <name>]")
		return nil
	}
	workspace, rest, err := parseWorkspaceFlag(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return errors.New("usage: zbrain status [--workspace <name>]")
	}
	workspace, err = app.resolveWorkspace(workspace)
	if err != nil {
		return err
	}
	idx := zruntime.IndexStore{Paths: app.Paths}
	summary := zruntime.IndexSummary{Workspace: workspace, Embedding: zruntime.EmbeddingSummary{Strategy: "lexical", Degraded: "embeddings not configured"}}
	if scan, scanErr := (zruntime.ClaimStore{Paths: app.Paths, Now: app.Now}).ScanWorkspaceForTrust(workspace); scanErr == nil {
		summary.Approved = len(scan.Claims)
		summary.Invalid = len(scan.Invalid)
		summary.InvalidCount = len(scan.Invalid)
		summary.InvalidClaims = scan.Invalid
	}
	if err := idx.CheckFresh(workspace); err != nil {
		summary.RebuildState = zruntime.RebuildStatusRejected
		if summary.Invalid == 0 {
			summary.InvalidClaims = []zruntime.InvalidClaim{{Path: "", Error: err.Error()}}
		}
	}
	return writeJSON(app.Stdout, struct {
		SchemaVersion int `json:"schema_version"`
		zruntime.IndexSummary
	}{2, summary})
}

func (app App) runDoctor(args []string) error {
	if helpRequested(args) {
		fmt.Fprintln(app.Stdout, "Usage: zbrain doctor [--workspace <name>] [--probe-embedder]")
		return nil
	}
	probe := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--probe-embedder" {
			probe = true
			continue
		}
		filtered = append(filtered, arg)
	}
	workspace, rest, err := parseWorkspaceFlag(filtered)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return errors.New("usage: zbrain doctor [--workspace <name>] [--probe-embedder]")
	}
	workspace, err = app.resolveWorkspace(workspace)
	if err != nil {
		return err
	}
	findings := []string{}
	if err := (zruntime.IndexStore{Paths: app.Paths}).CheckFresh(workspace); err != nil {
		findings = append(findings, err.Error())
	}
	if probe {
		embStore := zruntime.EmbeddingStore{Paths: app.Paths}
		count, err := embStore.Count(workspace)
		if err != nil {
			findings = append(findings, fmt.Sprintf("embedder probe error: %v", err))
		} else if count == 0 {
			findings = append(findings, "embedder probe unavailable: no embeddings found; run zbrain reindex --embed")
		}
	}
	status := "healthy"
	if len(findings) > 0 {
		status = "degraded"
	}
	if err := writeJSON(app.Stdout, struct {
		SchemaVersion int      `json:"schema_version"`
		Workspace     string   `json:"workspace"`
		Status        string   `json:"status"`
		Findings      []string `json:"findings"`
		NextAction    string   `json:"next_action"`
	}{2, workspace, status, findings, "zbrain reindex"}); err != nil {
		return err
	}
	if len(findings) > 0 {
		return exitCodeError(2, "doctor found domain findings")
	}
	return nil
}

func (app App) runMCP(args []string) error {
	if len(args) == 0 {
		return errors.New("mcp requires a subcommand: serve")
	}
	if helpRequested(args) {
		app.printMCPHelp()
		return nil
	}
	if strings.HasPrefix(args[0], "-") {
		return unknownFlag(args[0])
	}
	if args[0] != "serve" {
		return errors.New("mcp requires subcommand: serve")
	}
	if helpRequested(args[1:]) {
		app.printMCPServeHelp()
		return nil
	}
	_, rest, err := parseFlags(args[1:], noFlags)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return errors.New("usage: zbrain mcp serve")
	}
	return zmcp.Serve(context.Background(), zmcp.Options{
		Paths:   app.Paths,
		Now:     app.Now,
		Version: Version,
		Stderr:  app.Stderr,
	})
}

func (app App) runView(args []string) error {
	if helpRequested(args) {
		app.printViewHelp()
		return nil
	}
	_, rest, err := parseFlags(args, noFlags)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return errors.New("usage: zbrain view")
	}
	srv := view.Server{
		Stdout: app.Stdout,
		Stderr: app.Stderr,
		Paths:  app.Paths,
	}
	return srv.Run()
}

func (app App) runSetup(args []string) error {
	if helpRequested(args) {
		app.printSetupHelp()
		return nil
	}
	_, rest, err := parseFlags(args, noFlags)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
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
	if helpRequested(args) {
		app.printAskHelp()
		return nil
	}
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
	if helpRequested(args) {
		app.printReindexHelp()
		return nil
	}
	embed := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--embed" {
			embed = true
			continue
		}
		filtered = append(filtered, arg)
	}
	workspace, rest, err := parseWorkspaceFlag(filtered)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return errors.New("usage: zbrain reindex [--workspace <name>] [--embed]")
	}
	workspace, err = app.resolveWorkspace(workspace)
	if err != nil {
		return err
	}
	summary, err := (zruntime.IndexStore{Paths: app.Paths}).RebuildWithOptions(workspace, zruntime.RebuildOptions{Embedding: embed})
	if err != nil {
		return err
	}
	return writeJSON(app.Stdout, struct {
		SchemaVersion int `json:"schema_version"`
		zruntime.IndexSummary
	}{SchemaVersion: 1, IndexSummary: summary})
}

func (app App) runEvidence(args []string) error {
	if len(args) == 0 {
		return errors.New("evidence requires subcommand: add")
	}
	if helpRequested(args) {
		app.printEvidenceHelp()
		return nil
	}
	if strings.HasPrefix(args[0], "-") {
		return unknownFlag(args[0])
	}
	if args[0] != "add" {
		return errors.New("evidence requires subcommand: add")
	}
	if helpRequested(args[1:]) {
		app.printEvidenceAddHelp()
		return nil
	}
	flags, rest, err := parseFlags(args[1:], evidenceAddFlags)
	if err != nil {
		return err
	}
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
	if helpRequested(args) {
		app.printClaimHelp()
		return nil
	}
	if strings.HasPrefix(args[0], "-") {
		return unknownFlag(args[0])
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
	if helpRequested(args) {
		app.printClaimDraftHelp()
		return nil
	}
	flags, rest, err := parseFlags(args, claimDraftFlags)
	if err != nil {
		return err
	}
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
	if helpRequested(args) {
		app.printClaimApproveHelp()
		return nil
	}
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
	if helpRequested(args) {
		app.printClaimSupersedeHelp()
		return nil
	}
	flags, rest, err := parseFlags(args, claimSupersedeFlags)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return errors.New("usage: zbrain claim supersede <id> --tier <tier> --title <title> --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]")
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
	if helpRequested(args) {
		app.printClaimRevokeHelp()
		return nil
	}
	flags, rest, err := parseFlags(args, claimRevokeFlags)
	if err != nil {
		return err
	}
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
	if len(args) == 0 {
		return errors.New("migrate requires subcommand: okf")
	}
	if helpRequested(args) {
		app.printMigrateHelp()
		return nil
	}
	if strings.HasPrefix(args[0], "-") {
		return unknownFlag(args[0])
	}
	if args[0] != "okf" {
		return errors.New("migrate requires subcommand: okf")
	}
	if helpRequested(args[1:]) {
		app.printMigrateOKFHelp()
		return nil
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
	summary, err := claimStore.MigrateOKF(workspace)
	if err != nil {
		return err
	}
	indexFresh := true
	indexFreshError := ""
	if err := (zruntime.IndexStore{Paths: app.Paths}).CheckFresh(workspace); err != nil {
		indexFresh = false
		indexFreshError = err.Error()
	}
	return writeJSON(app.Stdout, struct {
		SchemaVersion   int    `json:"schema_version"`
		IndexFresh      bool   `json:"index_fresh"`
		IndexFreshError string `json:"index_fresh_error,omitempty"`
		zruntime.ClaimMigrationSummary
	}{SchemaVersion: 1, IndexFresh: indexFresh, IndexFreshError: indexFreshError, ClaimMigrationSummary: summary})
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
	if helpRequested(args) {
		app.printWorkspaceHelp()
		return nil
	}
	if strings.HasPrefix(args[0], "-") {
		return unknownFlag(args[0])
	}
	switch args[0] {
	case "create":
		if helpRequested(args[1:]) {
			app.printWorkspaceCreateHelp()
			return nil
		}
		_, rest, err := parseFlags(args[1:], noFlags)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return errors.New("usage: zbrain workspace create <name>")
		}
		if err := zruntime.CreateWorkspace(app.Paths, rest[0], app.Now()); err != nil {
			return err
		}
		_, err = fmt.Fprintf(app.Stdout, "workspace created: %s\n", rest[0])
		return err
	case "current":
		if helpRequested(args[1:]) {
			app.printWorkspaceCurrentHelp()
			return nil
		}
		_, rest, err := parseFlags(args[1:], noFlags)
		if err != nil {
			return err
		}
		if len(rest) > 0 {
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

func parseFlags(args []string, allowed map[string]struct{}) (parsedFlags, []string, error) {
	flags := parsedFlags{values: map[string][]string{}}
	rest := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(arg, "--")
			if _, ok := allowed[name]; !ok {
				return flags, rest, unknownFlag(arg)
			}
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return flags, rest, fmt.Errorf("flag %s requires a value", arg)
			}
			flags.values[name] = append(flags.values[name], args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return flags, rest, unknownFlag(arg)
		}
		rest = append(rest, arg)
	}
	return flags, rest, nil
}

func parseWorkspaceFlag(args []string) (string, []string, error) {
	flags, rest, err := parseFlags(args, workspaceFlags)
	return flags.single("workspace"), rest, err
}

func parseWorkspaceIncludeFlags(args []string) (string, []string, []string, error) {
	flags, rest, err := parseFlags(args, askFlags)
	return flags.single("workspace"), flags.values["include"], rest, err
}

func unknownFlag(arg string) error {
	return fmt.Errorf("unknown flag: %s", arg)
}

func helpRequested(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help")
}

func (app App) printHelp() {
	fmt.Fprint(app.Stdout, `zbrain - Go-native OKF trusted memory CLI

Usage:
  zbrain <command> [arguments]
  zbrain <command> --help

Commands:
  setup
  workspace create <name>
  workspace current
  evidence add --file <path> --origin <uri-or-path> [--media-type <type>] [--workspace <name>]
  claim draft --tier <tier> --title <title> --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]
  claim approve <id> [--workspace <name>]
  claim supersede <id> --tier <tier> --title <title> --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]
  claim revoke <id> --reason <text> [--workspace <name>]
  migrate okf [--workspace <name>]
  reindex [--workspace <name>] [--embed]
  ask [--workspace <name>] [--include <name>]... <query>
  status [--workspace <name>]
  doctor [--workspace <name>] [--probe-embedder]
  mcp serve
  view
  version

Use `+"`zbrain <command> --help`"+` for command-specific help.
`)
}

func (app App) printSetupHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain setup

Prepare the runtime directory and extract embedded assets.
`)
}

func (app App) printVersionHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain version

Print the CLI version.
`)
}

func (app App) printWorkspaceHelp() {
	fmt.Fprint(app.Stdout, `Usage:
  zbrain workspace create <name>
  zbrain workspace current

Manage the active workspace.
`)
}

func (app App) printWorkspaceCreateHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain workspace create <name>

Create a workspace using a lowercase name, digits, or hyphens.
`)
}

func (app App) printWorkspaceCurrentHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain workspace current

Print the active workspace as JSON.
`)
}

func (app App) printEvidenceHelp() {
	app.printEvidenceAddHelp()
}

func (app App) printEvidenceAddHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain evidence add --file <path> --origin <uri-or-path> [--media-type <type>] [--workspace <name>]

Options:
  --file <path>             Local source file to snapshot
  --origin <uri-or-path>    Origin recorded in evidence metadata
  --media-type <type>      Optional media type
  --workspace <name>       Target workspace; defaults to the current workspace
`)
}

func (app App) printClaimHelp() {
	fmt.Fprint(app.Stdout, `Usage:
  zbrain claim draft --tier <tier> --title <title> --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]
  zbrain claim approve <id> [--workspace <name>]
  zbrain claim supersede <id> --tier <tier> --title <title> --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]
  zbrain claim revoke <id> --reason <text> [--workspace <name>]

Manage the four-state OKF claim lifecycle.
`)
}

func (app App) printClaimDraftHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain claim draft --tier <tier> --title <title> --basis <owner|evidence|derived> [options]

Options:
  --tier <tier>             Claim tier
  --title <title>           Claim title
  --basis <basis>           owner, evidence, or derived
  --evidence <id>           Evidence ID; repeat for multiple IDs
  --support <id>            Supporting claim ID; repeat for multiple IDs
  --conflicts-with <id>     Conflicting claim ID; repeat for multiple IDs
  --workspace <name>       Target workspace; defaults to the current workspace

The claim body is read from stdin.
`)
}

func (app App) printClaimApproveHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain claim approve <id> [--workspace <name>]

Promote a valid draft claim.
`)
}

func (app App) printClaimSupersedeHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain claim supersede <id> --tier <tier> --title <title> --basis <owner|evidence|derived> [options]

Options:
  --tier <tier>             Replacement claim tier
  --title <title>           Replacement claim title
  --basis <basis>           owner, evidence, or derived
  --evidence <id>           Evidence ID; repeat for multiple IDs
  --support <id>            Supporting claim ID; repeat for multiple IDs
  --conflicts-with <id>     Conflicting claim ID; repeat for multiple IDs
  --workspace <name>       Target workspace; defaults to the current workspace

The replacement claim body is read from stdin.
`)
}

func (app App) printClaimRevokeHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain claim revoke <id> --reason <text> [--workspace <name>]

Revoke a claim with an operator-provided reason.
`)
}

func (app App) printMigrateHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain migrate okf [--workspace <name>]

Convert legacy zbrain claim files to OKF concepts.
`)
}

func (app App) printMigrateOKFHelp() {
	app.printMigrateHelp()
}

func (app App) printReindexHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain reindex [--workspace <name>] [--embed]

Rebuild the disposable workspace index.

Options:
  --embed        Also compute and store loopback embedding vectors
`)
}

func (app App) printViewHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain view

Serve the embedded read-only viewer over loopback (127.0.0.1).

The viewer binds loopback only, sends strict CSP and nosniff headers,
has no CORS, and returns 405 for every mutation method.
`)
}

func (app App) printAskHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain ask [--workspace <name>] [--include <name>]... <query>

Return trusted context JSON without calling an LLM.

Options:
  --workspace <name>       Primary workspace; defaults to the current workspace
  --include <name>         Explicit read-only secondary workspace; repeatable
`)
}

func (app App) printMCPHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain mcp serve

Serve the trusted-agent MCP gateway over stdio.
`)
}

func (app App) printMCPServeHelp() {
	fmt.Fprint(app.Stdout, `Usage: zbrain mcp serve

Run the MCP stdio gateway. stdout is protocol-only; diagnostics go to stderr.
`)
}
