package config

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/larkly/lazystack/internal/shared"
)

// colorStr returns a string representation of a color for comparison.
func colorStr(c fmt.Stringer) string { return c.String() }

func TestApplyGeneral_SetsPlainMode(t *testing.T) {
	prev := shared.PlainMode
	defer func() { shared.PlainMode = prev }()

	ApplyGeneral(GeneralConfig{PlainMode: true})
	if !shared.PlainMode {
		t.Error("expected PlainMode=true after ApplyGeneral")
	}

	ApplyGeneral(GeneralConfig{PlainMode: false})
	if shared.PlainMode {
		t.Error("expected PlainMode=false after ApplyGeneral")
	}
}

func TestApplyColors_SetsPrimary(t *testing.T) {
	prev := shared.ColorPrimary
	defer func() { shared.ColorPrimary = prev }()

	ApplyColors(ColorConfig{Primary: "#FF0000"})
	want := lipgloss.Color("#FF0000")
	if fmt.Sprint(shared.ColorPrimary) != fmt.Sprint(want) {
		t.Errorf("expected ColorPrimary=#FF0000, got %s", shared.ColorPrimary)
	}
}

func TestApplyColors_SetsError(t *testing.T) {
	prev := shared.ColorError
	defer func() { shared.ColorError = prev }()

	ApplyColors(ColorConfig{Error: "#FF00FF"})
	want := lipgloss.Color("#FF00FF")
	if fmt.Sprint(shared.ColorError) != fmt.Sprint(want) {
		t.Errorf("expected ColorError=#FF00FF, got %s", shared.ColorError)
	}
}

func TestApplyColors_SetsMuted(t *testing.T) {
	prev := shared.ColorMuted
	defer func() { shared.ColorMuted = prev }()

	ApplyColors(ColorConfig{Muted: "#AAAAAA"})
	want := lipgloss.Color("#AAAAAA")
	if fmt.Sprint(shared.ColorMuted) != fmt.Sprint(want) {
		t.Errorf("expected ColorMuted=#AAAAAA, got %s", shared.ColorMuted)
	}
}

func TestApplyColors_EmptyFieldDoesNotChange(t *testing.T) {
	prev := shared.ColorPrimary
	defer func() { shared.ColorPrimary = prev }()

	ApplyColors(ColorConfig{Primary: "#ABCDEF"})
	if fmt.Sprint(shared.ColorPrimary) != fmt.Sprint(lipgloss.Color("#ABCDEF")) {
		t.Fatalf("setup: ColorPrimary not set correctly")
	}

	// Apply again with empty Primary — should not overwrite
	ApplyColors(ColorConfig{Muted: "#000000"})
	if fmt.Sprint(shared.ColorPrimary) != fmt.Sprint(lipgloss.Color("#ABCDEF")) {
		t.Errorf("empty Primary field should not change existing value")
	}
}

func TestApplyColors_MultipleColors(t *testing.T) {
	prevPri := shared.ColorPrimary
	prevErr := shared.ColorError
	prevCya := shared.ColorCyan
	defer func() {
		shared.ColorPrimary = prevPri
		shared.ColorError = prevErr
		shared.ColorCyan = prevCya
	}()

	ApplyColors(ColorConfig{
		Primary:   "#111111",
		Secondary: "#222222",
		Success:   "#333333",
		Warning:   "#444444",
		Error:     "#555555",
		Muted:     "#666666",
		Bg:        "#777777",
		Fg:        "#888888",
		Highlight: "#999999",
		Cyan:      "#AAAAAA",
	})

	if fmt.Sprint(shared.ColorPrimary) != fmt.Sprint(lipgloss.Color("#111111")) {
		t.Errorf("ColorPrimary: got %s, want #111111", shared.ColorPrimary)
	}
	if fmt.Sprint(shared.ColorError) != fmt.Sprint(lipgloss.Color("#555555")) {
		t.Errorf("ColorError: got %s, want #555555", shared.ColorError)
	}
	if fmt.Sprint(shared.ColorCyan) != fmt.Sprint(lipgloss.Color("#AAAAAA")) {
		t.Errorf("ColorCyan: got %s, want #AAAAAA", shared.ColorCyan)
	}
}

func TestApplyKeybindings_SetsQuitAndHelp(t *testing.T) {
	prevQuit := shared.Keys.Quit
	prevHelp := shared.Keys.Help
	defer func() {
		shared.Keys.Quit = prevQuit
		shared.Keys.Help = prevHelp
	}()

	ApplyKeybindings(map[string]string{
		"quit": "ctrl+x",
		"help": "F1",
	})

	quitHelp := shared.Keys.Quit.Help()
	if !strings.Contains(quitHelp.Key, "ctrl+x") {
		t.Errorf("expected Quit key to contain ctrl+x, got %s", quitHelp.Key)
	}
	helpHelp := shared.Keys.Help.Help()
	if !strings.Contains(helpHelp.Key, "F1") || helpHelp.Desc != "help" {
		t.Errorf("expected Help key=F1 desc=help, got key=%s desc=%s", helpHelp.Key, helpHelp.Desc)
	}
}

func TestApplyKeybindings_HelpTextPreserved(t *testing.T) {
	prev := shared.Keys.Quit
	defer func() { shared.Keys.Quit = prev }()

	ApplyKeybindings(map[string]string{"quit": "ctrl+y"})

	help := shared.Keys.Quit.Help()
	if help.Desc != "quit" {
		t.Errorf("expected Quit.Help().Desc=quit, got %s", help.Desc)
	}
	if !strings.Contains(help.Key, "ctrl+y") {
		t.Errorf("expected Quit key to contain ctrl+y, got %s", help.Key)
	}
}

func TestApplyKeybindings_NilMap(t *testing.T) {
	// Should not panic
	ApplyKeybindings(nil)
}

func TestApplyKeybindings_UnknownKeyIgnored(t *testing.T) {
	prev := shared.Keys.Quit
	defer func() { shared.Keys.Quit = prev }()

	ApplyKeybindings(map[string]string{"quit": "q"})
	quitBefore := shared.Keys.Quit.Help().Key

	ApplyKeybindings(map[string]string{
		"quit":            "q",
		"nonexistent_key": "x",
	})

	if shared.Keys.Quit.Help().Key != quitBefore {
		t.Errorf("quit key changed after unknown key: %s -> %s", quitBefore, shared.Keys.Quit.Help().Key)
	}
}

func TestApplyAll_Integration(t *testing.T) {
	prevPlain := shared.PlainMode
	prevPri := shared.ColorPrimary
	prevQuit := shared.Keys.Quit
	defer func() {
		shared.PlainMode = prevPlain
		shared.ColorPrimary = prevPri
		shared.Keys.Quit = prevQuit
	}()

	cfg := Defaults()
	cfg.General.PlainMode = true
	cfg.Colors.Primary = "#FF0000"
	cfg.Colors.Error = "#0000FF"
	cfg.Keybindings = map[string]string{
		"quit": "ctrl+x",
		"help": "h",
	}

	ApplyAll(cfg)

	if !shared.PlainMode {
		t.Error("expected PlainMode=true after ApplyAll")
	}
	if fmt.Sprint(shared.ColorPrimary) != fmt.Sprint(lipgloss.Color("#FF0000")) {
		t.Errorf("expected ColorPrimary=#FF0000")
	}
	if shared.Keys.Quit.Help().Key != "ctrl+x" {
		t.Errorf("expected Quit key=ctrl+x, got %s", shared.Keys.Quit.Help().Key)
	}
}
