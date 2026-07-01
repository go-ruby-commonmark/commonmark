// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import "bytes"

// parseLinkDestination parses a link destination at s.pos. It accepts either an
// angle-bracket form `<...>` or a bare form (balanced parentheses, no spaces or
// control chars). On success it advances s.pos past the destination and returns
// the raw (backslash/entity-unresolved) destination bytes.
func parseLinkDestination(s *subject) ([]byte, bool) {
	if s.pos >= len(s.buf) {
		return nil, false
	}
	if s.buf[s.pos] == '<' {
		i := s.pos + 1
		var out []byte
		for i < len(s.buf) {
			c := s.buf[i]
			if c == '>' {
				s.pos = i + 1
				return unescapeString(out), true
			}
			if c == '\n' || c == '<' {
				return nil, false
			}
			if c == '\\' && i+1 < len(s.buf) && isASCIIPunct(s.buf[i+1]) {
				out = append(out, c, s.buf[i+1])
				i += 2
				continue
			}
			out = append(out, c)
			i++
		}
		return nil, false
	}

	// Bare destination.
	i := s.pos
	depth := 0
	var out []byte
	for i < len(s.buf) {
		c := s.buf[i]
		if c == '\\' && i+1 < len(s.buf) && isASCIIPunct(s.buf[i+1]) {
			out = append(out, c, s.buf[i+1])
			i += 2
			continue
		}
		if c == '(' {
			depth++
			out = append(out, c)
			i++
			continue
		}
		if c == ')' {
			if depth == 0 {
				break
			}
			depth--
			out = append(out, c)
			i++
			continue
		}
		if isWhitespaceChar(c) || isControl(c) {
			break
		}
		out = append(out, c)
		i++
	}
	if depth != 0 {
		return nil, false
	}
	if i == s.pos {
		// Empty destination is allowed (e.g. `[a]()`).
		s.pos = i
		return unescapeString(out), true
	}
	s.pos = i
	return unescapeString(out), true
}

func isControl(c byte) bool { return c < 0x20 || c == 0x7f }

// parseLinkTitle parses an optional link title delimited by ", ', or (...).
// On success it advances s.pos and returns the unescaped title.
func parseLinkTitle(s *subject) ([]byte, bool) {
	if s.pos >= len(s.buf) {
		return nil, false
	}
	quote := s.buf[s.pos]
	var closer byte
	switch quote {
	case '"':
		closer = '"'
	case '\'':
		closer = '\''
	case '(':
		closer = ')'
	default:
		return nil, false
	}
	i := s.pos + 1
	var out []byte
	for i < len(s.buf) {
		c := s.buf[i]
		if c == '\\' && i+1 < len(s.buf) && isASCIIPunct(s.buf[i+1]) {
			out = append(out, c, s.buf[i+1])
			i += 2
			continue
		}
		if c == closer {
			s.pos = i + 1
			return unescapeString(out), true
		}
		if quote == '(' && c == '(' {
			// Unescaped ( inside a (...) title is not allowed.
			return nil, false
		}
		out = append(out, c)
		i++
	}
	return nil, false
}

// unescapeString resolves backslash escapes and entity references in a raw
// string (used for destinations, titles, info strings). It returns new bytes.
func unescapeString(b []byte) []byte {
	if bytes.IndexByte(b, '\\') < 0 && bytes.IndexByte(b, '&') < 0 {
		return append([]byte{}, b...)
	}
	var out []byte
	for i := 0; i < len(b); {
		c := b[i]
		if c == '\\' && i+1 < len(b) && isASCIIPunct(b[i+1]) {
			out = append(out, b[i+1])
			i += 2
			continue
		}
		if c == '&' {
			if repl, length := matchEntity(b[i:]); length > 0 {
				out = append(out, repl...)
				i += length
				continue
			}
		}
		out = append(out, c)
		i++
	}
	return out
}
