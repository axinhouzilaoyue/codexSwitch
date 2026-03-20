package main

import (
	"flag"
	"fmt"
	"os"

	"codexswitch/internal/app"
	"codexswitch/internal/buildinfo"
	"codexswitch/internal/cli"
	"codexswitch/internal/selfmanage"
	"codexswitch/internal/store"
)

func main() {
	command := "tui"
	args := os.Args[1:]
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		command = args[0]
		args = args[1:]
	}

	flags := flag.NewFlagSet("ccodex", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "CodexSwitch %s\n\n", buildinfo.Version)
		fmt.Fprintln(flags.Output(), "Usage:")
		fmt.Fprintln(flags.Output(), "  ccodex [tui] [--store-dir DIR] [--codex-home DIR]")
		fmt.Fprintln(flags.Output(), "  ccodex list [--store-dir DIR] [--codex-home DIR]")
		fmt.Fprintln(flags.Output(), "  ccodex current [--store-dir DIR] [--codex-home DIR]")
		fmt.Fprintln(flags.Output(), "  ccodex doctor [--store-dir DIR] [--codex-home DIR]")
		fmt.Fprintln(flags.Output(), "  ccodex update")
		fmt.Fprintln(flags.Output(), "  ccodex uninstall")
		fmt.Fprintln(flags.Output(), "  ccodex version")
		fmt.Fprintln(flags.Output(), "")
		fmt.Fprintln(flags.Output(), "Commands:")
		fmt.Fprintln(flags.Output(), "  tui        Launch the interactive TUI (default)")
		fmt.Fprintln(flags.Output(), "  list       Print saved profiles")
		fmt.Fprintln(flags.Output(), "  current    Print the active account from target CODEX_HOME")
		fmt.Fprintln(flags.Output(), "  doctor     Print environment diagnostics")
		fmt.Fprintln(flags.Output(), "  update     Download and replace ccodex with the latest release")
		fmt.Fprintln(flags.Output(), "  uninstall  Remove the installed ccodex binary")
		fmt.Fprintln(flags.Output(), "  version    Print version and exit")
		fmt.Fprintln(flags.Output(), "")
		flags.PrintDefaults()
	}

	var storeDir string
	var codexHome string
	var showVersion bool
	flags.StringVar(&storeDir, "store-dir", "", "Directory used to persist saved auth profiles. Defaults to ~/.codex-switch")
	flags.StringVar(&codexHome, "codex-home", "", "Override the active CODEX_HOME for this command without changing saved settings")
	flags.BoolVar(&showVersion, "version", false, "Print version and exit")
	_ = flags.Parse(args)

	if showVersion || command == "version" {
		fmt.Println(buildinfo.Version)
		return
	}

	switch command {
	case "update":
		if err := selfmanage.RunUpdate(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	case "uninstall":
		if err := selfmanage.RunUninstall(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	profileStore, err := store.New(storeDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	switch command {
	case "tui":
		application, err := app.New(profileStore, codexHome)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := application.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "list":
		if err := cli.RunList(profileStore, codexHome); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "current":
		if err := cli.RunCurrent(profileStore, codexHome); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "doctor":
		if err := cli.RunDoctor(profileStore, codexHome); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		flags.Usage()
		os.Exit(2)
	}
}
