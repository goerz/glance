package plutotohtml

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/BurntSushi/toml"
)

// cacheFormat is the only sidecar layout this renderer understands. SpaceStation itself ignores a
// cache whose format does not match (OutputCache.jl), and so do we — falling back to a code-only
// render beats displaying outputs we may be misreading.
const cacheFormat = 1

var errUnsupportedCacheFormat = errors.New("unsupported output cache format")

// CachedCell is one cell's entry in the output-cache sidecar.
type CachedCell struct {
	Errored            bool   `toml:"errored"`
	MIME               string `toml:"mime"`
	TextRepresentation string `toml:"text_representation"`
	RuntimeNS          int64  `toml:"runtime_ns"`
	OutputPacked       string `toml:"output_packed"`
}

// Cache is a parsed `<notebook>.jl.pluto-cache.toml`.
type Cache struct {
	Format       int                   `toml:"format"`
	PlutoVersion string                `toml:"pluto_version"`
	JuliaVersion string                `toml:"julia_version"`
	CellOrder    []string              `toml:"cell_order"`
	Cells        map[string]CachedCell `toml:"cells"`
}

// Output is a cell's decoded result.
type Output struct {
	Body         any // string, []byte, map[string]any, or nil
	MIME         string
	RootAssignee string
	Errored      bool
	// Text is the sidecar's plain-text digest, used whenever Body cannot be rendered.
	Text string
}

// ParseCache parses the sidecar. An empty string yields an empty cache rather than an error — most
// notebooks on disk have never been run, and one that has not is still worth previewing.
func ParseCache(source string) (*Cache, error) {
	if source == "" {
		return &Cache{Format: cacheFormat}, nil
	}

	var c Cache

	_, err := toml.Decode(source, &c)
	if err != nil {
		return nil, fmt.Errorf("could not parse output cache: %w", err)
	}

	if c.Format != cacheFormat {
		return nil, fmt.Errorf("%w: %d", errUnsupportedCacheFormat, c.Format)
	}

	return &c, nil
}

// Output returns the decoded output for a cell, or nil when the cell has never been run.
func (c *Cache) Output(cellID string) *Output {
	if c == nil {
		return nil
	}

	entry, ok := c.Cells[cellID]
	if !ok {
		return nil
	}

	out := &Output{
		MIME:    entry.MIME,
		Errored: entry.Errored,
		Text:    entry.TextRepresentation,
	}

	// output_packed is optional; when packing failed Julia-side the text digest is all there is.
	packed := unpackOutput(entry.OutputPacked)
	if packed == nil {
		return out
	}

	out.Body = packed["body"]

	if mime, ok := packed["mime"].(string); ok && mime != "" {
		out.MIME = mime
	}

	if assignee, ok := packed["rootassignee"].(string); ok {
		out.RootAssignee = assignee
	}

	return out
}

// unpackOutput decodes the base64+MsgPack `output_packed` blob and returns its "output" table.
// Every failure yields nil: a cell whose output cannot be decoded still renders from its text
// digest, which beats failing the whole notebook.
func unpackOutput(encoded string) map[string]any {
	if encoded == "" {
		return nil
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}

	decoded, err := decodeMsgpack(raw)
	if err != nil {
		return nil
	}

	top, ok := decoded.(map[string]any)
	if !ok {
		return nil
	}

	packed, ok := top["output"].(map[string]any)
	if !ok {
		return nil
	}

	return packed
}
