package static

import "embed"

// FS contains local browser assets. KoalaBye never requires a CDN.
//
//go:embed app.css app.js htmx.min.js
var FS embed.FS
