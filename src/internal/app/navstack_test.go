package app

import (
	"testing"
)

func TestNavStackPushPop(t *testing.T) {
	var ns NavStack
	if !ns.IsEmpty() {
		t.Fatal("new stack should be empty")
	}
	if ns.Len() != 0 {
		t.Fatal("new stack len should be 0")
	}

	ns.Push(viewServerList, 0)
	if ns.IsEmpty() {
		t.Fatal("stack should not be empty after push")
	}
	if ns.Len() != 1 {
		t.Fatalf("len = %d, want 1", ns.Len())
	}
	if ns.TopView() != viewServerList {
		t.Fatalf("top view = %d, want viewServerList", ns.TopView())
	}

	entry, ok := ns.Peek()
	if !ok {
		t.Fatal("peek should succeed")
	}
	if entry.View != viewServerList {
		t.Fatalf("peeked view = %d, want viewServerList", entry.View)
	}
	if entry.Tab != 0 {
		t.Fatalf("peeked tab = %d, want 0", entry.Tab)
	}
	if ns.Len() != 1 {
		t.Fatal("peek should not remove entry")
	}

	entry, ok = ns.Pop()
	if !ok {
		t.Fatal("pop should succeed")
	}
	if entry.View != viewServerList || entry.Tab != 0 {
		t.Fatal("popped entry mismatch")
	}
	if !ns.IsEmpty() {
		t.Fatal("stack should be empty after pop")
	}
}

func TestNavStackPopEmpty(t *testing.T) {
	var ns NavStack
	_, ok := ns.Pop()
	if ok {
		t.Fatal("pop on empty stack should fail")
	}
}

func TestNavStackPeekEmpty(t *testing.T) {
	var ns NavStack
	_, ok := ns.Peek()
	if ok {
		t.Fatal("peek on empty stack should fail")
	}
	if ns.TopView() != 0 {
		t.Fatal("topView on empty stack should be 0")
	}
}

func TestNavStackMultipleItems(t *testing.T) {
	var ns NavStack
	ns.Push(viewCloudPicker, -1)
	ns.Push(viewServerList, 0)
	ns.Push(viewServerDetail, 2)

	if ns.Len() != 3 {
		t.Fatalf("len = %d, want 3", ns.Len())
	}

	// LIFO order
	e1, _ := ns.Pop()
	if e1.View != viewServerDetail || e1.Tab != 2 {
		t.Fatalf("pop 1 = %d/%d, want viewServerDetail/2", e1.View, e1.Tab)
	}
	e2, _ := ns.Pop()
	if e2.View != viewServerList || e2.Tab != 0 {
		t.Fatalf("pop 2 = %d/%d, want viewServerList/0", e2.View, e2.Tab)
	}
	e3, _ := ns.Pop()
	if e3.View != viewCloudPicker || e3.Tab != -1 {
		t.Fatalf("pop 3 = %d/%d, want viewCloudPicker/-1", e3.View, e3.Tab)
	}
}

func TestNavStackClear(t *testing.T) {
	var ns NavStack
	ns.Push(viewServerList, 0)
	ns.Push(viewServerDetail, 0)
	ns.Clear()
	if !ns.IsEmpty() {
		t.Fatal("stack should be empty after clear")
	}
	if ns.Len() != 0 {
		t.Fatal("len should be 0 after clear")
	}
}

func TestNavStackThreadSafety(t *testing.T) {
	var ns NavStack
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				ns.Push(viewServerList, 0)
				ns.Pop()
			}
			done <- true
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}
