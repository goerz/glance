package plutotohtml

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A minimal but complete notebook: a folded prose cell, a code cell, a disabled cell, and the two
// package-environment pseudo-cells Pluto always writes.
const sampleNotebook = `### A Pluto.jl notebook ###
# v0.20.24

#> [frontmatter]
#> title = "Sample"
#> description = "A tiny notebook"

using Markdown
using InteractiveUtils

# ╔═╡ aaaaaaaa-0000-0000-0000-000000000000
md"""
# Heading

Some *prose*.
"""

# ╔═╡ bbbbbbbb-0000-0000-0000-000000000000
x = 1 + 1

# ╔═╡ cccccccc-0000-0000-0000-000000000000
# ╠═╡ disabled = true
#=╠═╡
y = sqrt(2)
  ╠═╡ =#

# ╔═╡ 00000000-0000-0000-0000-000000000001
PLUTO_PROJECT_TOML_CONTENTS = """
[deps]
"""

# ╔═╡ 00000000-0000-0000-0000-000000000002
PLUTO_MANIFEST_TOML_CONTENTS = """
julia_version = "1.12.6"
"""

# ╔═╡ Cell order:
# ╟─aaaaaaaa-0000-0000-0000-000000000000
# ╠═bbbbbbbb-0000-0000-0000-000000000000
# ╠═cccccccc-0000-0000-0000-000000000000
# ╟─00000000-0000-0000-0000-000000000001
# ╟─00000000-0000-0000-0000-000000000002
`

// packedScalarOutput is a real output_packed value taken from a SpaceStation cache: body "1",
// mime "text/plain", rootassignee "x".
const packedScalarOutput = "grFwdWJsaXNoZWRfb2JqZWN0c4Cmb3V0cHV0hqRib2R5oTGwcGVyc2lzdF9qc19z" +
	"dGF0ZcKkbWltZap0ZXh0L3BsYWlusmxhc3RfcnVuX3RpbWVzdGFtcMtB2p7arMNo" +
	"rbdoYXNfcGx1dG9faG9va19mZWF0dXJlc8Kscm9vdGFzc2lnbmVloXg="

func TestIsNotebook(t *testing.T) {
	assert.True(t, IsNotebook(sampleNotebook))
	assert.True(t, IsNotebook("### A Pluto.jl notebook ###\n# v0.20.24\n"))
	assert.True(t, IsNotebook("### A Pluto.jl notebook ###"))
	assert.True(t, IsNotebook("### A Pluto.jl notebook ###\r\n# v1\n"))

	// Near-misses that must keep rendering as ordinary Julia source.
	assert.False(t, IsNotebook(""))
	assert.False(t, IsNotebook("x = 1\n"))
	assert.False(t, IsNotebook("# A comment\n### A Pluto.jl notebook ###\n"))
	assert.False(t, IsNotebook("x = 1\n### A Pluto.jl notebook ###\n"))
	assert.False(t, IsNotebook("  ### A Pluto.jl notebook ###\n"))
	assert.False(t, IsNotebook("### A Pluto.jl notebook ### trailing\n"))
}

func TestParseNotebook(t *testing.T) {
	nb, err := ParseNotebook(sampleNotebook)
	assert.NoError(t, err)
	assert.Equal(t, "v0.20.24", nb.PlutoVersion)

	// The environment pseudo-cells are dropped.
	assert.Len(t, nb.Cells, 3)

	assert.Equal(t, "aaaaaaaa-0000-0000-0000-000000000000", nb.Cells[0].ID)
	assert.True(t, nb.Cells[0].Folded)
	assert.True(t, strings.HasPrefix(nb.Cells[0].Code, `md"""`))

	assert.Equal(t, "x = 1 + 1", nb.Cells[1].Code)
	assert.False(t, nb.Cells[1].Folded)
	assert.False(t, nb.Cells[1].Disabled)

	// A disabled cell has its code unwrapped from the `#=╠═╡ … ╠═╡ =#` comment.
	assert.True(t, nb.Cells[2].Disabled)
	assert.Equal(t, "y = sqrt(2)", nb.Cells[2].Code)

	assert.Equal(t, "Sample", nb.Frontmatter["title"])
}

