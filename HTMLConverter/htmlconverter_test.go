package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
)

var minifier *minify.M

func minifyHTML(htmlString string) string {
	// Initialize minifier if necessary
	if minifier == nil {
		minifier = minify.New()
		minifier.Add("text/html", &html.Minifier{KeepEndTags: true, KeepQuotes: true})
	}

	minified, err := minifier.String("text/html", htmlString)
	if err != nil {
		panic(fmt.Sprintf("Could not minify HTML: %d", err))
	}

	return minified
}

func TestConvertCodeToHTML(t *testing.T) {
	source := `const print = (text) => console.log(text);
print("Hello world");`
	actual := convertToGoString(convertCodeToHTML(convertToCString(source), convertToCString("js")))
	actualTrimmed := strings.TrimSpace(actual)
	assert.True(t, strings.HasPrefix(actualTrimmed, `<pre tabindex="0" class="chroma">`))
	assert.True(t, strings.HasSuffix(actualTrimmed, `</pre>`))
}

func TestConvertMarkdownToHTML(t *testing.T) {
	source := `# Heading

Text`
	expected := "<h1>Heading</h1><p>Text</p>"
	actual := convertToGoString(convertMarkdownToHTML(convertToCString(source)))
	assert.Equal(t, expected, minifyHTML(actual))
}

func TestConvertMarkdownToHTMLWithFrontMatter(t *testing.T) {
	source := `---
key: Value
key2: Another value
---

# Heading

Text`
	actual := convertToGoString(convertMarkdownToHTML(convertToCString(source)))
	assert.True(t, strings.Contains(actual, `<pre tabindex="0" class="chroma">`))
	assert.True(t, strings.Contains(minifyHTML(actual), `<h1>Heading</h1><p>Text</p>`))
}

func TestConvertMarkdownToHTMLWithSyntaxHighlighting(t *testing.T) {
	source := "# Heading\n\nText\n\n```js\nconst print = (text) => console.log(text);\nprint(\"Hello world\");\n```" // nolint:lll
	actual := convertToGoString(convertMarkdownToHTML(convertToCString(source)))
	assert.True(t, strings.Contains(actual, `<pre tabindex="0" class="chroma">`))
}

func TestConvertNotebookToHTML(t *testing.T) {
	source := `{"cells":[{"cell_type":"code","execution_count":1,"metadata":{},"outputs":[{"name":"stdout","output_type":"stream","text":["Hello world\n"]}],"source":["print(\"Hello world\")"]}],"metadata":{"kernelspec":{"display_name":"Python 3","language":"python","name":"python3"},"language_info":{"codemirror_mode":{"name":"ipython","version":3},"file_extension":".py","mimetype":"text/x-python","name":"python","nbconvert_exporter":"python","pygments_lexer":"ipython3","version":"3.8.2"}},"nbformat":4,"nbformat_minor":4}` // nolint:lll
	actual := convertToGoString(convertNotebookToHTML(convertToCString(source)))
	actualTrimmed := strings.TrimSpace(actual)
	assert.True(t, strings.HasPrefix(actualTrimmed, `<div class="notebook">`))
	assert.True(t, strings.HasSuffix(actualTrimmed, `</div>`))
}

func TestConvertNotebookToHTMLInvalid(t *testing.T) {
	source := "This is not a valid JSON file."
	actual := convertToGoString(convertNotebookToHTML(convertToCString(source)))
	assert.True(t, strings.HasPrefix(actual, "error: "))
}

func TestConvertPlutoNotebookToHTML(t *testing.T) {
	source := "### A Pluto.jl notebook ###\n# v0.20.24\n\n" +
		"# ╔═╡ aaaaaaaa-0000-0000-0000-000000000000\nx = 1 + 1\n\n" +
		"# ╔═╡ Cell order:\n# ╠═aaaaaaaa-0000-0000-0000-000000000000\n"
	actual := convertToGoString(
		convertPlutoNotebookToHTML(convertToCString(source), convertToCString("")),
	)
	assert.True(t, strings.HasPrefix(actual, `<pluto-editor class="disable_ui">`))
	assert.True(t, strings.HasSuffix(actual, `</pluto-editor>`))
	assert.True(t, strings.Contains(actual, `<pre tabindex="0" class="chroma">`))
}

func TestConvertPlutoNotebookToHTMLWithCache(t *testing.T) {
	source := "### A Pluto.jl notebook ###\n# v0.20.24\n\n" +
		"# ╔═╡ aaaaaaaa-0000-0000-0000-000000000000\nx = 1\n\n" +
		"# ╔═╡ Cell order:\n# ╠═aaaaaaaa-0000-0000-0000-000000000000\n"
	cache := "format = 1\n\n[cells.aaaaaaaa-0000-0000-0000-000000000000]\n" +
		"errored = false\nmime = \"text/plain\"\ntext_representation = \"2\"\n"
	actual := convertToGoString(
		convertPlutoNotebookToHTML(convertToCString(source), convertToCString(cache)),
	)
	assert.True(t, strings.Contains(actual, "<code>2</code>"))
}

// A plain Julia script must be rejected, so that the Swift side can fall back to the source-code
// preview rather than rendering an empty notebook.
func TestConvertPlutoNotebookToHTMLInvalid(t *testing.T) {
	source := "x = 1\n"
	actual := convertToGoString(
		convertPlutoNotebookToHTML(convertToCString(source), convertToCString("")),
	)
	assert.True(t, strings.HasPrefix(actual, "error: "))
}
