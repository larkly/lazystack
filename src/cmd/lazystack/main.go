package main

import (
	"flag"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/bosse/lazystack/internal/app"
	"charm.land/bubbletea/v2"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	alwaysPick := flag.Bool("pick-cloud", false, "always show cloud picker, even if only one cloud is configured")
	refreshSec := flag.Int("refresh", 5, "server list auto-refresh interval in seconds")
	flag.Parse()

	if *showVersion {
		fmt.Println("lazystack " + version)
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