func TestParseNotebookCellOrderIsAuthoritative(t *testing.T) {
	// The definitions are written in the reverse of the display order, as Pluto does whenever
	// topological order and display order disagree.
	source := `### A Pluto.jl notebook ###
# v0.20.24

# ╔═╡ 22222222-0000-0000-0000-000000000000
second = 2

# ╔═╡ 11111111-0000-0000-0000-000000000000
first = 1

# ╔═╡ Cell order:
# ╠═11111111-0000-0000-0000-000000000000
# ╠═22222222-0000-0000-0000-000000000000
`
	nb, err := ParseNotebook(source)
	assert.NoError(t, err)
	assert.Len(t, nb.Cells, 2)
	assert.Equal(t, "first = 1", nb.Cells[0].Code)
	assert.Equal(t, "second = 2", nb.Cells[1].Code)
}

func TestParseNotebookWithoutCellOrder(t *testing.T) {
	source := `### A Pluto.jl notebook ###
# v0.20.24

# ╔═╡ 11111111-0000-0000-0000-000000000000
first = 1

# ╔═╡ 22222222-0000-0000-0000-000000000000
second = 2
`
	nb, err := ParseNotebook(source)
	assert.NoError(t, err)
	assert.Len(t, nb.Cells, 2)
	assert.Equal(t, "first = 1", nb.Cells[0].Code)
}

func TestProse(t *testing.T) {
	body, lang, ok := prose("md\"\"\"\n# Title\n\"\"\"")
	assert.True(t, ok)
	assert.Equal(t, langMarkdown, lang)
	assert.Equal(t, "# Title", body)

	body, lang, ok = prose(`md"one line"`)
	assert.True(t, ok)
	assert.Equal(t, langMarkdown, lang)
	assert.Equal(t, "one line", body)

	_, _, ok = prose("x = 1")
	assert.False(t, ok)

	// `md"` alone is not a complete string macro.
	_, _, ok = prose(`md"`)
	assert.False(t, ok)
}

func TestDecodeMsgpackScalarOutput(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(packedScalarOutput)
	assert.NoError(t, err)

	decoded, err := decodeMsgpack(raw)
	assert.NoError(t, err)

	top, ok := decoded.(map[string]any)
	assert.True(t, ok)

	output, ok := top["output"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "1", output["body"])
	assert.Equal(t, mimePlain, output["mime"])
	assert.Equal(t, "x", output["rootassignee"])
	assert.Equal(t, false, output["persist_js_state"])
}

func TestDecodeMsgpackPrimitives(t *testing.T) {
	cases := []struct {
		name     string
		input    []byte
		expected any
	}{
		{"nil", []byte{0xc0}, nil},
		{"true", []byte{0xc3}, true},
		{"false", []byte{0xc2}, false},
		{"positive fixint", []byte{0x2a}, int64(42)},
		{"negative fixint", []byte{0xff}, int64(-1)},
		{"int8", []byte{0xd0, 0x80}, int64(-128)},
		{"int16", []byte{0xd1, 0xff, 0x00}, int64(-256)},
		{"int32", []byte{0xd2, 0xff, 0xff, 0xff, 0x00}, int64(-256)},
		{"uint16", []byte{0xcd, 0x01, 0x00}, int64(256)},
		{"float64", []byte{0xcb, 0x3f, 0xf0, 0, 0, 0, 0, 0, 0}, float64(1)},
		{"fixstr", []byte{0xa2, 'h', 'i'}, "hi"},
		{"bin8", []byte{0xc4, 0x02, 0x01, 0x02}, []byte{1, 2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := decodeMsgpack(c.input)
			assert.NoError(t, err)
			assert.Equal(t, c.expected, v)
		})
	}
}

// Pluto packs Vector{UInt8} — every image body — as extension type 0x12 rather than as msgpack
// `bin`. Decoding that as an opaque extension is the quiet way to break every plot in a notebook.
func TestDecodeMsgpackByteArrayExtension(t *testing.T) {
	// ext8, length 4, type 0x12, PNG magic prefix
	input := []byte{0xc7, 0x04, 0x12, 0x89, 'P', 'N', 'G'}
	v, err := decodeMsgpack(input)
	assert.NoError(t, err)
	assert.Equal(t, []byte{0x89, 'P', 'N', 'G'}, v)

	bytes, ok := asBytes(v)
	assert.True(t, ok)
	assert.Equal(t, 4, len(bytes))

	// fixext4 carrying the same type
	v, err = decodeMsgpack([]byte{0xd6, 0x12, 1, 2, 3, 4})
	assert.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3, 4}, v)

	// A different extension type stays wrapped rather than being mistaken for bytes.
	v, err = decodeMsgpack([]byte{0xd7, 0x0d, 0, 0, 0, 0, 0, 0, 0, 0})
	assert.NoError(t, err)

	ext, ok := v.(Ext)
	assert.True(t, ok)
	assert.Equal(t, byte(0x0d), ext.Type)
}

