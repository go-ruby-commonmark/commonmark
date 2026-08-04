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

// specKnownFailing is the frozen set of CommonMark spec.txt examples that this
// renderer does not yet match byte-for-byte against the canonical dart/cmark
// reference output. It is a conformance RATCHET: every example NOT listed here
// must pass (TestSpecConformance is a hard CI gate), so no change may introduce
// a new incompatibility. When a listed example is fixed, the test flags it and
// the entry must be removed here — the set only ever shrinks. Baseline captured
// 2026-08-03 against CommonMark spec v0.31.2: 593/652 pass (90.95%). Ratcheted
// 2026-08-04 to 621/652 (95.25%) after fixing loose/tight list detection
// (lastLineBlank propagation up the ancestor chain), then to 635/652 (97.39%)
// after making block finalize idempotent so a type-1..5 HTML block closed
// mid-document is not re-finalized (which had clobbered its Literal), then to
// 647/652 (99.23%) after fixing delimiter-run emphasis: a closer that still
// carries delimiters after forming a span is now reused to match a further
// opener (e.g. ***foo*** -> <em><strong>). Then to 649/652 (99.54%) after the
// setext-underline matcher resolves leading link reference definitions and
// declines to form a heading from a paragraph that was only definitions (215,
// 216). Residual gaps: list-marker-indent edge cases (300, 312, 313).
var specKnownFailing = map[int]bool{
	300: true, 312: true, 313: true,
}

// TestSpecConformance is the differential conformance gate against the canonical
// CommonMark spec.txt corpus (github.com/commonmark/commonmark-spec) — the same
// "gate on the reference's own conformance suite" move that lifted go-scss from
// a self-tested 85% to an sass-spec-verified 98.46%. The spec's reference
// renderer is "unsafe" (raw HTML and unfiltered URLs pass through), so we match
// that here. Every example outside specKnownFailing MUST render byte-exact; a
// new failure fails CI, and a known failure that now passes is reported so the
// ratchet can be tightened.
func TestSpecConformance(t *testing.T) {
	examples := parseSpecExamples(t)
	if len(examples) < 600 {
		t.Fatalf("expected ~652 spec examples, parsed %d", len(examples))
	}
	opts := &Options{Unsafe: true}
	pass := 0
	var newFail, fixed []int
	for _, ex := range examples {
		ok := ToHTML(ex.markdown, opts) == ex.html
		if ok {
			pass++
		}
		switch {
		case ok && specKnownFailing[ex.number]:
			fixed = append(fixed, ex.number)
		case !ok && !specKnownFailing[ex.number]:
			newFail = append(newFail, ex.number)
		}
	}
	t.Logf("CommonMark spec.txt v0.31.2: %d/%d examples pass (%.2f%%), %d known gaps",
		pass, len(examples), 100*float64(pass)/float64(len(examples)), len(specKnownFailing))
	if len(fixed) > 0 {
		t.Errorf("examples now passing that are still listed in specKnownFailing: %v\n"+
			"remove them from specKnownFailing to tighten the conformance ratchet", fixed)
	}
	if len(newFail) > 0 {
		t.Errorf("REGRESSION: %d spec example(s) that must pass now fail: %v", len(newFail), newFail)
	}
}
