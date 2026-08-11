package plutotohtml

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Julia's Markdown flavour, which is what a `md"…"` cell contains, is not CommonMark. Two
// differences matter for a preview:
//
//	```math          fenced blocks are display math
//	``\alpha``       double backticks are inline math (single backticks are still code)
//
// Goldmark renders both as code, so the math would show as raw LaTeX in a grey box. Worse, feeding
// LaTeX through a CommonMark parser is lossy: `\{`, `\\`, `\_` and `\&` are all valid backslash
// escapes in CommonMark, so a matrix like `\begin{pmatrix} a \\ b \end{pmatrix}` comes out mangled.
//
// So the math is lifted out before conversion and put back afterwards, as the `$$…$$` and `$…$`
// that the bundled KaTeX picks up. This only applies to cells with no cached output — when the
// sidecar has one, Julia already rendered the Markdown and there is nothing to guess at.

// inlineMathPattern matches Julia's double-backtick inline math. The body cannot contain a
// backtick, so this can never span a fence delimiter.
var inlineMathPattern = regexp.MustCompile("``([^`]+)``")

// mathPlaceholder is deliberately bare lowercase alphanumeric: Markdown leaves it alone, neither
// escaping it nor treating any part of it as punctuation.
const mathPlaceholder = "xplutomathx%dx"

var placeholderPattern = regexp.MustCompile(`xplutomathx(\d+)x`)

type mathSegment struct {
	latex   string
	display bool
}

// extractJuliaMath replaces every math segment with a placeholder, returning the rewritten source
// and the segments in placeholder order.
func extractJuliaMath(source string) (string, []mathSegment) {
	var segments []mathSegment

	placeholder := func(latex string, display bool) string {
		segments = append(segments, mathSegment{latex: latex, display: display})

		return strings.ReplaceAll(mathPlaceholder, "%d", strconv.Itoa(len(segments)-1))
	}

	var (
		out       []string
		fenceMath []string
	)

	inFence, inMathFence := false, false

	for line := range strings.SplitSeq(source, "\n") {
		if info, isFence := strings.CutPrefix(strings.TrimSpace(line), "```"); isFence {
			keep, math := stepFence(&inFence, &inMathFence, info, &fenceMath)
			if math != nil {
				out = append(out, placeholder(strings.Join(*math, "\n"), true))
			}

			if keep {
				out = append(out, line)
			}

			continue
		}

		switch {
		case inMathFence:
			fenceMath = append(fenceMath, line)
		case inFence:
			out = append(out, line) // a real code block; leave it alone
		default:
			out = append(out, inlineMathPattern.ReplaceAllStringFunc(line, func(m string) string {
				return placeholder(strings.Trim(m, "`"), false)
			}))
		}
	}

	// An unterminated ```math fence still has content worth showing.
	if inMathFence {
		out = append(out, placeholder(strings.Join(fenceMath, "\n"), true))
	}

	return strings.Join(out, "\n"), segments
}

// restoreJuliaMath puts the extracted math back into the rendered HTML as KaTeX delimiters. The
// LaTeX is HTML-escaped, exactly as Pluto does for its own `<p class="tex">` output: KaTeX reads
// element text content, so entities are resolved before it ever sees them.
func restoreJuliaMath(rendered string, segments []mathSegment) string {
	if len(segments) == 0 {
		return rendered
	}

	return placeholderPattern.ReplaceAllStringFunc(rendered, func(m string) string {
		index, err := strconv.Atoi(placeholderPattern.FindStringSubmatch(m)[1])
		if err != nil || index >= len(segments) {
			return m
		}

		segment := segments[index]
		delimiter := "$"

		if segment.display {
			delimiter = "$$"
		}

		return delimiter + html.EscapeString(segment.latex) + delimiter
	})
}

// stepFence advances the fence state machine for one ``` line. It reports whether the delimiter
// line itself should be kept in the output, and returns the collected lines when a math fence
// just closed.
func stepFence(inFence, inMathFence *bool, info string, collected *[]string) (bool, *[]string) {
	switch {
	case !*inFence:
		*inFence = true
		*inMathFence = strings.TrimSpace(info) == "math"
		*collected = nil

		return !*inMathFence, nil
	case *inMathFence:
		*inFence, *inMathFence = false, false
		done := *collected

		return false, &done
	default:
		*inFence = false

		return true, nil
	}
}