func TestDecodeMsgpackRejectsTruncated(t *testing.T) {
	_, err := decodeMsgpack([]byte{0xa5, 'h', 'i'}) // claims 5 bytes, supplies 2
	assert.Error(t, err)

	_, err = decodeMsgpack([]byte{0xdd, 0xff, 0xff, 0xff, 0xff}) // array of 4 billion
	assert.Error(t, err)

	_, err = decodeMsgpack([]byte{})
	assert.Error(t, err)
}

func TestParseCache(t *testing.T) {
	source := `format = 1
pluto_version = "v0.2.6"
julia_version = "v1.12.6"
cell_order = ["bbbbbbbb-0000-0000-0000-000000000000"]

[cells.bbbbbbbb-0000-0000-0000-000000000000]
errored = false
execution_key = "291d51e057ae00cc"
mime = "text/plain"
output_packed = "` + packedScalarOutput + `"
result_hash = "4237143861eea985"
runtime_ns = 5667
text_representation = "1"
`
	cache, err := ParseCache(source)
	assert.NoError(t, err)

	out := cache.Output("bbbbbbbb-0000-0000-0000-000000000000")
	assert.NotNil(t, out)
	assert.Equal(t, mimePlain, out.MIME)
	assert.Equal(t, "1", out.Body)
	assert.Equal(t, "x", out.RootAssignee)
	assert.False(t, out.Errored)

	// An unknown cell has simply never been run.
	assert.Nil(t, cache.Output("unknown"))
}

func TestParseCacheEmptyAndUnsupported(t *testing.T) {
	cache, err := ParseCache("")
	assert.NoError(t, err)
	assert.Empty(t, cache.Cells)
	assert.Nil(t, cache.Output("anything"))

	// A nil cache must also be safe to query: Convert drops to one when the sidecar is broken.
	var missing *Cache

	assert.Nil(t, missing.Output("anything"))

	// A future sidecar layout is ignored rather than half-read.
	_, err = ParseCache("format = 2\n")
	assert.Error(t, err)

	_, err = ParseCache("this is not toml {{{")
	assert.Error(t, err)
}

func TestConvertWithoutCache(t *testing.T) {
	out, err := Convert(sampleNotebook, "")
	assert.NoError(t, err)

	assert.True(t, strings.HasPrefix(out, `<pluto-editor class="disable_ui">`))
	assert.True(t, strings.HasSuffix(out, "</pluto-editor>"))

	// The folded prose cell still renders as prose even with no cached output.
	assert.Contains(t, out, `<div class="markdown">`)
	assert.Contains(t, out, "<h1")
	assert.Contains(t, out, "Some <em>prose</em>")

	// Code cells are highlighted and the package pseudo-cells are gone.
	assert.Contains(t, out, `class="chroma"`)
	assert.NotContains(t, out, "PLUTO_PROJECT_TOML_CONTENTS")
	assert.NotContains(t, out, "PLUTO_MANIFEST_TOML_CONTENTS")

	// Frontmatter surfaces as a header.
	assert.Contains(t, out, "Sample")
}

func TestConvertWithCache(t *testing.T) {
	cache := `format = 1
cell_order = ["bbbbbbbb-0000-0000-0000-000000000000"]

[cells.bbbbbbbb-0000-0000-0000-000000000000]
errored = false
mime = "text/plain"
output_packed = "` + packedScalarOutput + `"
text_representation = "1"
`
	out, err := Convert(sampleNotebook, cache)
	assert.NoError(t, err)

	assert.Contains(t, out, "<assignee>x</assignee>")
	assert.Contains(t, out, `<pre class="no-block"><code>1</code></pre>`)
	// text/plain outputs scroll rather than being treated as rich output.
	assert.Contains(t, out, `class="scroll_y"`)
}

func TestConvertFoldedCellHidesInput(t *testing.T) {
	out, err := Convert(sampleNotebook, "")
	assert.NoError(t, err)

	// The class attribute precedes the id, so slice whole <pluto-cell> elements rather than
	// starting from the id.
	cellHTML := func(id string) string {
		for _, chunk := range strings.Split(out, "<pluto-cell ")[1:] {
			openTag, _, _ := strings.Cut(chunk, ">")
			if !strings.Contains(openTag, id) {
				continue
			}

			cell, _, _ := strings.Cut(chunk, "</pluto-cell>")

			return cell
		}

		t.Fatalf("no cell with id %s", id)

		return ""
	}

	folded := cellHTML("aaaaaaaa")
	assert.Contains(t, folded, "code_folded")
	assert.NotContains(t, folded, "<pluto-input>")

	shown := cellHTML("bbbbbbbb")
	assert.Contains(t, shown, "show_input")
	assert.Contains(t, shown, "<pluto-input>")
}

