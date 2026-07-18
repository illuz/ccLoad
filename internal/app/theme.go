package app

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"

	"ccLoad/internal/version"

	"github.com/gin-gonic/gin"
)

const (
	envThemeColor     = "CCLOAD_THEME_COLOR"
	envThemeColorDark = "CCLOAD_THEME_COLOR_DARK"
)

type uiColor struct {
	R uint8
	G uint8
	B uint8
}

type uiTheme struct {
	Primary50  uiColor
	Primary100 uiColor
	Primary200 uiColor
	Primary300 uiColor
	Primary400 uiColor
	Primary500 uiColor
	Primary600 uiColor
	Primary700 uiColor
	Primary800 uiColor
	Primary900 uiColor
	LogoEnd    uiColor
}

var namedThemeColors = map[string]string{
	"blue":    "#3b82f6",
	"default": "#3b82f6",
	"green":   "#10b981",
	"emerald": "#10b981",
	"red":     "#ef4444",
	"rose":    "#f43f5e",
	"orange":  "#f97316",
	"amber":   "#f59e0b",
	"purple":  "#8b5cf6",
	"violet":  "#8b5cf6",
	"cyan":    "#06b6d4",
	"teal":    "#14b8a6",
}

var hexColorPattern = regexp.MustCompile(`^#?[0-9a-fA-F]{6}$|^#?[0-9a-fA-F]{3}$`)

func defaultUITheme() uiTheme {
	primary := uiColor{R: 0x3b, G: 0x82, B: 0xf6}
	theme := deriveUITheme(primary)
	theme.Primary50 = uiColor{R: 0xf0, G: 0xf7, B: 0xff}
	theme.Primary100 = uiColor{R: 0xe0, G: 0xef, B: 0xff}
	theme.Primary200 = uiColor{R: 0xb8, G: 0xdc, B: 0xff}
	theme.Primary300 = uiColor{R: 0x7c, G: 0xc3, B: 0xff}
	theme.Primary400 = uiColor{R: 0x3c, G: 0xa3, B: 0xff}
	theme.Primary500 = primary
	theme.Primary600 = uiColor{R: 0x25, G: 0x63, B: 0xeb}
	theme.Primary700 = uiColor{R: 0x1d, G: 0x4e, B: 0xd8}
	theme.Primary800 = uiColor{R: 0x1e, G: 0x40, B: 0xaf}
	theme.Primary900 = uiColor{R: 0x1e, G: 0x3a, B: 0x8a}
	theme.LogoEnd = theme.Primary700
	return theme
}

func currentUITheme() uiTheme {
	primary, ok := parseThemeColor(os.Getenv(envThemeColor))
	if !ok {
		return defaultUITheme()
	}
	theme := deriveUITheme(primary)
	if logoEnd, ok := parseThemeColor(os.Getenv(envThemeColorDark)); ok {
		theme.LogoEnd = logoEnd
		theme.Primary700 = logoEnd
	}
	return theme
}

func deriveUITheme(primary uiColor) uiTheme {
	white := uiColor{R: 255, G: 255, B: 255}
	black := uiColor{}
	theme := uiTheme{
		Primary50:  mixColor(primary, white, 0.94),
		Primary100: mixColor(primary, white, 0.88),
		Primary200: mixColor(primary, white, 0.72),
		Primary300: mixColor(primary, white, 0.50),
		Primary400: mixColor(primary, white, 0.24),
		Primary500: primary,
		Primary600: mixColor(primary, black, 0.12),
		Primary700: mixColor(primary, black, 0.26),
		Primary800: mixColor(primary, black, 0.42),
		Primary900: mixColor(primary, black, 0.56),
	}
	theme.LogoEnd = theme.Primary700
	return theme
}

func parseThemeColor(value string) (uiColor, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return uiColor{}, false
	}
	if alias, ok := namedThemeColors[strings.ToLower(value)]; ok {
		value = alias
	}
	if !hexColorPattern.MatchString(value) {
		return uiColor{}, false
	}
	value = strings.TrimPrefix(value, "#")
	if len(value) == 3 {
		value = strings.Repeat(value[0:1], 2) + strings.Repeat(value[1:2], 2) + strings.Repeat(value[2:3], 2)
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(value, "%02x%02x%02x", &r, &g, &b); err != nil {
		return uiColor{}, false
	}
	return uiColor{R: r, G: g, B: b}, true
}

func mixColor(from, to uiColor, amount float64) uiColor {
	if amount < 0 {
		amount = 0
	} else if amount > 1 {
		amount = 1
	}
	mix := func(a, b uint8) uint8 {
		return uint8(math.Round(float64(a)*(1-amount) + float64(b)*amount))
	}
	return uiColor{R: mix(from.R, to.R), G: mix(from.G, to.G), B: mix(from.B, to.B)}
}

