package shared

import "strings"

// PadContent lays out a bordered panel's inside: the title, a blank line,
// then the content, each line indented by one space from the border.
func PadContent(title, content string) string {
	out := []string{" " + title, ""}
	if content != "" {
		for _, l := range strings.Split(content, "\n") {
			out = append(out, " "+l)
		}
	}
	return strings.Join(out, "\n")
}
