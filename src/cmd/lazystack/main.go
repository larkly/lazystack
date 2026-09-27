package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/app"
	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/config"
	"github.com/larkly/lazystack/internal/selfupdate"
	"github.com/larkly/lazystack/internal/shared"
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
		signed, err := selfupdate.Apply(ctx, version, latest, downloadURL, checksumsURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			os.Exit(1)
		}
		if !signed {
			fmt.Fprintf(os.Stderr, "Warning: %s was verified against its SHA256SUMS only, without a release signature. Signatures are required from %s.\n", latest, selfupdate.SignatureRequiredFrom)
		}
		fmt.Printf("Successfully updated to %s.\n", latest)
		return
	}

	cfg, cfgErr := config.Load()
	reportConfigLoad(os.Stderr, cfg, cfgErr)

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
		// Only returns if the restart failed.
		os.Exit(restartOrReport(os.Stderr))
	}
}

// executable is a variable so tests can point the restart at a binary that
// cannot be executed.
var executable = os.Executable

// restartSelf replaces the current process with a fresh copy of the running
// binary. On success it never returns.
func restartSelf() error {
	exe, err := executable()
	if err != nil {
		return fmt.Errorf("locating executable: %w", err)
	}
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		return fmt.Errorf("exec %s: %w", exe, err)
	}
	return nil
}

// restartOrReport restarts lazystack; if that fails it reports why on w and
// returns a nonzero exit status instead of letting main exit successfully.
func restartOrReport(w io.Writer) int {
	if err := restartSelf(); err != nil {
		fmt.Fprintf(w, "restart failed: %v\n", err)
		return 1
	}
	return 0
}

// reportConfigLoad prints config load problems (a failed load, or values
// that were rejected and replaced by defaults) as warnings.
func reportConfigLoad(w io.Writer, cfg config.Config, err error) {
	if err != nil {
		fmt.Fprintf(w, "Warning: failed to load config: %v\n", err)
	}
	for _, warning := range cfg.Warnings {
		fmt.Fprintf(w, "Warning: config: %s\n", warning)
	}
}