func TestConvertRejectsNonNotebook(t *testing.T) {
	_, err := Convert("x = 1\n", "")
	assert.Error(t, err)
}

// A malformed sidecar must not take the whole preview down with it.
func TestConvertSurvivesBrokenCache(t *testing.T) {
	out, err := Convert(sampleNotebook, "format = 99\ngarbage")
	assert.NoError(t, err)
	assert.Contains(t, out, "<pluto-notebook>")
}

func TestSanitizeHTML(t *testing.T) {
	// A script at the root of the fragment, which is where PlutoUI's TableOfContents puts one.
	assert.Equal(t, "", sanitizeHTML(`<script>alert(1)</script>`))
	assert.Equal(t, "<div></div>", sanitizeHTML(`<div><script>alert(1)</script></div>`))
	assert.Equal(t, "<p>kept</p>", sanitizeHTML(`<p>kept</p><script src="x.js"></script>`))

	// Case and whitespace variants are handled by the parser, not by pattern-matching.
	assert.Equal(t, "", sanitizeHTML("<SCRIPT >alert(1)</SCRIPT>"))

	assert.NotContains(t, sanitizeHTML(`<img src="x" onerror="alert(1)">`), "onerror")
	assert.NotContains(t, sanitizeHTML(`<a href="javascript:alert(1)">x</a>`), "javascript:")
	assert.Equal(t, "", sanitizeHTML(`<iframe src="http://example.com"></iframe>`))

	// Everything else survives untouched, including Pluto's custom elements.
	kept := sanitizeHTML(`<div class="markdown"><h1 id="t">Title</h1><bond def="x"></bond></div>`)
	assert.Contains(t, kept, `class="markdown"`)
	assert.Contains(t, kept, "<bond")
	assert.Contains(t, kept, `id="t"`)
}

