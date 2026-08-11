package plutotohtml

// The MIME types Pluto can attach to a cell output, in the preference order of `allmimes` in
// SpaceStation.jl/src/runner/PlutoRunner/src/display/mime dance.jl. Note that `text/latex` never
// reaches the sidecar: Pluto converts it to `text/html` in-process.
const (
	mimePlain      = "text/plain"
	mimeHTML       = "text/html"
	mimeTree       = "application/vnd.pluto.tree+object"
	mimeTable      = "application/vnd.pluto.table+object"
	mimeStacktrace = "application/vnd.pluto.stacktrace+object"
	mimeParseError = "application/vnd.pluto.parseerror+object"
	mimeReactDOM   = "application/vnd.pluto.reactdomelement+object"
	mimeDivElement = "application/vnd.pluto.divelement+object"

	mimePNG  = "image/png"
	mimeJPG  = "image/jpg"
	mimeJPEG = "image/jpeg"
	mimeGIF  = "image/gif"
	mimeBMP  = "image/bmp"
	mimeSVG  = "image/svg+xml"
)

// Keys used inside Pluto's rich-output objects.
const (
	keyType = "type"
	keyTag  = "tag"
)

const (
	langMarkdown = "markdown"
	langHTML     = "html"
	tagDiv       = "div"
)
