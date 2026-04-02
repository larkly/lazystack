package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/larkly/lazystack/internal/app"
	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/config"
	"github.com/larkly/lazystack/internal/selfupdate"
	"github.com/larkly/lazystack/internal/shared"
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
	idleTimeoutMin := flag.Int("idle-timeout", 0, "pause polling after N minutes of no input (0 = disabled)")
	plainMode := flag.Bool("plain", false, "disable Unicode status icons")
	debugMode := flag.Bool("debug", false, "write debug log to ~/.cache/lazystack/debug.log")
	flag.Parse()

	if *debugMode {
		if err := shared.EnableDebug(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not enable debug logging: %v\n", err)
		}
	}

	if *showVersion {
		fmt.Println("lazystack " + version)
		return
	}

	if *doUpdate {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		latest, downloadURL, checksumsURL, err := selfupdate.CheckLatest(ctx, version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if latest == "" {
			fmt.Printf("lazystack %s is already up to date.\n", version)
			return
		}
		fmt.Printf("Updating lazystack %s → %s...\n", version, latest)
		if err := selfupdate.Apply(ctx, downloadURL, checksumsURL); err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully updated to %s.\n", latest)
		return
	}

	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", cfgErr)
	}

	// Detect which CLI flags were explicitly set
	var cliFlags config.CLIFlags
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "refresh":
			d := time.Duration(*refreshSec) * time.Second
			cliFlags.RefreshInterval = &d
		case "idle-timeout":
			d := time.Duration(*idleTimeoutMin) * time.Minute
			cliFlags.IdleTimeout = &d
		case "plain":
			cliFlags.PlainMode = plainMode
		case "no-check-update":
			v := !*noCheckUpdate
			cliFlags.CheckForUpdates = &v
		case "pick-cloud":
			cliFlags.AlwaysPickCloud = alwaysPick
		}
	})
	cliFlags.Cloud = *cloudFlag

	cfg = config.Merge(cfg, cliFlags)
	config.ApplyAll(cfg)

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
		AlwaysPickCloud: cfg.General.AlwaysPickCloud,
		Cloud:           cliFlags.Cloud,
		RefreshInterval: time.Duration(cfg.General.RefreshInterval) * time.Second,
		IdleTimeout:     time.Duration(cfg.General.IdleTimeout) * time.Minute,
		Version:         version,
		CheckUpdate:     cfg.General.CheckForUpdates,
		Plain:           cfg.General.PlainMode,
		Config:          &cfg,
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
