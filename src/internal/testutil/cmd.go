package testutil

import (
	"reflect"

	tea "charm.land/bubbletea/v2"
)

var cmdType = reflect.TypeOf((tea.Cmd)(nil))

// CmdMessages runs cmd and returns the messages it produces in order,
// unpacking batches and sequences. Commands that block (timers) block it
// too, so use it only for commands that return immediately.
func CmdMessages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	v := reflect.ValueOf(msg)
	if v.Kind() == reflect.Slice && v.Type().Elem() == cmdType {
		var out []tea.Msg
		for i := 0; i < v.Len(); i++ {
			out = append(out, CmdMessages(v.Index(i).Interface().(tea.Cmd))...)
		}
		return out
	}
	return []tea.Msg{msg}
}
