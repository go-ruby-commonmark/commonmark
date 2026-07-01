// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import (
	_ "embed"
	"strings"
	"testing"
)

//go:embed spec.txt
var specTxt string

// specExample is one numbered markdown->HTML conformance example.
type specExample struct {
	number   int
	markdown string
	html     string
	section  string
}

// parseSpecExamples extracts the numbered examples from the CommonMark spec.txt.
// The spec uses a run of 32 backticks + " example" to open an example, a line
// containing only "." to separate markdown from expected HTML, and a run of 32
// backticks to close it. Tabs are written as U+2192 in the spec source.
func parseSpecExamples(t *testing.T) []specExample {
	t.Helper()
	const fence = "````````````````````````````````"
	lines := strings.Split(specTxt, "\n")
	var examples []specExample
	num := 0
	section := ""
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		if strings.HasPrefix(ln, "#") {
			section = strings.TrimSpace(strings.TrimLeft(ln, "#"))
			continue
		}
		if ln != fence+" example" {
			continue
		}
		// Collect markdown until the "." separator.
		var md, html []string
		i++
		for i < len(lines) && lines[i] != "." {
			md = append(md, lines[i])
			i++
		}
		i++ // skip "."
		for i < len(lines) && lines[i] != fence {
			html = append(html, lines[i])
			i++
		}
		num++
		mdText := joinLines(md)
		htmlText := joinLines(html)
		examples = append(examples, specExample{
			number:   num,
			markdown: unescapeArrows(mdText),
			html:     unescapeArrows(htmlText),
			section:  section,
		})
	}
	return examples
}

func joinLines(ls []string) string {
	if len(ls) == 0 {
		return ""
	}
	return strings.Join(ls, "\n") + "\n"
}

// unescapeArrows replaces the spec's tab visualisation (U+2192) with a real tab.
func unescapeArrows(s string) string {
	return strings.ReplaceAll(s, "→", "\t")
}

// TestSpecExamples runs every CommonMark spec example and reports the pass rate.
// The reference renderer used by the spec is "unsafe" (it passes raw HTML and
// unfiltered URLs through), so we set that option here.
func TestSpecExamples(t *testing.T) {
	examples := parseSpecExamples(t)
	if len(examples) < 600 {
		t.Fatalf("expected ~652 spec examples, parsed %d", len(examples))
	}
	opts := &Options{Unsafe: true}
	pass := 0
	var failed []int
	for _, ex := range examples {
		got := ToHTML(ex.markdown, opts)
		if got == ex.html {
			pass++
		} else {
			failed = append(failed, ex.number)
		}
	}
	t.Logf("CommonMark spec.txt: %d/%d examples pass", pass, len(examples))
	if len(failed) > 0 {
		t.Logf("failing examples: %v", failed)
	}
}
