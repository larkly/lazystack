package app

import (
	"sync"
)

// NavStack is an explicit navigation stack for back-navigation.
// Replaces the implicit previousView/returnToView fields on the Model.
type NavStack struct {
	mu    sync.Mutex
	items []NavEntry
}

// NavEntry represents one navigation point on the stack.
type NavEntry struct {
	View activeView // view to restore
	Tab  int        // active tab to restore, -1 if unchanged
}

// Push saves the current state. Use before navigating away.
func (ns *NavStack) Push(view activeView, tab int) {
	ns.mu.Lock()
	ns.items = append(ns.items, NavEntry{View: view, Tab: tab})
	ns.mu.Unlock()
}

// Pop removes and returns the top entry. Returns (zero, false) on empty stack.
func (ns *NavStack) Pop() (NavEntry, bool) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	if len(ns.items) == 0 {
		return NavEntry{}, false
	}
	idx := len(ns.items) - 1
	entry := ns.items[idx]
	ns.items = ns.items[:idx]
	return entry, true
}

// Peek returns the top entry without removing it.
func (ns *NavStack) Peek() (NavEntry, bool) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	if len(ns.items) == 0 {
		return NavEntry{}, false
	}
	return ns.items[len(ns.items)-1], true
}

// TopView returns the view of the top entry, or 0 if empty.
func (ns *NavStack) TopView() activeView {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	if len(ns.items) == 0 {
		return 0
	}
	return ns.items[len(ns.items)-1].View
}

// IsEmpty reports whether the stack has no entries.
func (ns *NavStack) IsEmpty() bool {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	return len(ns.items) == 0
}

// Len returns the number of entries in the stack.
func (ns *NavStack) Len() int {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	return len(ns.items)
}

// Clear empties the stack.
func (ns *NavStack) Clear() {
	ns.mu.Lock()
	ns.items = nil
	ns.mu.Unlock()
}