func (c uiColor) Hex() string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func (c uiColor) RGB() string {
	return fmt.Sprintf("%d, %d, %d", c.R, c.G, c.B)
}

func serveThemeCSS(c *gin.Context) {
	theme := currentUITheme()
	css := themeCSS(theme)
	c.Header("Cache-Control", "no-cache, must-revalidate")
	c.Header("Content-Type", "text/css; charset=utf-8")
	c.String(http.StatusOK, css)
}

func serveThemeLogoSVG(c *gin.Context) {
	theme := currentUITheme()
	svg := themeLogoSVG(theme)
	c.Header("Cache-Control", "no-cache, must-revalidate")
	c.Header("Content-Type", "image/svg+xml")
	c.String(http.StatusOK, svg)
}

func serveThemedManifestFrom(c *gin.Context, fileSystem fs.FS, filePath string) {
	content, err := fs.ReadFile(fileSystem, filePath)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	var manifest map[string]any
	if err := json.Unmarshal(content, &manifest); err == nil && manifest != nil {
		manifest["theme_color"] = currentUITheme().Primary500.Hex()
		manifest["icons"] = prependThemedSVGIcon(manifest["icons"])
		if encoded, err := json.MarshalIndent(manifest, "", "  "); err == nil {
			content = encoded
		}
	}

	if staticCacheDisabled || version.Version == "dev" {
		c.Header("Cache-Control", "no-cache, must-revalidate")
	} else {
		c.Header("Cache-Control", "public, max-age=3600, must-revalidate")
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Data(http.StatusOK, "application/json; charset=utf-8", content)
}

func prependThemedSVGIcon(existing any) []any {
	themedIcon := map[string]any{
		"src":     "/web/favicon.svg",
		"sizes":   "any",
		"type":    "image/svg+xml",
		"purpose": "any maskable",
	}

	icons, ok := existing.([]any)
	if !ok || len(icons) == 0 {
		return []any{themedIcon}
	}
	for _, icon := range icons {
		item, ok := icon.(map[string]any)
		if !ok {
			continue
		}
		if src, _ := item["src"].(string); src == "/web/favicon.svg" {
			return icons
		}
	}
	return append([]any{themedIcon}, icons...)
}

func themeCSS(theme uiTheme) string {
	return fmt.Sprintf(`/* Generated from %s. Edit .env and restart ccLoad to change the brand color. */
:root {
  --primary-rgb: %s;
  --primary-600-rgb: %s;
  --primary-700-rgb: %s;
  --primary-50: %s;
  --primary-100: %s;
  --primary-200: %s;
  --primary-300: %s;
  --primary-400: %s;
  --primary-500: %s;
  --primary-600: %s;
  --primary-700: %s;
  --primary-800: %s;
  --primary-900: %s;
  --page-bg:
    radial-gradient(1200px 600px at -10%% -10%%, rgba(var(--primary-rgb), 0.10), transparent 58%%),
    radial-gradient(900px 600px at 110%% -10%%, rgba(5, 150, 105, 0.08), transparent 58%%),
    linear-gradient(180deg, #f8fafc 0%%, var(--primary-50) 100%%);
  --surface-hover: rgba(var(--primary-600-rgb), 0.05);
  --surface-active: rgba(var(--primary-600-rgb), 0.10);
  --table-row-hover: rgba(var(--primary-600-rgb), 0.03);
  --glass-shadow: 0 8px 24px rgba(var(--primary-600-rgb), 0.06);
  --glass-shadow-hover: 0 12px 34px rgba(var(--primary-600-rgb), 0.10);
}

html[data-theme="dark"],
html[data-theme="system"][data-resolved-theme="dark"] {
  --page-bg:
    radial-gradient(900px 520px at 8%% -8%%, rgba(var(--primary-rgb), 0.16), transparent 58%%),
    radial-gradient(820px 520px at 108%% 0%%, rgba(16, 185, 129, 0.10), transparent 56%%),
    linear-gradient(180deg, #0f172a 0%%, #111827 100%%);
}

.brand-icon,
.logo-icon {
  box-shadow: 0 8px 32px rgba(var(--primary-rgb), 0.30);
}

.btn-primary {
  background: linear-gradient(135deg, var(--primary-500), var(--primary-600));
}

.btn-primary:hover,
.btn-success:hover {
  box-shadow: 0 8px 25px rgba(var(--primary-rgb), 0.40);
}

.login-container::before,
.input-decoration {
  background: linear-gradient(90deg, var(--primary-400), var(--primary-600), var(--primary-400));
}

.login-button {
  background: linear-gradient(135deg, var(--primary-500), var(--primary-700));
  box-shadow: 0 10px 28px rgba(var(--primary-rgb), 0.32);
}

.login-button:hover {
  box-shadow: 0 12px 36px rgba(var(--primary-rgb), 0.35);
}

html[data-theme="dark"] .login-button,
html[data-theme="system"][data-resolved-theme="dark"] .login-button {
  box-shadow: 0 10px 28px rgba(var(--primary-rgb), 0.24);
}

.form-input:focus,
.modal-input:focus,
.inline-input:focus,
.inline-select:focus {
  border-color: var(--primary-500);
  box-shadow: 0 0 0 3px rgba(var(--primary-rgb), 0.10);
}

.toggle-btn.active,
.time-range-btn.active {
  color: var(--primary-600);
}

.custom-range-link-btn {
  color: var(--primary-600);
}

.custom-range-link-btn:hover {
  background: rgba(var(--primary-rgb), 0.08);
}

.custom-range-confirm-btn,
.custom-range-day.selected-start,
.custom-range-day.selected-end {
  background: var(--primary-500);
}

.custom-range-confirm-btn:hover {
  background: var(--primary-600);
}

@keyframes brand-pulse {
  0%%, 100%% {
    transform: scale(1);
    box-shadow: 0 4px 16px rgba(var(--primary-rgb), 0.35);
  }
  50%% {
    transform: scale(1.18);
    box-shadow: 0 6px 22px rgba(var(--primary-rgb), 0.75);
  }
}
`, envThemeColor,
		theme.Primary500.RGB(), theme.Primary600.RGB(), theme.Primary700.RGB(),
		theme.Primary50.Hex(), theme.Primary100.Hex(), theme.Primary200.Hex(), theme.Primary300.Hex(), theme.Primary400.Hex(),
		theme.Primary500.Hex(), theme.Primary600.Hex(), theme.Primary700.Hex(), theme.Primary800.Hex(), theme.Primary900.Hex())
}

func themeLogoSVG(theme uiTheme) string {
	return fmt.Sprintf(`<svg width="64" height="64" viewBox="0 0 64 64" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <linearGradient id="bgGrad" x1="0%%" y1="0%%" x2="100%%" y2="100%%">
      <stop offset="0%%" stop-color="%s" stop-opacity="1" />
      <stop offset="100%%" stop-color="%s" stop-opacity="1" />
    </linearGradient>
  </defs>
  <rect width="64" height="64" rx="14" fill="url(#bgGrad)"/>
  <g fill="white" transform="translate(32,32) scale(1.3) translate(-32,-32)">
    <path d="M 15 32 C 15 26.5 19.5 22 25 22 C 27.5 22 29.7 23 31.2 24.5 L 28.5 27.2 C 27.5 26.2 26.3 25.5 25 25.5 C 21.4 25.5 18.5 28.4 18.5 32 C 18.5 35.6 21.4 38.5 25 38.5 C 26.3 38.5 27.5 37.8 28.5 36.8 L 31.2 39.5 C 29.7 41 27.5 42 25 42 C 19.5 42 15 37.5 15 32 Z" />
    <path d="M 33 32 C 33 26.5 37.5 22 43 22 C 45.5 22 47.7 23 49.2 24.5 L 46.5 27.2 C 45.5 26.2 44.3 25.5 43 25.5 C 39.4 25.5 36.5 28.4 36.5 32 C 36.5 35.6 39.4 38.5 43 38.5 C 44.3 38.5 45.5 37.8 46.5 36.8 L 49.2 39.5 C 47.7 41 45.5 42 43 42 C 37.5 42 33 37.5 33 32 Z" />
  </g>
</svg>
`, theme.Primary500.Hex(), theme.LogoEnd.Hex())
}

func injectThemeIntoHTML(page string) string {
	theme := currentUITheme()
	page = strings.ReplaceAll(page, `content="#3b82f6"`, `content="`+theme.Primary500.Hex()+`"`)
	page = strings.ReplaceAll(page, `content="#3B82F6"`, `content="`+theme.Primary500.Hex()+`"`)
	page = strings.ReplaceAll(page, "__THEME_COLOR__", theme.Primary500.Hex())

	marker := `  <script src="/web/assets/js/theme-init.js`
	if !strings.Contains(page, marker) || strings.Contains(page, "window.CCLOAD_THEME") {
		return page
	}
	return strings.Replace(page, marker, "  "+themeInlineScript(theme)+"\n"+marker, 1)
}

func themeInlineScript(theme uiTheme) string {
	payload := map[string]string{
		"primary50":  theme.Primary50.Hex(),
		"primary100": theme.Primary100.Hex(),
		"primary200": theme.Primary200.Hex(),
		"primary300": theme.Primary300.Hex(),
		"primary400": theme.Primary400.Hex(),
		"primary500": theme.Primary500.Hex(),
		"primary600": theme.Primary600.Hex(),
		"primary700": theme.Primary700.Hex(),
		"primary800": theme.Primary800.Hex(),
		"primary900": theme.Primary900.Hex(),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return `<script>window.CCLOAD_THEME=` + string(encoded) + `;</script>`
}
