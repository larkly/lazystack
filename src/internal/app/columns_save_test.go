package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/larkly/lazystack/internal/config"
	"github.com/larkly/lazystack/internal/ui/columnpicker"
	"github.com/larkly/lazystack/internal/ui/serverlist"
)

var chosenColumns = []config.ColumnConfig{
	{Key: "status"}, {Key: "name"}, {Key: "ipv4", Hidden: true},
}

func TestColumnLayoutSaveFailureIsReported(t *testing.T) {
	home := t.TempDir()
	// A regular file where the config directory should be makes the save fail.
	if err := os.WriteFile(filepath.Join(home, ".config"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	m := initTestModel()
	m.serverList = serverlist.New(nil, nil, time.Hour)
	next, _ := m.Update(columnpicker.ColumnsChosenMsg{Columns: chosenColumns})
	m = next.(Model)
	if !strings.Contains(m.statusBar.StickyHint, "not saved") {
		t.Fatalf("save failure not surfaced, hint = %q", m.statusBar.StickyHint)
	}
	if !reflect.DeepEqual(m.configView.Cfg().Columns, chosenColumns) {
		t.Fatal("in-memory layout should still be applied after a failed save")
	}
}

func TestColumnLayoutSavePersists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	m := initTestModel()
	m.serverList = serverlist.New(nil, nil, time.Hour)
	next, _ := m.Update(columnpicker.ColumnsChosenMsg{Columns: chosenColumns})
	m = next.(Model)
	if strings.Contains(m.statusBar.StickyHint, "not saved") {
		t.Fatalf("unexpected failure hint %q", m.statusBar.StickyHint)
	}
	loaded, err := config.LoadFrom(filepath.Join(home, ".config", "lazystack", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Columns, chosenColumns) {
		t.Fatalf("reloaded columns = %+v, want %+v", loaded.Columns, chosenColumns)
	}
}
