package theme

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
)

// cssColors maps shadcn/ui's CSS variables to the tokens they set.
var cssColors = map[string]func(*Theme) *color.Color{
	"background":                 func(t *Theme) *color.Color { return &t.Bg },
	"foreground":                 func(t *Theme) *color.Color { return &t.Fg },
	"card":                       func(t *Theme) *color.Color { return &t.Card },
	"card-foreground":            func(t *Theme) *color.Color { return &t.CardFg },
	"popover":                    func(t *Theme) *color.Color { return &t.Popover },
	"popover-foreground":         func(t *Theme) *color.Color { return &t.PopoverFg },
	"primary":                    func(t *Theme) *color.Color { return &t.Primary },
	"primary-foreground":         func(t *Theme) *color.Color { return &t.PrimaryFg },
	"secondary":                  func(t *Theme) *color.Color { return &t.Secondary },
	"secondary-foreground":       func(t *Theme) *color.Color { return &t.SecondaryFg },
	"muted":                      func(t *Theme) *color.Color { return &t.Muted },
	"muted-foreground":           func(t *Theme) *color.Color { return &t.MutedFg },
	"accent":                     func(t *Theme) *color.Color { return &t.Accent },
	"accent-foreground":          func(t *Theme) *color.Color { return &t.AccentFg },
	"destructive":                func(t *Theme) *color.Color { return &t.Destructive },
	"destructive-foreground":     func(t *Theme) *color.Color { return &t.DestructiveFg },
	"border":                     func(t *Theme) *color.Color { return &t.Border },
	"input":                      func(t *Theme) *color.Color { return &t.InputBorder },
	"ring":                       func(t *Theme) *color.Color { return &t.Ring },
	"chart-1":                    func(t *Theme) *color.Color { return &t.Chart[0] },
	"chart-2":                    func(t *Theme) *color.Color { return &t.Chart[1] },
	"chart-3":                    func(t *Theme) *color.Color { return &t.Chart[2] },
	"chart-4":                    func(t *Theme) *color.Color { return &t.Chart[3] },
	"chart-5":                    func(t *Theme) *color.Color { return &t.Chart[4] },
	"sidebar":                    func(t *Theme) *color.Color { return &t.Sidebar },
	"sidebar-background":         func(t *Theme) *color.Color { return &t.Sidebar },
	"sidebar-foreground":         func(t *Theme) *color.Color { return &t.SidebarFg },
	"sidebar-primary":            func(t *Theme) *color.Color { return &t.SidebarPrimary },
	"sidebar-primary-foreground": func(t *Theme) *color.Color { return &t.SidebarPrimaryFg },
	"sidebar-accent":             func(t *Theme) *color.Color { return &t.SidebarAccent },
	"sidebar-accent-foreground":  func(t *Theme) *color.Color { return &t.SidebarAccentFg },
	"sidebar-border":             func(t *Theme) *color.Color { return &t.SidebarBorder },
	"sidebar-ring":               func(t *Theme) *color.Color { return &t.SidebarRing },
}

// FromCSS reads a shadcn/ui theme as its generators export it, a :root
// block and a .dark block of CSS variables, laid over Default and Dark:
//
//	light, dark, err := theme.FromCSS(`
//		:root { --radius: 0.625rem; --primary: oklch(0.205 0 0); ... }
//		.dark { --primary: oklch(0.922 0 0); ... }`)
//
// As in a browser, .dark inherits what it does not set from :root, so a
// stylesheet without one gives a dark theme in the light colors. Blocks may
// be nested in at-rules such as @layer; other selectors and variables ggui
// has no token for, such as fonts, are ignored. See ApplyCSS for the
// values it reads.
func FromCSS(css string) (light, dark Theme, err error) {
	var root, darkVars []string
	collectBlocks(stripComments(css), func(selector, body string) {
		switch {
		case strings.Contains(selector, ":root"):
			root = append(root, body)
		case strings.Contains(selector, ".dark"):
			darkVars = append(darkVars, body)
		}
	})
	rootDecls := strings.Join(root, ";")
	if light, err = Default().ApplyCSS(rootDecls); err != nil {
		return Theme{}, Theme{}, err
	}
	if dark, err = Dark().ApplyCSS(rootDecls + ";" + strings.Join(darkVars, ";")); err != nil {
		return Theme{}, Theme{}, err
	}
	return light, dark, nil
}

