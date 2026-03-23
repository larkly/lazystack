package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/larkly/lazystack/internal/app"
	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/selfupdate"
	"charm.land/bubbletea/v2"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	noCheckUpdate := flag.Bool("no-check-update", false, "skip automatic update check on startup")
	doUpdate := flag.Bool("update", false, "update to the latest version")
	alwaysPick := flag.Bool("pick-cloud", false, "always show cloud picker, even if only one cloud is configured")
	cloudFlag := flag.String("cloud", "", "connect directly to named cloud, skip picker")
	refreshSec := flag.Int("refresh", 5, "server list auto-refresh interval in seconds")
	flag.Parse()

	if *showVersion {
		fmt.Println("lazystack " + version)
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

	if *cloudFlag != "" {
		clouds, err := cloud.ListCloudNames()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading clouds.yaml: %v\n", err)
			os.Exit(1)
		}
		if !slices.Contains(clouds, *cloudFlag) {
			fmt.Fprintf(os.Stderr, "Cloud %q not found. Available clouds: %s\n", *cloudFlag, strings.Join(clouds, ", "))
			os.Exit(1)
		}
	}

	m := app.New(app.Options{
		AlwaysPickCloud: *alwaysPick,
		Cloud:           *cloudFlag,
		RefreshInterval: time.Duration(*refreshSec) * time.Second,
		Version:         version,
		CheckUpdate:     !*noCheckUpdate,
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
