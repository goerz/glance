// Package plutotohtml renders a Pluto.jl notebook to static HTML.
//
// A notebook is two files: the `.jl` itself, which holds the code, the display order and the fold
// state, and an optional sidecar `<notebook>.jl.pluto-cache.toml` written by SpaceStation, which
// holds every cell's last output. Together they contain everything Pluto's own frontend receives,
// so the notebook can be reproduced offline — without Julia, without a server, and without Pluto's
// multi-megabyte JavaScript bundle.
//
// The markup deliberately reuses Pluto's custom element names (pluto-cell, pluto-output,
// pluto-tree, p-r/p-k/p-v, jlerror) so that stylesheets lifted from Pluto's frontend apply
// unchanged.
package plutotohtml

import (
	"bytes"
	"errors"
	"html"
	"strings"

	"github.com/alecthomas/chroma"
	htmlFormatter "github.com/alecthomas/chroma/formatters/html"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting"
	"github.com/yuin/goldmark/extension"
)

// errNotANotebook is returned for a `.jl` file that is not a Pluto notebook. Callers route those
// to the plain source-code preview instead.
var errNotANotebook = errors.New("not a Pluto notebook")

// markdownParser mirrors the one in htmlconverter.go, for prose cells in notebooks that have never
// been run and so have no cached HTML output.
var markdownParser = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		highlighting.NewHighlighting(
			highlighting.WithFormatOptions(htmlFormatter.WithClasses(true)),
		),
	),
)

// Convert renders a Pluto notebook to an HTML fragment.
//
// cacheSource is the contents of the `.pluto-cache.toml` sidecar, or "" when it is missing or
// unreadable — a notebook that has never been run still renders, just without outputs.
func Convert(source string, cacheSource string) (string, error) {
	if !IsNotebook(source) {
		return "", errNotANotebook
	}

	notebook, err := ParseNotebook(source)
	if err != nil {
		return "", err
	}

	// A malformed or unsupported sidecar is not fatal: drop to the code-only rendering rather
	// than failing the preview outright.
	cache, err := ParseCache(cacheSource)
	if err != nil {
		cache = nil
	}

	var b strings.Builder
	b.WriteString("<pluto-editor class=\"disable_ui\"><main><pluto-notebook>")
	b.WriteString(renderFrontmatter(notebook.Frontmatter))

	for _, cell := range notebook.Cells {
		b.WriteString(renderCell(cell, cache.Output(cell.ID)))
	}

	b.WriteString("</pluto-notebook></main></pluto-editor>")

	return b.String(), nil
}

func renderCell(cell Cell, out *Output) string {
	outputHTML, mime := renderCellOutput(cell, out)

	assignee := ""
	if out != nil {
		assignee = out.RootAssignee
	}

	var b strings.Builder
	b.WriteString("<pluto-cell class=\"" + strings.Join(cellClasses(cell, out), " ") + "\" id=\"" +
		html.EscapeString(cell.ID) + "\">")

	// Pluto puts the output above the input; keeping that order is what makes the preview read
	// as a Pluto notebook rather than as a source listing.
	b.WriteString("<pluto-output class=\"" +
		strings.Join(outputClasses(mime, outputHTML, out), " ") + "\" mime=\"" +
		html.EscapeString(mime) + "\">")
	b.WriteString("<assignee>" + html.EscapeString(assignee) + "</assignee>")
	b.WriteString(outputHTML)
	b.WriteString("</pluto-output>")

	if !cell.Folded {
		b.WriteString("<pluto-input>" + highlightJulia(cell.Code) + "</pluto-input>")
	}

	b.WriteString("</pluto-cell>")

	return b.String()
}

// cellClasses mirrors the class list Pluto's Cell.js builds, restricted to the states a static
// render can be in.
func cellClasses(cell Cell, out *Output) []string {
	classes := []string{"show_input"}
	if cell.Folded {
		classes = []string{"code_folded"}
	}

	if out == nil {
		classes = append(classes, "no_output_yet")
	} else if out.Errored {
		classes = append(classes, "errored")
	}

	if cell.Disabled {
		classes = append(classes, "running_disabled")
	}

	return classes
}

