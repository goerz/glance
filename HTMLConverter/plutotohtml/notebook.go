package plutotohtml

import (
	"strings"

	"github.com/BurntSushi/toml"
)

// Delimiters of the Pluto file format, mirroring
// SpaceStation.jl/src/notebook/saving and loading.jl.
const (
	notebookHeader       = "### A Pluto.jl notebook ###"
	metadataPrefix       = "#> "
	cellIDDelimiter      = "# ╔═╡ "
	cellMetadataPrefix   = "# ╠═╡ "
	cellOrderMarker      = "Cell order:"
	orderDelimiter       = "# ╠═"
	orderDelimiterFolded = "# ╟─"
	disabledPrefix       = "#=╠═╡"
	disabledSuffix       = "╠═╡ =#"
	projectTOMLCellID    = "00000000-0000-0000-0000-000000000001"
	manifestTOMLCellID   = "00000000-0000-0000-0000-000000000002"
)

// Cell is one notebook cell in display order.
type Cell struct {
	ID       string
	Code     string
	Folded   bool
	Disabled bool
}

// Notebook is the parsed contents of a `.jl` notebook file.
type Notebook struct {
	PlutoVersion string
	Frontmatter  map[string]any
	Cells        []Cell
}

// IsNotebook reports whether source is a Pluto notebook, i.e. whether its very first line is the
// Pluto header. A mention of the header anywhere later in the file does not count.
func IsNotebook(source string) bool {
	line := source
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}

	return strings.TrimRight(line, "\r") == notebookHeader
}

// ParseNotebook parses a `.jl` notebook into cells in display order.
func ParseNotebook(source string) (*Notebook, error) {
	nb := &Notebook{}
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")

	// Line 2 carries the Pluto version as `# v0.20.24`.
	if len(lines) > 1 && strings.HasPrefix(lines[1], "# v") {
		nb.PlutoVersion = strings.TrimSpace(strings.TrimPrefix(lines[1], "# "))
	}

	nb.Frontmatter = parseFrontmatter(lines)

	code, definitionOrder, order := scanCells(lines)
	nb.Cells = assembleCells(code, definitionOrder, order)

	return nb, nil
}

// assembleCells joins the scanned code to the display order, dropping the package-environment
// pseudo-cells and orphaned Cell order entries.
func assembleCells(code map[string]string, definitionOrder []string, order []Cell) []Cell {
	// The Cell order block is authoritative for display order; fall back to definition order
	// only when the block is missing entirely (a hand-written or truncated notebook).
	if len(order) == 0 {
		for _, id := range definitionOrder {
			order = append(order, Cell{ID: id})
		}
	}

	var cells []Cell

	seen := map[string]bool{}

	for _, cell := range order {
		src, ok := code[cell.ID]
		if !ok || isEnvironmentCell(cell.ID) {
			continue
		}

		seen[cell.ID] = true
		cell.Code, cell.Disabled = undisable(src)
		cells = append(cells, cell)
	}

	// A cell defined but absent from the Cell order block would otherwise vanish.
	for _, id := range definitionOrder {
		if seen[id] || isEnvironmentCell(id) {
			continue
		}

		c := Cell{ID: id}
		c.Code, c.Disabled = undisable(code[id])
		cells = append(cells, c)
	}

	return cells
}

// isEnvironmentCell reports whether an id is one of the two pseudo-cells holding the notebook's
// Project.toml and Manifest.toml. Pluto never displays them.
func isEnvironmentCell(id string) bool {
	return id == projectTOMLCellID || id == manifestTOMLCellID
}

// scanCells walks the file once, collecting each cell's code, the order in which the cells were
// defined, and the Cell order block that determines how they are displayed.
func scanCells(lines []string) (code map[string]string, definitionOrder []string, order []Cell) {
	code = map[string]string{}

	var (
		currentID    string
		body         []string
		inOrderBlock bool
	)

	flush := func() {
		if currentID == "" {
			return
		}

		if _, seen := code[currentID]; !seen {
			definitionOrder = append(definitionOrder, currentID)
		}

		code[currentID] = cleanCellCode(body)
		currentID, body = "", nil
	}

	for _, line := range lines {
		if after, ok := strings.CutPrefix(line, cellIDDelimiter); ok {
			flush()

			rest := strings.TrimSpace(after)
			inOrderBlock = rest == cellOrderMarker

			if !inOrderBlock {
				currentID = rest
			}

			continue
		}

		switch {
		case inOrderBlock:
			if cell, ok := parseOrderLine(line); ok {
				order = append(order, cell)
			}
		case currentID != "":
			body = append(body, line)
		}
	}

	flush()

	return code, definitionOrder, order
}

