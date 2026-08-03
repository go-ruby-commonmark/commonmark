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
// 2026-08-03 against CommonMark spec v0.31.2: 593/652 pass (90.95%), 59 known
// gaps, concentrated in List items (21/48), HTML blocks (12/44), Emphasis
// (12/132) and Lists (8/26). See PR "gate CI on the CommonMark spec.txt".
var specKnownFailing = map[int]bool{
	4: true, 5: true, 108: true, 109: true, 169: true, 170: true, 171: true,
	172: true, 176: true, 177: true, 178: true, 179: true, 180: true, 181: true,
	182: true, 183: true, 215: true, 216: true, 254: true, 256: true, 258: true,
	259: true, 262: true, 263: true, 264: true, 270: true, 271: true, 273: true,
	274: true, 277: true, 278: true, 281: true, 282: true, 283: true, 286: true,
	287: true, 288: true, 290: true, 300: true, 307: true, 308: true, 309: true,
	312: true, 313: true, 319: true, 320: true, 324: true, 409: true, 414: true,
	415: true, 416: true, 417: true, 427: true, 431: true, 464: true, 465: true,
	466: true, 467: true, 468: true,
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
