package shared

// Glyph returns unicode normally and ascii in plain mode, for symbols that
// should degrade to text on terminals without Unicode support.
func Glyph(unicode, ascii string) string {
	if PlainMode {
		return ascii
	}
	return unicode
}