// parseOrderLine reads one line of the Cell order block. The prefix carries the fold state:
// `# ╠═` shows the code, `# ╟─` hides it.
func parseOrderLine(line string) (Cell, bool) {
	if after, ok := strings.CutPrefix(line, orderDelimiterFolded); ok {
		return Cell{ID: strings.TrimSpace(after), Folded: true}, true
	}

	if after, ok := strings.CutPrefix(line, orderDelimiter); ok {
		return Cell{ID: strings.TrimSpace(after), Folded: false}, true
	}

	return Cell{}, false
}

// parseFrontmatter reads the contiguous run of `#> ` lines below the version comment and returns
// the `frontmatter` table from the TOML they encode.
func parseFrontmatter(lines []string) map[string]any {
	var collected []string

	for _, line := range lines {
		if strings.HasPrefix(line, cellIDDelimiter) {
			break // frontmatter always precedes the first cell
		}

		if after, ok := strings.CutPrefix(line, metadataPrefix); ok {
			collected = append(collected, after)
		} else if line == strings.TrimSpace(metadataPrefix) {
			collected = append(collected, "")
		}
	}

	if len(collected) == 0 {
		return nil
	}

	var metadata map[string]any

	_, err := toml.Decode(strings.Join(collected, "\n"), &metadata)
	if err != nil {
		return nil
	}

	fm, _ := metadata["frontmatter"].(map[string]any)

	return fm
}

// cleanCellCode strips the cell's metadata lines and the blank lines Pluto writes between cells.
func cleanCellCode(body []string) string {
	i := 0
	for i < len(body) && strings.HasPrefix(body[i], cellMetadataPrefix) {
		i++ // `# ╠═╡ disabled = true` and friends
	}

	return strings.Trim(strings.Join(body[i:], "\n"), "\n")
}

// undisable unwraps the `#=╠═╡ … ╠═╡ =#` comment that Pluto puts around a disabled cell's code,
// reporting whether the cell was disabled.
func undisable(code string) (string, bool) {
	trimmed := strings.TrimSpace(code)
	if !strings.HasPrefix(trimmed, disabledPrefix) || !strings.HasSuffix(trimmed, disabledSuffix) {
		return code, false
	}

	inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, disabledPrefix), disabledSuffix)
	// Pluto writes the closing marker as "\n  ╠═╡ =#", so the indentation in front of it is left
	// behind once the marker itself is removed.
	return strings.TrimRight(strings.TrimLeft(inner, "\n"), " \t\n"), true
}

// prose reports whether a cell is a Markdown or HTML string macro, and returns its body. Pluto's
// own answer (`is_just_text` in src/analysis/is_just_text.jl) needs a full Julia parse and the
// reactivity graph, so this is a syntactic approximation — good enough for the only case that
// needs it, a folded cell with no cached output, which would otherwise render blank.
func prose(code string) (body string, lang string, ok bool) {
	trimmed := strings.TrimSpace(code)

	for _, m := range []struct{ prefix, lang string }{
		{`md"""`, langMarkdown},
		{`html"""`, langHTML},
		{`md"`, langMarkdown},
		{`html"`, langHTML},
	} {
		quote := `"""`
		if !strings.HasSuffix(m.prefix, `"""`) {
			quote = `"`
		}

		if strings.HasPrefix(trimmed, m.prefix) && strings.HasSuffix(trimmed, quote) {
			inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, m.prefix), quote)
			if len(strings.TrimPrefix(trimmed, m.prefix)) < len(quote) {
				continue // the opening delimiter is also the closing one
			}

			return strings.Trim(inner, "\n"), m.lang, true
		}
	}

	return "", "", false
}
