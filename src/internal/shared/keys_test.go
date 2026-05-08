package shared

import (
	"testing"

	"charm.land/bubbles/v2/key"
)

func TestKeys_IsNonNil(t *testing.T) {
	if Keys.Quit.Keys() == nil {
		t.Fatal("Keys.Quit is nil")
	}
}

func TestKeys_EssentialBindingsExist(t *testing.T) {
	tests := []struct {
		name    string
		binding key.Binding
	}{
		{"Quit", Keys.Quit},
		{"Help", Keys.Help},
		{"Enter", Keys.Enter},
		{"Back", Keys.Back},
		{"Up", Keys.Up},
		{"Down", Keys.Down},
		{"Refresh", Keys.Refresh},
		{"Delete", Keys.Delete},
		{"Create", Keys.Create},
		{"Select", Keys.Select},
		{"Confirm", Keys.Confirm},
		{"Deny", Keys.Deny},
		{"Filter", Keys.Filter},
		{"Console", Keys.Console},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.binding.Keys() == nil || len(tt.binding.Keys()) == 0 {
				t.Errorf("%s binding has no keys", tt.name)
			}
			if tt.binding.Help().Key == "" {
				t.Errorf("%s binding has empty help key", tt.name)
			}
		})
	}
}

func TestKeys_QuitHasCtrlC(t *testing.T) {
	keys := Keys.Quit.Keys()
	found := false
	for _, k := range keys {
		if k == "ctrl+c" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Quit binding missing ctrl+c")
	}
}

func TestKeys_NavigationBindingsAreVimCompatible(t *testing.T) {
	upKeys := Keys.Up.Keys()
	hasK := false
	for _, k := range upKeys {
		if k == "k" {
			hasK = true
			break
		}
	}
	if !hasK {
		t.Error("Up binding missing vim-compatible 'k'")
	}

	downKeys := Keys.Down.Keys()
	hasJ := false
	for _, k := range downKeys {
		if k == "j" {
			hasJ = true
			break
		}
	}
	if !hasJ {
		t.Error("Down binding missing vim-compatible 'j'")
	}
}

func TestKeys_DestructiveActionsHaveConfirm(t *testing.T) {
	if len(Keys.Confirm.Keys()) == 0 {
		t.Error("Confirm binding must have keys")
	}
	if len(Keys.Deny.Keys()) == 0 {
		t.Error("Deny (cancel) binding must have keys")
	}
}
