package shared

import "fmt"

// DeduplicateName generates a unique "-clone" suffixed name.
// existingNames is the set of names already in use.
// Returns "base-clone", "base-clone-2", "base-clone-3", etc.
func DeduplicateName(base string, existingNames map[string]bool) string {
	candidate := base + "-clone"
	if !existingNames[candidate] {
		return candidate
	}
	for i := 2; ; i++ {
		candidate = fmt.Sprintf("%s-clone-%d", base, i)
		if !existingNames[candidate] {
			return candidate
		}
	}
}
