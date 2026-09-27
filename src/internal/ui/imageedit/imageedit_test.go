package imageedit

import "testing"

// #325: edits share the create form's min disk/RAM validation.
func TestSubmitRejectsInvalidMinimums(t *testing.T) {
	for _, tc := range []struct{ disk, ram string }{
		{"-1", "0"},
		{"abc", "0"},
		{"0", "-512"},
		{"0", "1.5"},
	} {
		m := New(nil, "img-1", "image", "private", 1, 2, nil, false)
		m.minDiskInput.SetValue(tc.disk)
		m.minRAMInput.SetValue(tc.ram)

		m, cmd := m.submit()
		if cmd != nil || m.err == "" || m.submitting {
			t.Fatalf("disk=%q ram=%q: cmd=%v err=%q, want inline error", tc.disk, tc.ram, cmd != nil, m.err)
		}
	}
}

func TestSubmitAcceptsValidAndBlankMinimums(t *testing.T) {
	for _, tc := range []struct{ disk, ram string }{
		{"", ""},
		{"20", "2048"},
		{"0", "0"},
	} {
		m := New(nil, "img-1", "image", "private", 1, 2, nil, false)
		m.minDiskInput.SetValue(tc.disk)
		m.minRAMInput.SetValue(tc.ram)

		m, cmd := m.submit()
		if cmd == nil || m.err != "" {
			t.Fatalf("disk=%q ram=%q: err=%q, want submit", tc.disk, tc.ram, m.err)
		}
	}
}
