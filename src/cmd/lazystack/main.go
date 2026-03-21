package main

import (
	"flag"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/larkly/lazystack/internal/app"
	"github.com/larkly/lazystack/internal/selfupdate"
	"charm.land/bubbletea/v2"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	checkUpdate := flag.Bool("check-update", false, "check if a newer version is available")
	doUpdate := flag.Bool("update", false, "update to the latest version")
	alwaysPick := flag.Bool("pick-cloud", false, "always show cloud picker, even if only one cloud is configured")
	refreshSec := flag.Int("refresh", 5, "server list auto-refresh interval in seconds")
	flag.Parse()

	if *showVersion {
		fmt.Println("lazystack " + version)
		return
	}

	if *checkUpdate {
		latest, _, _, err := selfupdate.CheckLatest(version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if latest == "" {
			fmt.Printf("lazystack %s is already up to date.\n", version)
		} else {
			fmt.Printf("A new version is available: %s (current: %s)\nRun with --update to install it.\n", latest, version)
		}
		return
	}

	if *doUpdate {
		latest, downloadURL, checksumsURL, err := selfupdate.CheckLatest(version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if latest == "" {
			fmt.Printf("lazystack %s is already up to date.\n", version)
			return
		}
		fmt.Printf("Updating lazystack %s → %s...\n", version, latest)
		if err := selfupdate.Apply(downloadURL, checksumsURL); err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully updated to %s.\n", latest)
		return
	}

	m := app.New(app.Options{
		AlwaysPickCloud: *alwaysPick,
		RefreshInterval: time.Duration(*refreshSec) * time.Second,
		Version:         version,
	})
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if fm, ok := finalModel.(app.Model); ok && fm.ShouldRestart() {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "restart failed: %v\n", err)
			os.Exit(1)
		}
		syscall.Exec(exe, os.Args, os.Environ())
	}
}
