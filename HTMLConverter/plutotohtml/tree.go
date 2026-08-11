package plutotohtml

import (
	"html"
	"strings"
)

// Rich Pluto values arrive as nested objects rather than markup. The functions here rebuild the
// exact element names Pluto's frontend emits (frontend/components/TreeView.js), because all of the
// visual work — brackets, commas, `=>` separators, the disclosure caret — is done by CSS
// ::before/::after rules keyed to those names in frontend/treeview.css. Get the names right and the
// styling comes for free; invent your own and nothing lines up.

// moreMarker is spliced into elements/rows in place of a value when Pluto truncated the output. It
// can appear in the middle of an array, not just at the end.
const moreMarker = "more"

// mimePairLen is the length of the `[body, mime]` arrays Pluto nests inside trees and tables.
const mimePairLen = 2

// tableIndexColumns is the slack Pluto leaves in a table's colspan beyond the schema columns.
const tableIndexColumns = 3

// renderTree renders application/vnd.pluto.tree+object.
func renderTree(body any, depth int) string {
	node, ok := body.(map[string]any)
	if !ok {
		return ""
	}

	treeType, _ := node[keyType].(string)

	if treeType == "circular" {
		return "<em>circular reference</em>"
	}

	if treeType == "Pair" {
		pair, _ := node["key_value"].([]any)
		if len(pair) != mimePairLen {
			return ""
		}

		return "<pluto-tree-pair class=\"Pair\"><p-r><p-k>" +
			renderMIMEPair(pair[0], depth+1) + "</p-k><p-v>" +
			renderMIMEPair(pair[1], depth+1) + "</p-v></p-r></pluto-tree-pair>"
	}

	elements, _ := node["elements"].([]any)
	items := renderTreeItems(elements, treeType, depth)

	prefix, _ := node["prefix"].(string)
	prefixShort, _ := node["prefix_short"].(string)

	// A Tuple has no type prefix, but the element still has to be there for the CSS that draws
	// the opening bracket off ::after.
	if treeType == "Tuple" {
		prefix, prefixShort = "", ""
	}

	prefixHTML := "<pluto-tree-prefix><span class=\"long\">" +
		html.EscapeString(prefix) + "</span><span class=\"short\">" +
		html.EscapeString(prefixShort) + "</span></pluto-tree-prefix>"

	// `collapsed` is the state Pluto itself opens in, and it keeps a preview compact.
	return "<pluto-tree class=\"collapsed " + html.EscapeString(treeType) + "\">" +
		prefixHTML +
		"<pluto-tree-items class=\"" + html.EscapeString(treeType) + "\">" +
		items +
		"</pluto-tree-items></pluto-tree>"
}

// renderTreeItems renders the `<p-r>` rows of a tree. How the key is shown depends on the
// container: sets hide it, dicts render it as a nested value, and everything else prints it.
func renderTreeItems(elements []any, treeType string, depth int) string {
	var items strings.Builder

	for _, element := range elements {
		if s, ok := element.(string); ok && s == moreMarker {
			items.WriteString("<p-r>" + moreElement() + "</p-r>")
			continue
		}

		row, ok := element.([]any)
		if !ok || len(row) != mimePairLen {
			continue
		}

		items.WriteString("<p-r>")

		switch treeType {
		case "Set":
			// Pluto hides the index for sets entirely.
		case "Dict":
			items.WriteString("<p-k>" + renderMIMEPair(row[0], depth+1) + "</p-k>")
		default:
			// Array and Tuple carry an integer index; NamedTuple and struct a field name.
			items.WriteString("<p-k>" + html.EscapeString(scalarText(row[0])) + "</p-k>")
		}

		items.WriteString("<p-v>" + renderMIMEPair(row[1], depth+1) + "</p-v>")
		items.WriteString("</p-r>")
	}

	return items.String()
}

// renderTable renders application/vnd.pluto.table+object.
func renderTable(body any, depth int) string {
	node, ok := body.(map[string]any)
	if !ok {
		return ""
	}

	var names, types []any
	if schema, ok := node["schema"].(map[string]any); ok {
		names, _ = schema["names"].([]any)
		types, _ = schema["types"].([]any)
	}

	maxColspan := tableIndexColumns + len(names)
	rows, _ := node["rows"].([]any)

	return "<table class=\"pluto-table\">" +
		renderTableHead(names, types, maxColspan) +
		renderTableBody(rows, maxColspan, depth) +
		"</table>"
}

// renderTableHead renders the two header rows Pluto shows, column names above column types.
func renderTableHead(names, types []any, maxColspan int) string {
	if len(names) == 0 {
		return "<thead><tr class=\"empty\"><td colspan=\"" + itoa(maxColspan) +
			"\"><div>⌀ <small>no columns</small></div></td></tr></thead>"
	}

	var b strings.Builder

	b.WriteString("<thead><tr class=\"schema-names\"><th></th>")

	for _, n := range names {
		if isMore(n) {
			b.WriteString("<th>" + moreElement() + "</th>")
			continue
		}

		b.WriteString("<th>" + html.EscapeString(scalarText(n)) + "</th>")
	}

	b.WriteString("</tr><tr class=\"schema-types\"><th></th>")

	for _, t := range types {
		if isMore(t) {
			// Pluto leaves the type cell blank under a truncated column.
			b.WriteString("<th></th>")
			continue
		}

		b.WriteString("<th>" + html.EscapeString(scalarText(t)) + "</th>")
	}

	b.WriteString("</tr></thead>")

	return b.String()
}

