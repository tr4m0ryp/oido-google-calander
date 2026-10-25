package views

import (
	"fmt"
	"html/template"
)

// Inline SVG icons on a 24px grid with a 1.75 stroke, matching the Gmail
// admin set. Decorative, so hidden from assistive technology.
var iconPaths = map[string]string{
	"calendar": `<rect x="3.5" y="5" width="17" height="15" rx="2"/><path d="M3.5 10h17M8 3v4M16 3v4"/>`,
	"plus":     `<path d="M12 5v14M5 12h14"/>`,
	"copy": `<rect x="9" y="9" width="11" height="11" rx="2"/>` +
		`<path d="M5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1"/>`,
	"trash":   `<path d="M4 7h16M10 11v6M14 11v6M6 7l1 12a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-12M9 7V4h6v3"/>`,
	"alert":   `<circle cx="12" cy="12" r="9"/><path d="M12 7.5v5.5M12 16.5h.01"/>`,
	"success": `<circle cx="12" cy="12" r="9"/><path d="m8 12.5 2.8 2.8L16 9.5"/>`,
	"logout":  `<path d="M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4M10 16l-4-4 4-4M6 12h10"/>`,
	"refresh": `<path d="M20 11a8 8 0 0 0-14.3-4.9L4 8M4 4v4h4M4 13a8 8 0 0 0 14.3 4.9L20 16M20 20v-4h-4"/>`,
}

// icon renders a named icon; names are compile-time constants in templates.
func icon(name string, class ...string) (template.HTML, error) {
	path, ok := iconPaths[name]
	if !ok {
		return "", fmt.Errorf("unknown icon %q", name)
	}
	cls := ""
	if len(class) > 0 && class[0] != "" {
		cls = ` class="` + template.HTMLEscapeString(class[0]) + `"`
	}
	return template.HTML(`<svg` + cls + ` viewBox="0 0 24 24" fill="none" stroke="currentColor" ` +
		`stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" ` +
		`focusable="false">` + path + `</svg>`), nil
}
