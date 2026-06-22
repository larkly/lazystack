package servercreate

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var templatePlaceholderRe = regexp.MustCompile(`\{(n)(?::(\d+)d)?\}|\{(timestamp)\}|\{(random)\}`)

// expandNameTemplate replaces placeholders in a name template.
// Supported placeholders:
//   {n}        - zero-based index
//   {n:02d}    - zero-padded index (any width)
//   {timestamp} - Unix timestamp in seconds
//   {random}    - 6-character hex random string
func expandNameTemplate(tmpl string, idx int) string {
	now := time.Now().Unix()
	rnd := fmt.Sprintf("%06x", rand.Intn(0xFFFFFF))
	return templatePlaceholderRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		switch {
		case m == "{timestamp}":
			return strconv.FormatInt(now, 10)
		case m == "{random}":
			return rnd
		case strings.HasPrefix(m, "{n:") && strings.HasSuffix(m, "d}"):
			widthStr := m[3 : len(m)-2]
			width, err := strconv.Atoi(widthStr)
			if err != nil || width < 1 {
				return m
			}
			return fmt.Sprintf("%0*d", width, idx)
		case m == "{n}":
			return strconv.Itoa(idx)
		default:
			return m
		}
	})
}

// previewNames generates a preview of up to limit names from a template.
func previewNames(tmpl string, count int, limit int) []string {
	if tmpl == "" || count <= 1 {
		return nil
	}
	if limit > count {
		limit = count
	}
	out := make([]string, limit)
	for i := 0; i < limit; i++ {
		out[i] = expandNameTemplate(tmpl, i)
	}
	return out
}
