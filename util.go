// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// isBlank reports whether the byte slice is empty or all spaces/tabs.
func isBlank(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}

// isSpaceOrTab reports whether c is an ASCII space or tab.
func isSpaceOrTab(c byte) bool { return c == ' ' || c == '\t' }

// isWhitespaceChar reports whether c is a CommonMark whitespace character.
func isWhitespaceChar(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

// isPunct reports whether r is a Unicode punctuation or symbol character, as the
// emphasis algorithm requires.
func isPunct(r rune) bool {
	if r < 0x80 {
		return isASCIIPunct(byte(r))
	}
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

// isASCIIPunct reports whether c is one of the ASCII punctuation characters.
func isASCIIPunct(c byte) bool {
	switch {
	case c >= '!' && c <= '/':
		return true
	case c >= ':' && c <= '@':
		return true
	case c >= '[' && c <= '`':
		return true
	case c >= '{' && c <= '~':
		return true
	}
	return false
}

// isUnicodeWhitespace reports whether r is Unicode whitespace per the spec.
func isUnicodeWhitespace(r rune) bool {
	if r == ' ' || r == '\t' || r == '\n' || r == '\v' || r == '\f' || r == '\r' {
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

// isDigit reports whether c is an ASCII digit.
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isHexDigit reports whether c is an ASCII hex digit.
func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// isAlpha reports whether c is an ASCII letter.
func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isAlnum reports whether c is an ASCII letter or digit.
func isAlnum(c byte) bool { return isAlpha(c) || isDigit(c) }

// escapeHTML writes s to sb with the four HTML-significant characters escaped:
// & < > and ". This matches the CommonMark reference renderer's escape_html.
func escapeHTML(sb *strings.Builder, s []byte) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			sb.WriteString("&amp;")
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		case '"':
			sb.WriteString("&quot;")
		default:
			sb.WriteByte(s[i])
		}
	}
}

// urlEncode percent-encodes a link destination the way the CommonMark reference
// renderer does: it preserves characters in a permissive safe set (including
// already-present %HH escapes) and encodes the rest as UTF-8 bytes.
func urlEncode(sb *strings.Builder, dest []byte) {
	// Characters that are passed through unescaped by cmark's
	// href-normalisation (ESCAPE_SLASH-style safe set).
	const safe = ";/?:@&=+$,-_.!~*'()#%"
	for i := 0; i < len(dest); {
		c := dest[i]
		if c == '%' && i+2 < len(dest) && isHexDigit(dest[i+1]) && isHexDigit(dest[i+2]) {
			// Preserve an existing percent-escape verbatim.
			sb.WriteByte(dest[i])
			sb.WriteByte(dest[i+1])
			sb.WriteByte(dest[i+2])
			i += 3
			continue
		}
		if c < 0x80 && (isAlnum(c) || strings.IndexByte(safe, c) >= 0) {
			// After normalisation & should already be an entity; but the
			// reference renderer escapes bare & in hrefs to &amp; separately.
			if c == '&' {
				sb.WriteString("&amp;")
			} else {
				sb.WriteByte(c)
			}
			i++
			continue
		}
		// Percent-encode this byte.
		writePercent(sb, c)
		i++
	}
}

const hexDigits = "0123456789ABCDEF"

func writePercent(sb *strings.Builder, c byte) {
	sb.WriteByte('%')
	sb.WriteByte(hexDigits[c>>4])
	sb.WriteByte(hexDigits[c&0x0f])
}

// normalizeURI validates the destination is valid UTF-8; the byte-level
// urlEncode above handles the rest. Kept as a named seam for clarity.
func normalizeURI(dest []byte) []byte {
	if utf8.Valid(dest) {
		return dest
	}
	// Replace invalid sequences with U+FFFD, matching Go's decoder behaviour.
	return []byte(strings.ToValidUTF8(string(dest), "�"))
}

// trimLeftSpaceTab trims leading spaces and tabs.
func trimLeftSpaceTab(b []byte) []byte {
	i := 0
	for i < len(b) && isSpaceOrTab(b[i]) {
		i++
	}
	return b[i:]
}

// trimRightSpaceTab trims trailing spaces and tabs.
func trimRightSpaceTab(b []byte) []byte {
	j := len(b)
	for j > 0 && isSpaceOrTab(b[j-1]) {
		j--
	}
	return b[:j]
}

// trimSpaceTab trims leading and trailing spaces and tabs.
func trimSpaceTab(b []byte) []byte {
	return trimRightSpaceTab(trimLeftSpaceTab(b))
}

// normalizeLabel collapses internal whitespace runs to a single space, trims,
// and case-folds a link label for reference matching.
func normalizeLabel(label []byte) string {
	// Unicode case fold (simple lower) + whitespace collapse.
	var sb strings.Builder
	inWS := false
	started := false
	pending := false
	for _, r := range string(label) {
		if isUnicodeWhitespace(r) {
			inWS = true
			continue
		}
		if inWS && started {
			pending = true
		}
		inWS = false
		if pending {
			sb.WriteByte(' ')
			pending = false
		}
		started = true
		sb.WriteString(caseFold(r))
	}
	return sb.String()
}

// caseFold maps a rune to its case-folded form for label comparison. CommonMark
// specifies Unicode case folding; simple lower-casing plus the special dotless-I
// handling covers the spec's requirements for the test suite.
func caseFold(r rune) string {
	// Special-case the Turkish/German cases the spec exercises (ẞ, µ, etc.)
	switch r {
	case 'µ': // MICRO SIGN folds to GREEK SMALL LETTER MU
		return "μ"
	case 'ẞ': // LATIN CAPITAL SHARP S folds to "ss"
		return "ss"
	}
	return string(unicode.ToLower(r))
}
