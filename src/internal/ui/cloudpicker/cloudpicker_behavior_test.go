package cloudpicker

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"strings"
	"testing"
)

func TestRenderStatesAndQuitMessage(t *testing.T) {
	for _, tc := range []struct {
		clouds []string
		err    error
		want   string
	}{{[]string{"production"}, nil, "production"}, {nil, nil, "Select Cloud"}, {nil, errors.New("missing config"), "missing config"}} {
		m := New(tc.clouds, tc.err)
		m.SetSize(100, 30)
		if m.Init() != nil || !strings.Contains(m.View(), tc.want) || !strings.Contains(m.Hints(), "navigate") {
			t.Fatal(m.View())
		}
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
		if cmd == nil {
			t.Fatal("no quit")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("not quit message")
		}
	}
}
