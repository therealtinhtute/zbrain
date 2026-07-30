package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

const Version = "0.1.0-go"

type App struct {
	Stdout io.Writer
	Stderr io.Writer
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

func (app App) printHelp() {
	fmt.Fprint(app.Stdout, `zbrain - Go-native personal memory CLI

Usage:
  zbrain <command> [arguments]

Commands:
  setup                      Prepare the runtime directory
  workspace create <name>    Create a workspace
  workspace current          Print the active workspace as JSON
  version                    Print version
`)
}