func TestRenderImage(t *testing.T) {
	out := renderImage([]byte{0x89, 'P', 'N', 'G'}, mimePNG)
	assert.Contains(t, out, `<img src="data:image/png;base64,`)
	assert.Contains(t, out, base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G'}))

	// An SVG that arrives as text rather than bytes still renders.
	assert.Contains(t, renderImage("<svg></svg>", mimeSVG), "data:image/svg+xml;base64,")

	assert.Equal(t, "", renderImage(nil, mimePNG))
}

func TestRenderTree(t *testing.T) {
	body := map[string]any{
		keyType:        "Array",
		"prefix":       "Vector{Int64}",
		"prefix_short": "",
		"elements": []any{
			[]any{int64(1), []any{"10", mimePlain}},
			[]any{int64(2), []any{"20", mimePlain}},
			"more",
		},
	}
	out := renderTree(body, 0)

	assert.Contains(t, out, `<pluto-tree class="collapsed Array">`)
	assert.Contains(t, out, `<pluto-tree-items class="Array">`)
	assert.Contains(t, out, "<p-r><p-k>1</p-k><p-v>")
	assert.Contains(t, out, "Vector{Int64}")
	// Truncated output is marked, and disabled: there is no server to ask for the rest.
	assert.Contains(t, out, `<pluto-tree-more class="disabled"`)
}

func TestRenderTreeSetOmitsKeys(t *testing.T) {
	body := map[string]any{
		keyType:    "Set",
		"prefix":   "Set{Int64}",
		"elements": []any{[]any{int64(1), []any{"10", mimePlain}}},
	}
	out := renderTree(body, 0)
	assert.Contains(t, out, "<p-r><p-v>")
	assert.NotContains(t, out, "<p-k>")
}

func TestRenderTreePairAndCircular(t *testing.T) {
	pair := renderTree(map[string]any{
		keyType:     "Pair",
		"key_value": []any{[]any{"a", mimePlain}, []any{"1", mimePlain}},
	}, 0)
	assert.Contains(t, pair, "<pluto-tree-pair")
	assert.Contains(t, pair, "<p-k>")

	assert.Equal(t, "<em>circular reference</em>",
		renderTree(map[string]any{keyType: "circular"}, 0))
}

func TestRenderTable(t *testing.T) {
	body := map[string]any{
		"schema": map[string]any{
			"names": []any{"a", "b"},
			"types": []any{"Int64", "String"},
		},
		"rows": []any{
			[]any{int64(1), []any{[]any{"1", mimePlain}, []any{"x", mimePlain}}},
			"more",
		},
	}
	out := renderTable(body, 0)
	assert.Contains(t, out, `<table class="pluto-table">`)
	assert.Contains(t, out, `<tr class="schema-names">`)
	assert.Contains(t, out, `<tr class="schema-types">`)
	assert.Contains(t, out, "pluto-tree-more-td")
}

func TestRenderStacktrace(t *testing.T) {
	out := renderStacktrace(map[string]any{
		"msg":         "UndefVarError: `foo` not defined",
		"plain_error": "UndefVarError: `foo` not defined\nStacktrace:\n [1] top-level scope",
	})
	assert.Contains(t, out, "<jlerror>")
	assert.Contains(t, out, "UndefVarError")
	assert.Contains(t, out, "Stacktrace:")
	// The message is escaped, not interpolated as markup.
	assert.NotContains(t, out, "<script")
}

func TestRenderReactDOMElementRejectsUnsafeTags(t *testing.T) {
	assert.Equal(t, "", renderReactDOMElement(map[string]any{keyTag: "script"}, 0))
	assert.Equal(t, "", renderReactDOMElement(map[string]any{keyTag: "iframe"}, 0))

	out := renderReactDOMElement(map[string]any{
		keyTag:       tagDiv,
		"attributes": map[string]any{"class": "x", "onclick": "alert(1)"},
		"children":   []any{"hello"},
	}, 0)
	assert.Contains(t, out, `class="x"`)
	assert.NotContains(t, out, "onclick")
	assert.Contains(t, out, "hello")
}

func TestStripANSI(t *testing.T) {
	assert.Equal(t, "plain", stripANSI("plain"))
	assert.Equal(t, "red text", stripANSI("\x1b[31mred text\x1b[0m"))
}

func TestFullPageHTMLIsNotSpliced(t *testing.T) {
	out := renderRawHTML("<!DOCTYPE html><html><body>hi</body></html>")
	assert.Contains(t, out, "full-page HTML output")
	assert.NotContains(t, out, "<html")
}

// `md"…"` cells are Julia Markdown, not CommonMark: ```math fences and double-backtick spans are
// math, and LaTeX must survive a parser that would otherwise treat `\{` and `\\` as escapes.
func TestJuliaMarkdownMath(t *testing.T) {
	source := "Text with ``\\hat{A}`` inline.\n\n" +
		"```math\n\\lambda_{\\pm} = \\frac{a}{2} \\\\ b\n```\n\n" +
		"And ``x`` again.\n"

	rewritten, segments := extractJuliaMath(source)
	assert.Len(t, segments, 3)
	assert.NotContains(t, rewritten, "\\hat")
	assert.NotContains(t, rewritten, "```math")

	assert.Equal(t, "\\hat{A}", segments[0].latex)
	assert.False(t, segments[0].display)
	assert.Equal(t, "\\lambda_{\\pm} = \\frac{a}{2} \\\\ b", segments[1].latex)
	assert.True(t, segments[1].display)

	restored := restoreJuliaMath(rewritten, segments)
	assert.Contains(t, restored, "$\\hat{A}$")
	assert.Contains(t, restored, "$$\\lambda_{\\pm}")
	// The double backslash of a LaTeX line break must survive intact.
	assert.Contains(t, restored, "\\\\ b$$")
}

func TestJuliaMarkdownLeavesCodeFencesAlone(t *testing.T) {
	source := "```julia\nx = 1  # ``not math``\n```\n"

	rewritten, segments := extractJuliaMath(source)
	assert.Empty(t, segments)
	assert.Equal(t, source, rewritten)
}

func TestRenderProseKeepsLaTeXIntact(t *testing.T) {
	out := renderProse("A matrix ``\\begin{pmatrix} a & b \\\\ c & d\\end{pmatrix}`` inline.",
		langMarkdown)

	assert.Contains(t, out, `<div class="markdown">`)
	// Escaped for HTML, but structurally unharmed: no CommonMark escape swallowed a backslash.
	assert.Contains(t, out, "\\begin{pmatrix}")
	assert.Contains(t, out, "\\\\ c &amp; d")
	assert.NotContains(t, out, "<code>")
}