// outputClasses mirrors CellOutput.js: everything except the three plain-ish mimes counts as rich
// output, and tables and plain text get their own scroll container.
func outputClasses(mime string, outputHTML string, out *Output) []string {
	rich := outputHTML == "" ||
		(out != nil && out.Errored) ||
		(mime != mimeTree && mime != mimeTable && mime != mimePlain)

	classes := []string{}
	if rich {
		classes = append(classes, "rich_output")
	}

	if mime == mimeTable || mime == mimePlain {
		classes = append(classes, "scroll_y")
	}

	return classes
}

// renderCellOutput produces a cell's output markup and the mime it was rendered as.
func renderCellOutput(cell Cell, out *Output) (string, string) {
	if out != nil {
		if rendered := renderOutputBody(out.Body, out.MIME, 0, false); rendered != "" {
			return rendered, out.MIME
		}
		// The body was absent or of a shape we do not render; the sidecar's plain-text digest is
		// the graceful degradation Pluto already provides for exactly this case.
		if out.Text != "" {
			return "<div>" + preformatted(out.Text) + "</div>", mimePlain
		}

		return "", out.MIME
	}

	// No cached output. A folded cell would render as a blank box, which for a prose cell — the
	// overwhelmingly common folded cell — loses the entire point of the notebook.
	if cell.Folded {
		if body, lang, ok := prose(cell.Code); ok {
			return renderProse(body, lang), mimeHTML
		}
		// Folded, not prose, never run: show the code rather than an empty box.
		return "<div class=\"pluto-preview-note\">" + highlightJulia(cell.Code) + "</div>", mimeHTML
	}

	return "", ""
}

func renderProse(body string, lang string) string {
	if lang == langHTML {
		return "<div class=\"raw-html-wrapper\">" + sanitizeHTML(body) + "</div>"
	}

	// Lift the math out before Goldmark sees it: `md"…"` is Julia's Markdown flavour, not
	// CommonMark, and a CommonMark parser both mis-reads its math syntax and eats LaTeX
	// backslash escapes.
	source, math := extractJuliaMath(body)

	var buf bytes.Buffer

	err := markdownParser.Convert([]byte(source), &buf)
	if err != nil {
		return "<pre class=\"no-block\"><code>" + html.EscapeString(body) + "</code></pre>"
	}
	// `div.markdown` is the wrapper Julia's own Markdown.html emits and that Pluto's CSS styles,
	// so a never-run prose cell matches a cached one.
	return "<div class=\"raw-html-wrapper\"><div class=\"markdown\">" +
		restoreJuliaMath(buf.String(), math) + "</div></div>"
}

func renderFrontmatter(fm map[string]any) string {
	if fm == nil {
		return ""
	}

	title, _ := fm["title"].(string)

	description, _ := fm["description"].(string)
	if title == "" && description == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("<header class=\"pluto-frontmatter\">")

	if title != "" {
		b.WriteString("<h1>" + html.EscapeString(title) + "</h1>")
	}

	if description != "" {
		b.WriteString("<p>" + html.EscapeString(description) + "</p>")
	}

	b.WriteString("</header>")

	return b.String()
}

// highlightJulia renders cell code with Chroma, matching how CodePreview highlights a plain `.jl`
// file so the two previews share a stylesheet.
func highlightJulia(code string) string {
	lexer := lexers.Get("julia")
	if lexer == nil {
		lexer = lexers.Fallback
	}

	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return "<pre class=\"chroma\"><code>" + html.EscapeString(code) + "</code></pre>"
	}

	var buf bytes.Buffer

	formatter := htmlFormatter.New(htmlFormatter.WithClasses(true))

	err = formatter.Format(&buf, styles.GitHub, iterator)
	if err != nil {
		return "<pre class=\"chroma\"><code>" + html.EscapeString(code) + "</code></pre>"
	}

	return buf.String()
}
