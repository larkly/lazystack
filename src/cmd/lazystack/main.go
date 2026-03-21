package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/bosse/lazystack/internal/app"
	"charm.land/bubbletea/v2"
)

func main() {
	alwaysPick := flag.Bool("pick-cloud", false, "always show cloud picker, even if only one cloud is configured")
	refresh := flag.Duration("refresh", 5*time.Second, "server list auto-refresh interval")
	flag.Parse()

	m := app.New(app.Options{
		AlwaysPickCloud: *alwaysPick,
		RefreshInterval: *refresh,
	})
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
