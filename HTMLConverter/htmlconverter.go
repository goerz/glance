package main

import (
	"C"
	"bytes"
	"fmt"
	"regexp"

	"github.com/alecthomas/chroma"
	htmlFormatter "github.com/alecthomas/chroma/formatters/html"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/styles"
	"github.com/goerz/glance/plutotohtml"
	"github.com/samuelmeuli/nbtohtml"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting"
	"github.com/yuin/goldmark/extension"
)

// Regex for YAML front matter in a Markdown document
var markdownFrontMatterRegex = regexp.MustCompile(`^---\n([\s\S]*?)\n---\n`)

// Enable syntax highlighting in Markdown
var markdownParser = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		highlighting.NewHighlighting(
			highlighting.WithFormatOptions(
				htmlFormatter.WithClasses(true),
			),
		),
	),
)

// Functions for conversion between C and Go strings. Required here because cgo cannot be used in
// tests.

func convertToCString(goString string) *C.char {
	return C.CString(goString)
}

func convertToGoString(cString *C.char) string {
	return C.GoString(cString)
}

// Convention: Because all functions return C strings, errors are implemented as return values which
// start with "error: ".

// convertCodeToHTML converts the provided source code string to HTML. Classes for syntax
// highlighting are generated using Chroma.
//
//export convertCodeToHTML
func convertCodeToHTML(source *C.char, lexer *C.char) *C.char {
	sourceString := convertToGoString(source)
	lexerString := convertToGoString(lexer)
	htmlBuffer := new(bytes.Buffer)

	// Set up lexer for programming language
	var l chroma.Lexer
	if lexerString != "" {
		l = lexers.Get(lexerString)
	}
	if l == nil {
		l = lexers.Analyse(sourceString)
	}
	if l == nil {
		l = lexers.Fallback
	}
	l = chroma.Coalesce(l)

	// Use classes instead of inline styles
	formatter := htmlFormatter.New(htmlFormatter.WithClasses(true))

	iterator, err := l.Tokenise(nil, sourceString)
	if err != nil {
		errMessage := fmt.Sprintf("error: Could not render source code (tokenization error): %d", err)
		return convertToCString(errMessage)
	}

	err = formatter.Format(htmlBuffer, styles.GitHub, iterator)
	if err != nil {
		errMessage := fmt.Sprintf("error: Could not render source code (formatting error): %d", err)
		return convertToCString(errMessage)
	}

	// Chroma escapes tags, so HTML should be safe from code injection
	htmlString := htmlBuffer.String()
	return convertToCString(htmlString)
}

// convertMarkdownToHTML converts the provided Markdown string to HTML using goldmark. Classes for
// syntax highlighting inside code blocks are generated using Chroma.
//
//export convertMarkdownToHTML
func convertMarkdownToHTML(source *C.char) *C.char {
	sourceString := convertToGoString(source)

	// Render YAML front matter as code block
	sourceString = markdownFrontMatterRegex.ReplaceAllString(sourceString, "```yaml\n$1\n```\n")

	// Convert Markdown to HTML
	var htmlBuffer bytes.Buffer
	err := markdownParser.Convert([]byte(sourceString), &htmlBuffer)
	if err != nil {
		errMessage := fmt.Sprintf("error: Could not convert Markdown to HTML: %d", err)
		return convertToCString(errMessage)
	}
	// goldmark does not render raw HTML or potentially-dangerous URLs, so HTML should be safe from
	// code injection
	return convertToCString(htmlBuffer.String())
}

// convertPlutoNotebookToHTML converts a Pluto.jl notebook to HTML. `cache` is the contents of the
// notebook's `.pluto-cache.toml` sidecar, which holds the cell outputs, or an empty string when
// there is none — a notebook that has never been run still renders, just without its results.
//
//export convertPlutoNotebookToHTML
func convertPlutoNotebookToHTML(source *C.char, cache *C.char) *C.char {
	sourceString := convertToGoString(source)
	cacheString := convertToGoString(cache)

	htmlString, err := plutotohtml.Convert(sourceString, cacheString)
	if err != nil {
		errMessage := fmt.Sprintf("error: Could not convert Pluto notebook to HTML: %s", err)

		return convertToCString(errMessage)
	}

	return convertToCString(htmlString)
}

// convertNotebookToHTML converts the provided Jupyter Notebook JSON to HTML using `nbtohtml`.
//
//export convertNotebookToHTML
func convertNotebookToHTML(source *C.char) *C.char {
	sourceString := convertToGoString(source)

	html := new(bytes.Buffer)
	err := nbtohtml.ConvertString(html, sourceString)
	if err != nil {
		errMessage := fmt.Sprintf("error: Could not convert Notebook to HTML: %d", err)
		return convertToCString(errMessage)
	}
	htmlString := html.String()
	return convertToCString(htmlString)
}

// Main function is required for `c-archive` builds.
func main() {}
