package diagnose

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// SplitSelector splits a CSS selector group into alternatives, and each
// alternative into cumulative prefixes at its top-level combinators. For
// "a b > c" it returns [["a", "a b", "a b > c"]]. Brackets, parentheses,
// and quotes are respected.
func SplitSelector(sel string) [][]string {
	var groups [][]string
	for _, alt := range splitTopLevel(sel, ',') {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		groups = append(groups, prefixes(alt))
	}
	return groups
}

func splitTopLevel(s string, sep rune) []string {
	var parts []string
	depth := 0
	var quote rune
	start := 0
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '(' || r == '[':
			depth++
		case r == ')' || r == ']':
			depth--
		case r == sep && depth == 0:
			parts = append(parts, s[start:i])
			start = i + len(string(r))
		}
	}
	return append(parts, s[start:])
}

// prefixes returns the selector text up to the end of each compound.
func prefixes(alt string) []string {
	var out []string
	depth := 0
	var quote rune
	inCompound := false
	for i, r := range alt {
		isComb := false
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '(' || r == '[':
			depth++
		case r == ')' || r == ']':
			depth--
		case depth == 0 && (r == ' ' || r == '\t' || r == '\n' || r == '>' || r == '+' || r == '~'):
			isComb = true
		}
		if isComb {
			if inCompound {
				out = append(out, strings.TrimSpace(alt[:i]))
				inCompound = false
			}
			continue
		}
		inCompound = true
	}
	if inCompound {
		out = append(out, strings.TrimSpace(alt))
	}
	return out
}

// Trace evaluates every prefix of sel inside scope and returns the steps of
// the alternative that got deepest, plus the deepest matched selection.
func Trace(scope *goquery.Selection, sel string) ([]TraceStep, *goquery.Selection) {
	var best []TraceStep
	var bestSel *goquery.Selection
	bestDepth := -1
	for _, group := range SplitSelector(sel) {
		var steps []TraceStep
		var deepest *goquery.Selection
		depth := 0
		for _, p := range group {
			m := scope.Find(p)
			steps = append(steps, TraceStep{Selector: p, Matches: m.Length()})
			if m.Length() == 0 {
				break
			}
			deepest = m.First()
			depth++
		}
		if depth > bestDepth {
			best, bestSel, bestDepth = steps, deepest, depth
		}
	}
	return best, bestSel
}
