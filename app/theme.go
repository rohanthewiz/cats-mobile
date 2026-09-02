package catsapp

import (
	"regexp"
	"strings"

	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/core"
)

// The app is dark by default: it renders terminals, and a terminal on a
// white card is a terminal nobody at the desk is looking at. The palette
// below is the fallback; the server's own theme replaces it the moment the
// `theme` message lands, so the phone wears the same colours the browser
// does for the same session.
//
// cats sends its palette as CSS custom-property names without the "--"
// (internal/theme): bg, fg, muted, line, accent, ok, warn, err. Those map
// onto grmob's semantic roles one for one; anything the server did not send,
// or sent as something that is not a hex colour, keeps the fallback, so the
// app is never painted with a garbage value.
const (
	darkBackground = "#14161c"
	darkSurface    = "#1e2129"
	darkText       = "#e6e6e6"
	darkMuted      = "#8b93a5"
	darkBorder     = "#2c3140"
	darkAccent     = "#7aa2f7"
	darkOK         = "#9ece6a"
	darkWarn       = "#e0af68"
	darkErr        = "#f7768e"
)

// themeFor builds the grmob theme from the server's palette (nil before it
// arrives). A fresh Theme value every call: it is a handful of struct copies,
// and caching would need invalidating exactly when the theme has to change.
func themeFor(server *wire.Theme) *core.Theme {
	theme := *core.DefaultTheme
	theme.Colors = core.ColorPalette{
		Primary:       darkAccent,
		Secondary:     darkOK,
		Background:    darkBackground,
		Surface:       darkSurface,
		TextPrimary:   darkText,
		TextSecondary: darkMuted,
		Error:         darkErr,
		Border:        darkBorder,
		Success:       darkOK,
		Warning:       darkWarn,
	}
	if server != nil {
		pick := func(key string, into *string) {
			if hex := normalizedHex(server.Colors[key]); hex != "" {
				*into = hex
			}
		}
		pick("bg", &theme.Colors.Background)
		pick("fg", &theme.Colors.TextPrimary)
		pick("muted", &theme.Colors.TextSecondary)
		pick("line", &theme.Colors.Border)
		pick("accent", &theme.Colors.Primary)
		pick("ok", &theme.Colors.Success)
		pick("warn", &theme.Colors.Warning)
		pick("err", &theme.Colors.Error)
		// A surface a shade above the background, since the server has no
		// such role: cards and dialogs sit on it.
		theme.Colors.Surface = lighten(theme.Colors.Background)
	}
	// The button base and the typography styles are separate from the
	// palette and do not re-derive from it (grmob's components.Button
	// documents exactly this, and ListRow draws its title in
	// Typography.Body), so the ink has to be pushed into each. Without this
	// every list title stays DefaultTheme's black on the dark ground.
	theme.Components.Button.Background = theme.Colors.Primary
	theme.Components.Button.TextColor = darkBackground
	theme.Typography.Title.TextColor = theme.Colors.TextPrimary
	theme.Typography.Subtitle.TextColor = theme.Colors.TextPrimary
	theme.Typography.Body.TextColor = theme.Colors.TextPrimary
	theme.Typography.Caption.TextColor = theme.Colors.TextSecondary
	theme.Components.Text.TextColor = theme.Colors.TextPrimary
	theme.Components.Input.TextColor = theme.Colors.TextPrimary
	theme.Components.Input.Background = theme.Colors.Surface
	theme.Components.TextArea.TextColor = theme.Colors.TextPrimary
	theme.Components.TextArea.Background = theme.Colors.Surface
	theme.Components.Card.Background = theme.Colors.Surface
	return &theme
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// normalizedHex accepts "#rrggbb" in any case and returns it lowercased;
// anything else (a name, an rgb() function, a 3-digit short form) is "".
func normalizedHex(s string) string {
	s = strings.TrimSpace(s)
	if !hexColor.MatchString(s) {
		return ""
	}
	return strings.ToLower(s)
}

// lighten nudges a hex colour towards white by a fixed step, for deriving
// a surface from a background. Rough on purpose: the goal is "visibly a
// panel", not colour science.
func lighten(hex string) string {
	if !hexColor.MatchString(hex) {
		return darkSurface
	}
	var out strings.Builder
	out.WriteByte('#')
	for i := 1; i < 7; i += 2 {
		v := hexByte(hex[i])<<4 | hexByte(hex[i+1])
		v += 12
		if v > 255 {
			v = 255
		}
		out.WriteString(strings.ToLower(string(hexDigits[v>>4])) + strings.ToLower(string(hexDigits[v&0xF])))
	}
	return out.String()
}

const hexDigits = "0123456789abcdef"

func hexByte(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}
