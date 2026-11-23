package docx

import "github.com/4nonX/interlinea/internal/xmlraw"

// The filter never re-encodes XML it did not change: parts are read with
// xmlraw, which keeps the original bytes of every node.
type (
	node  = xmlraw.Node
	span  = xmlraw.Span
	token = xmlraw.Token
)

var (
	tokenize      = xmlraw.Tokenize
	build         = xmlraw.Build
	parseFragment = xmlraw.ParseFragment
	escapeText    = xmlraw.EscapeText
	escapeAttr    = xmlraw.EscapeAttr
)