// ApplyCSS returns t with CSS variable declarations laid over it, such as
// the body of a shadcn/ui :root block:
//
//	t, err := theme.Default().ApplyCSS(`--primary: #2563eb; --radius: 0.5rem;`)
//
// Each color variable sets the token the Theme field comments name,
// --input setting InputBorder; ParseColor lists the color syntaxes. A value
// may be var() of another variable in decls. --radius sets the radii as
// shadcn derives its own from it: Radius 2px less, for the rounded-md
// controls, RadiusSm 4px less and RadiusLg 4px more. A later declaration
// wins, and variables with no token are ignored.
func (t Theme) ApplyCSS(decls string) (Theme, error) {
	vars := map[string]string{}
	var order []string
	for decl := range strings.SplitSeq(stripComments(decls), ";") {
		name, value, ok := strings.Cut(decl, ":")
		name = strings.TrimSpace(name)
		if !ok || !strings.HasPrefix(name, "--") {
			continue
		}
		name = strings.TrimPrefix(name, "--")
		if _, seen := vars[name]; !seen {
			order = append(order, name)
		}
		vars[name] = strings.TrimSpace(value)
	}
	for _, name := range order {
		value, err := resolveVar(vars, name, 0)
		if err != nil {
			return t, err
		}
		if field, ok := cssColors[name]; ok {
			c, err := ParseColor(value)
			if err != nil {
				return t, fmt.Errorf("theme: --%s: %w", name, err)
			}
			p := field(&t)
			*p = c
			t.pin(p)
			continue
		}
		if name == "radius" {
			r, err := parseLength(value)
			if err != nil {
				return t, fmt.Errorf("theme: --radius: %w", err)
			}
			t.Radius, t.RadiusSm, t.RadiusLg = max(r-2, 0), max(r-4, 0), r+4
			t.auto.radiusSm, t.auto.radiusLg = 0, 0
		}
	}
	return t.Resolve(), nil
}

// resolveVar returns the value of the variable name, following var().
func resolveVar(vars map[string]string, name string, depth int) (string, error) {
	value := vars[name]
	inner, ok := strings.CutPrefix(value, "var(")
	if !ok {
		return value, nil
	}
	if depth > 8 {
		return "", fmt.Errorf("theme: --%s: var() nests too deep", name)
	}
	ref, fallback, _ := strings.Cut(strings.TrimSuffix(inner, ")"), ",")
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "--")
	if _, ok := vars[ref]; !ok {
		if fallback = strings.TrimSpace(fallback); fallback != "" {
			return fallback, nil
		}
		return "", fmt.Errorf("theme: --%s: undefined %s", name, value)
	}
	return resolveVar(vars, ref, depth+1)
}

// collectBlocks calls fn with the selector and body of every innermost
// rule in css, descending into at-rules.
func collectBlocks(css string, fn func(selector, body string)) {
	for {
		open := strings.IndexByte(css, '{')
		if open < 0 {
			return
		}
		selector := css[:open]
		if i := strings.LastIndexAny(selector, ";}"); i >= 0 {
			selector = selector[i+1:]
		}
		depth, end := 1, open+1
		for ; end < len(css) && depth > 0; end++ {
			switch css[end] {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		body := css[open+1 : max(end-1, open+1)]
		if strings.ContainsRune(body, '{') {
			collectBlocks(body, fn)
		} else {
			fn(strings.TrimSpace(selector), body)
		}
		css = css[end:]
	}
}

func stripComments(s string) string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			return s[:i]
		}
		s = s[:i] + s[i+2+j+2:]
	}
}

// parseLength reads a CSS length in logical pixels: px, rem and em count
// 16px to the em, and a bare number is pixels.
func parseLength(s string) (float64, error) {
	s = strings.TrimSpace(s)
	scale := 1.0
	for _, u := range []struct {
		suffix string
		scale  float64
	}{{"rem", 16}, {"em", 16}, {"px", 1}} {
		if v, ok := strings.CutSuffix(s, u.suffix); ok {
			s, scale = v, u.scale
			break
		}
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("bad length %q", s)
	}
	return v * scale, nil
}

// ParseColor parses a CSS color as shadcn/ui themes write them: #rgb,
// #rgba, #rrggbb and #rrggbbaa; rgb(), rgba(), hsl(), hsla() and oklch(),
// in the comma or the space syntax with an optional / alpha; shadcn v3's
// bare HSL channels, such as "222.2 84% 4.9%"; and white, black and
// transparent. oklch colors outside sRGB are clipped to it.
func ParseColor(s string) (color.Color, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "white":
		return color.NRGBA{255, 255, 255, 255}, nil
	case "black":
		return color.NRGBA{0, 0, 0, 255}, nil
	case "transparent":
		return color.NRGBA{}, nil
	}
	if hex, ok := strings.CutPrefix(s, "#"); ok {
		return parseHex(hex)
	}
	fn, args := "hsl", s // shadcn v3 bare channels
	if open := strings.IndexByte(s, '('); open >= 0 && strings.HasSuffix(s, ")") {
		fn, args = s[:open], s[open+1:len(s)-1]
	}
	ch, alpha, err := channels(args)
	if err != nil {
		return nil, fmt.Errorf("bad color %q: %w", s, err)
	}
	var c color.NRGBA
	switch fn {
	case "rgb", "rgba":
		var v [3]float64
		for i, x := range ch {
			v[i] = x.of(255)
		}
		c = color.NRGBA{to8(v[0] / 255), to8(v[1] / 255), to8(v[2] / 255), 0}
	case "hsl", "hsla":
		c = hsl(ch[0].of(1), ch[1].of(100)/100, ch[2].of(100)/100)
	case "oklch":
		c = oklch(ch[0].of(1), ch[1].of(.4), ch[2].of(1))
	default:
		return nil, fmt.Errorf("bad color %q: unknown function %s", s, fn)
	}
	c.A = to8(alpha)
	return c, nil
}