// renderTableBody renders the data rows. A row is `[index, cells]`, or the literal "more" when
// Pluto truncated the row set.
func renderTableBody(rows []any, maxColspan int, depth int) string {
	var b strings.Builder

	b.WriteString("<tbody>")

	if len(rows) == 0 {
		b.WriteString("<tr class=\"empty\"><td colspan=\"" + itoa(maxColspan) +
			"\"><div><div>⌀</div><small>no rows</small></div></td></tr>")
	}

	for _, r := range rows {
		if isMore(r) {
			b.WriteString("<tr><td class=\"pluto-tree-more-td\" colspan=\"" + itoa(maxColspan) +
				"\">" + moreElement() + "</td></tr>")

			continue
		}

		row, ok := r.([]any)
		if !ok || len(row) != mimePairLen {
			continue
		}

		b.WriteString("<tr><th>" + html.EscapeString(scalarText(row[0])) + "</th>")

		cells, _ := row[1].([]any)
		for _, cell := range cells {
			if isMore(cell) {
				b.WriteString("<td><div></div></td>")
				continue
			}

			b.WriteString("<td><div>" + renderMIMEPair(cell, depth+1) + "</div></td>")
		}

		b.WriteString("</tr>")
	}

	b.WriteString("</tbody>")

	return b.String()
}

// isMore reports whether a decoded element is Pluto's truncation marker.
func isMore(v any) bool {
	s, ok := v.(string)

	return ok && s == moreMarker
}

// renderStacktrace renders application/vnd.pluto.stacktrace+object and parseerror+object.
//
// Pluto's own ErrorMessage.js is 673 lines of message-rewriting heuristics and stack-frame ranking.
// None of that is reproduced here: `plain_error` already contains the full uncolored `showerror`
// output including the backtrace, which is the faithful thing to show in a preview.
func renderStacktrace(body any) string {
	node, ok := body.(map[string]any)
	if !ok {
		return ""
	}

	msg, _ := node["msg"].(string)
	plain, _ := node["plain_error"].(string)

	var b strings.Builder
	b.WriteString("<jlerror>")

	if msg != "" {
		b.WriteString("<header><p>" + html.EscapeString(msg) + "</p></header>")
	}

	if plain != "" && plain != msg {
		b.WriteString("<section><pre>" + html.EscapeString(plain) + "</pre></section>")
	}

	b.WriteString("</jlerror>")

	return b.String()
}

// renderReactDOMElement renders application/vnd.pluto.reactdomelement+object.
func renderReactDOMElement(body any, depth int) string {
	node, ok := body.(map[string]any)
	if !ok {
		return ""
	}

	tag, _ := node[keyTag].(string)
	if !isSafeTagName(tag) {
		return ""
	}

	var attrs strings.Builder

	if attributes, ok := node["attributes"].(map[string]any); ok {
		for name, value := range attributes {
			if !isSafeAttributeName(name) {
				continue
			}

			attrs.WriteString(" " + name + "=\"" + html.EscapeString(scalarText(value)) + "\"")
		}
	}

	var children strings.Builder

	if list, ok := node["children"].([]any); ok {
		for _, child := range list {
			if s, ok := child.(string); ok {
				children.WriteString(html.EscapeString(s))
				continue
			}

			children.WriteString(renderReactDOMElement(child, depth+1))
		}
	}

	return "<" + tag + attrs.String() + ">" + children.String() + "</" + tag + ">"
}

// renderMIMEPair renders a `[body, mime]` pair, the recursive unit inside trees and tables.
func renderMIMEPair(v any, depth int) string {
	pair, ok := v.([]any)
	if !ok || len(pair) != mimePairLen {
		return html.EscapeString(scalarText(v))
	}

	mime, _ := pair[1].(string)

	return renderOutputBody(pair[0], mime, depth, true)
}

// moreElement stands in for output Pluto truncated. It is always disabled: expanding it would mean
// asking a running Julia process for the rest, and there isn't one.
func moreElement() string {
	return "<pluto-tree-more class=\"disabled\" " +
		"title=\"Truncated by Pluto; open the notebook to see more\"></pluto-tree-more>"
}

// maxNameLen bounds a tag or attribute name decoded from an output object.
const maxNameLen = 64

// unsafeTags can never be emitted from decoded output: each one would either execute code or pull
// in a remote resource, neither of which belongs in a sandboxed preview.
var unsafeTags = map[string]bool{
	"script": true, "iframe": true, "object": true,
	"embed": true, "link": true, "meta": true, "base": true,
}

func isAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// isPlainName reports whether every byte is alphanumeric or one of extra.
func isPlainName(name string, extra string) bool {
	if name == "" || len(name) > maxNameLen {
		return false
	}

	for i := range len(name) {
		if !isAlphanumeric(name[i]) && !strings.ContainsRune(extra, rune(name[i])) {
			return false
		}
	}

	return true
}

func isSafeTagName(tag string) bool {
	return isPlainName(tag, "-") && !unsafeTags[strings.ToLower(tag)]
}

func isSafeAttributeName(name string) bool {
	// No event handlers.
	return isPlainName(name, "-_") && !strings.HasPrefix(strings.ToLower(name), "on")
}
