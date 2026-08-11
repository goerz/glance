package plutotohtml

import (
	"encoding/base64"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// maxRenderDepth bounds recursion through nested mime pairs.
const maxRenderDepth = 16

// maxImageBytes caps a single embedded image. Data URIs are base64, so the document grows by about
// a third more than this.
const maxImageBytes = 8 << 20

// renderOutputBody renders one cell output, dispatching on mime exactly as
// frontend/components/CellOutput.js does. nested suppresses the wrapper `<div>` that Pluto adds
// for a cell's own output but not for values inside a tree or table.
//
//nolint:cyclop // a mime dispatch table; splitting it would only scatter the mapping
func renderOutputBody(body any, mime string, depth int, nested bool) string {
	if depth > maxRenderDepth {
		return ""
	}

	wrap := func(s string) string {
		if nested || s == "" {
			return s
		}

		return "<div>" + s + "</div>"
	}

	switch mime {
	case mimePNG, mimeJPG, mimeJPEG, mimeGIF, mimeBMP, mimeSVG:
		return wrap(renderImage(body, mime))

	case mimeHTML:
		return renderRawHTML(body)

	case mimeTree:
		return wrap(renderTree(body, depth))

	case mimeTable:
		return renderTable(body, depth)

	case mimeStacktrace, mimeParseError:
		return wrap(renderStacktrace(body))

	case mimeReactDOM:
		return renderReactDOMElement(body, depth)

	case mimeDivElement:
		// Older statefiles still carry this; Pluto maps it onto a plain <div>.
		return renderDivElement(body, depth)

	case mimePlain:
		text := scalarText(body)
		if text == "" {
			return ""
		}

		return wrap(preformatted(text))

	case "":
		return ""
	}

	return ""
}

func renderImage(body any, mime string) string {
	data, ok := asBytes(body)
	if !ok {
		// An SVG can also arrive as text rather than bytes.
		if s, isString := body.(string); isString {
			data, ok = []byte(s), true
		}
	}

	if !ok || len(data) == 0 {
		return ""
	}

	if len(data) > maxImageBytes {
		return "<pre class=\"no-block\"><code>[image omitted: " + mime + ", " +
			itoa(len(data)) + " bytes]</code></pre>"
	}

	return "<img src=\"data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data) + "\">"
}

func renderDivElement(body any, depth int) string {
	node, ok := body.(map[string]any)
	if !ok {
		return ""
	}

	attributes := map[string]any{}
	if style, ok := node["style"].(string); ok {
		attributes["style"] = style
	}

	if class, ok := node["classname"].(string); ok {
		attributes["class"] = class
	}

	children, _ := node["children"].([]any)

	return renderReactDOMElement(map[string]any{
		keyTag:       tagDiv,
		"attributes": attributes,
		"children":   children,
	}, depth)
}

// renderRawHTML emits a cell's text/html output. Pluto wraps this in a RawHTMLContainer, whose
// `div.markdown` styling is what makes a Markdown cell look right.
func renderRawHTML(body any) string {
	source, ok := body.(string)
	if !ok || source == "" {
		return ""
	}
	// Pluto gives full pages their own iframe. There is no iframe here and no scripts to run in
	// one, so say so rather than splicing a whole document into the middle of the preview.
	trimmed := strings.TrimSpace(source)
	if strings.HasPrefix(trimmed, "<!DOCTYPE") || strings.HasPrefix(trimmed, "<html") {
		return "<div class=\"pluto-preview-note\">[full-page HTML output, not shown]</div>"
	}

	return "<div class=\"raw-html-wrapper\">" + sanitizeHTML(source) + "</div>"
}

// Elements dropped wholesale. Nothing here can work in a Quick Look preview: there is no JS
// runtime, no network, and no Pluto server to talk to. Leaving a <script> in place would at best
// do nothing and at worst throw on load.
var droppedElements = map[atom.Atom]bool{
	atom.Script: true,
	atom.Iframe: true,
	atom.Object: true,
	atom.Embed:  true,
	atom.Base:   true,
	atom.Meta:   true,
	atom.Link:   true,
}

// sanitizeHTML removes script-bearing constructs from a Pluto HTML output while leaving everything
// else — custom elements, classes, inline styles — untouched. Parsing rather than pattern-matching
// keeps it honest: a `<script>` written as `<SCRIPT >` or split across lines is still a script node
// to the parser.
func sanitizeHTML(source string) string {
	context := &nethtml.Node{Type: nethtml.ElementNode, Data: "div", DataAtom: atom.Div}

	nodes, err := nethtml.ParseFragment(strings.NewReader(source), context)
	if err != nil {
		// Fall back to showing the markup as text rather than injecting something unparsed.
		return "<pre class=\"no-block\"><code>" + html.EscapeString(source) + "</code></pre>"
	}

	var b strings.Builder

	for _, n := range nodes {
		// The fragment's own top-level nodes need the same check as their descendants: a cell
		// whose entire output is a <script> (PlutoUI's TableOfContents, for one) has it right
		// here at the root.
		if n.Type == nethtml.ElementNode && droppedElements[n.DataAtom] {
			continue
		}

		sanitizeNode(n)

		err := nethtml.Render(&b, n)
		if err != nil {
			return "<pre class=\"no-block\"><code>" + html.EscapeString(source) + "</code></pre>"
		}
	}

	return b.String()
}

// sanitizeNode strips dangerous children and attributes in place.
func sanitizeNode(n *nethtml.Node) {
	var next *nethtml.Node
	for child := n.FirstChild; child != nil; child = next {
		next = child.NextSibling
		if child.Type == nethtml.ElementNode && droppedElements[child.DataAtom] {
			n.RemoveChild(child)
			continue
		}

		sanitizeNode(child)
	}

	if n.Type == nethtml.ElementNode {
		n.Attr = safeAttributes(n.Attr)
	}
}

// preformatted renders plain text the way Pluto's ANSITextOutput does.
func preformatted(text string) string {
	return "<pre class=\"no-block\"><code>" +
		html.EscapeString(stripANSI(text)) + "</code></pre>"
}

func isURLAttribute(key string) bool {
	return key == "href" || key == "src" || key == "xlink:href"
}

// safeAttributes drops event handlers and javascript: URLs, leaving everything else alone.
func safeAttributes(in []nethtml.Attribute) []nethtml.Attribute {
	attrs := in[:0]
	for _, a := range in {
		key := strings.ToLower(a.Key)
		if strings.HasPrefix(key, "on") {
			continue // event handlers
		}

		if isURLAttribute(key) &&
			strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.Val)), "javascript:") {
			continue
		}

		attrs = append(attrs, a)
	}

	return attrs
}

// ansiPattern matches SGR escape sequences. Pluto colorizes these in the browser with ansi_up;
// here they are removed so they do not show up as literal `[0m` noise.
var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}

	return ansiPattern.ReplaceAllString(s, "")
}

// scalarText renders a decoded MsgPack scalar as the text Julia would have shown.
func scalarText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}

		return "false"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case []byte:
		return string(x)
	}

	return fmt.Sprint(v)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