// channel is one channel of a CSS color function: a number, or a
// percentage of the channel's full range.
type channel struct {
	v       float64
	percent bool
}

// of is the channel's value where 100% is full.
func (c channel) of(full float64) float64 {
	if c.percent {
		return c.v / 100 * full
	}
	return c.v
}

// channels splits the arguments of a color function into three channels
// and an alpha from 0 to 1.
func channels(args string) ([3]channel, float64, error) {
	var out [3]channel
	alpha := 1.0
	args = strings.ReplaceAll(args, ",", " ")
	main, a, hasAlpha := strings.Cut(args, "/")
	fields := strings.Fields(main)
	if !hasAlpha && len(fields) == 4 { // rgba(r, g, b, a)
		a, hasAlpha, fields = fields[3], true, fields[:3]
	}
	if len(fields) != 3 {
		return out, 0, fmt.Errorf("want 3 channels, got %d", len(fields))
	}
	for i, f := range fields {
		c, err := parseChannel(f)
		if err != nil {
			return out, 0, err
		}
		out[i] = c
	}
	if hasAlpha {
		c, err := parseChannel(strings.TrimSpace(a))
		if err != nil {
			return out, 0, err
		}
		alpha = c.of(1)
	}
	return out, alpha, nil
}

func parseChannel(s string) (channel, error) {
	if s == "none" {
		return channel{}, nil
	}
	s = strings.TrimSuffix(s, "deg")
	v, percent := strings.CutSuffix(s, "%")
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return channel{}, fmt.Errorf("bad channel %q", s)
	}
	return channel{f, percent}, nil
}

func parseHex(h string) (color.Color, error) {
	if len(h) == 3 || len(h) == 4 {
		var b strings.Builder
		for _, r := range h {
			b.WriteRune(r)
			b.WriteRune(r)
		}
		h = b.String()
	}
	if len(h) == 6 {
		h += "ff"
	}
	n, err := strconv.ParseUint(h, 16, 32)
	if len(h) != 8 || err != nil {
		return nil, fmt.Errorf("bad color #%s", h)
	}
	return color.NRGBA{uint8(n >> 24), uint8(n >> 16), uint8(n >> 8), uint8(n)}, nil
}

// to8 is a 0–1 channel as a byte, clipped.
func to8(v float64) uint8 { return uint8(math.Round(max(0, min(1, v)) * 255)) }

func hsl(h, s, l float64) color.NRGBA {
	h = math.Mod(math.Mod(h, 360)+360, 360) / 360
	f := func(n float64) float64 {
		k := math.Mod(n+h*12, 12)
		return l - s*min(l, 1-l)*max(-1, min(k-3, 9-k, 1))
	}
	return color.NRGBA{to8(f(0)), to8(f(8)), to8(f(4)), 0}
}

// oklch converts to sRGB; out-of-gamut channels are clipped.
func oklch(l, c, h float64) color.NRGBA {
	angle := h * math.Pi / 180
	x, y := c*math.Cos(angle), c*math.Sin(angle)
	ll, mm, ss := l+.3963377774*x+.2158037573*y, l-.1055613458*x-.0638541728*y, l-.0894841775*x-1.291485548*y
	ll, mm, ss = ll*ll*ll, mm*mm*mm, ss*ss*ss
	gamma := func(v float64) uint8 {
		if v <= .0031308 {
			v *= 12.92
		} else {
			v = 1.055*math.Pow(v, 1/2.4) - .055
		}
		return to8(v)
	}
	return color.NRGBA{
		gamma(4.0767416621*ll - 3.3077115913*mm + .2309699292*ss),
		gamma(-1.2684380046*ll + 2.6097574011*mm - .3413193965*ss),
		gamma(-.0041960863*ll - .7034186147*mm + 1.707614701*ss), 0,
	}
}
