package i18n

import (
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/ironpark/ggui"
)

// The formats below are Messages too: what they write follows the
// language as a sentence does. English and Korean are built in. Another
// language words them in its catalog, under keys that are the English
// they replace (see docs/i18n.md for the list); what it leaves out is
// written in English.

// now is the time Since measures from; a test replaces it.
var now = time.Now

// timeArg is the value a Since or a Date message holds.
type timeArg struct {
	t     time.Time
	style DateStyle
}

// Since returns how long ago t was, or how long until it is, in words:
// "just now", "5 minutes ago", "in 2 days"; in Korean "방금", "5분 전",
// "2일 후". It is measured when it is read, at layout: nothing lays it out
// again as time passes, so a list showing it refreshes it with a state of
// its own that changes every minute or so.
//
// A catalog words it under "just now" in Messages and, in Plurals, "%d
// minute ago", "%d hour ago", "%d day ago", "%d week ago", "%d month ago"
// and "%d year ago", and the same starting "in %d" for the future.
func Since(t time.Time) Message {
	return Message{kind: kindSince, args: &[]any{timeArg{t: t}}}
}

// spans are Since's units, largest first: how long one is and the
// messages for a count of them past and to come.
var spans = []struct {
	d                  time.Duration
	ago, agoN, in, inN string
}{
	{365 * 24 * time.Hour, "%d year ago", "%d years ago", "in %d year", "in %d years"},
	{30 * 24 * time.Hour, "%d month ago", "%d months ago", "in %d month", "in %d months"},
	{7 * 24 * time.Hour, "%d week ago", "%d weeks ago", "in %d week", "in %d weeks"},
	{24 * time.Hour, "%d day ago", "%d days ago", "in %d day", "in %d days"},
	{time.Hour, "%d hour ago", "%d hours ago", "in %d hour", "in %d hours"},
	{time.Minute, "%d minute ago", "%d minutes ago", "in %d minute", "in %d minutes"},
}

func (l *Localizer) since(tag string, t time.Time) string {
	d := now().Sub(t)
	future := d < 0
	var abs uint64
	if future {
		if d == math.MinInt64 {
			abs = 1 << 63
		} else {
			abs = uint64(-d)
		}
	} else {
		abs = uint64(d)
	}
	for _, s := range spans {
		sd := uint64(s.d)
		if abs < sd {
			continue
		}
		n := int(abs / sd)
		if future {
			return l.plural(tag, n, s.in, s.inN, "", nil)
		}
		return l.plural(tag, n, s.ago, s.agoN, "", nil)
	}
	return l.text(tag, "just now", "")
}

// Bytes returns the size n in bytes as a person reads it, in units of
// 1000 as the Finder and most file managers count: "512 bytes", "1.5 KB",
// "23 MB", with the language's decimal separator.
//
// A catalog words it under "%d byte" in Plurals and "%s KB", "%s MB",
// "%s GB", "%s TB" and "%s PB" in Messages.
func Bytes(n int64) Message {
	return Message{kind: kindBytes, args: &[]any{n}}
}

var byteUnits = []string{"%s KB", "%s MB", "%s GB", "%s TB", "%s PB", "%s EB"}

func (l *Localizer) bytes(tag string, n int64) string {
	abs := math.Abs(float64(n))
	if abs < 1000 {
		return l.plural(tag, int(n), "%d byte", "%d bytes", "", nil)
	}
	v, unit := abs, -1
	for v >= 1000 && unit < len(byteUnits)-1 {
		v /= 1000
		unit++
	}
	var s string
	if r := math.Round(v*10) / 10; r < 10 {
		s = strconv.FormatFloat(r, 'f', -1, 64)
	} else {
		r = math.Round(v)
		if r >= 1000 && unit < len(byteUnits)-1 {
			r, unit = 1, unit+1
		}
		s = strconv.FormatFloat(r, 'f', 0, 64)
	}
	s = l.localDigits(tag, s)
	if n < 0 {
		s = "-" + s
	}
	return l.format(tag, l.text(tag, byteUnits[unit], ""), []any{s})
}

// Number returns n written as the language writes numbers: its digits
// grouped by thousands and its decimal separator, "1,234,567.5" in English
// and "1.234.567,5" in a German catalog that says so. A float is written
// with as many digits as tell it apart from its neighbours.
//
// A catalog gives its separators as the Messages "." and "," with the
// contexts "decimal separator" and "group separator": ". [decimal
// separator]" and ", [group separator]".
func Number[N ggui.Number](n N) Message {
	var v any
	switch r := reflect.ValueOf(n); r.Kind() {
	case reflect.Float32:
		v = float32(r.Float())
	case reflect.Float64:
		v = r.Float()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v = r.Uint()
	default:
		v = r.Int()
	}
	return Message{kind: kindNumber, args: &[]any{v}}
}

// number writes v, an int64, a uint64, a float32 or a float64, in tag.
func (l *Localizer) number(tag string, v any) string {
	var s string
	switch n := v.(type) {
	case int64:
		s = strconv.FormatInt(n, 10)
	case uint64:
		s = strconv.FormatUint(n, 10)
	case float32:
		s = strconv.FormatFloat(float64(n), 'f', -1, 32)
	case float64:
		s = strconv.FormatFloat(n, 'f', -1, 64)
	}
	return l.localDigits(tag, s)
}

// localDigits groups the integer part of the plain number s by thousands
// and puts in the language's separators.
func (l *Localizer) localDigits(tag, s string) string {
	if s == "NaN" || strings.Contains(s, "Inf") {
		return s
	}
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	whole, frac, hasFrac := strings.Cut(s, ".")
	group := l.text(tag, ",", "group separator")
	var b strings.Builder
	b.WriteString(sign)
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteString(group)
		}
		b.WriteRune(c)
	}
	if hasFrac {
		b.WriteString(l.text(tag, ".", "decimal separator"))
		b.WriteString(frac)
	}
	return b.String()
}

// DateStyle is how much of a date Date writes.
type DateStyle uint8

const (
	// DateShort is the date in figures: "1/2/2006", "2006. 1. 2.".
	DateShort DateStyle = iota
	// DateMedium names the month briefly: "Jan 2, 2006", "2006년 1월 2일".
	DateMedium
	// DateLong names the month and the weekday in full: "Monday, January 2,
	// 2006", "2006년 1월 2일 월요일".
	DateLong
)

// dateLayouts are the English layouts of each DateStyle, which are also
// the keys a catalog gives its own under.
var dateLayouts = [...]string{
	DateShort:  "1/2/2006",
	DateMedium: "Jan 2, 2006",
	DateLong:   "Monday, January 2, 2006",
}

// Date returns the date of t written in style, as the language writes
// dates: "Jan 2, 2006" in English, "2006년 1월 2일" in Korean. The date is
// t's own, in t's location.
//
// A catalog gives a style's layout, a time.Time.Format layout, as the
// translation of the English one in Messages: "1/2/2006", "Jan 2, 2006"
// and "Monday, January 2, 2006". The names of months and weekdays in it
// are translated under their English with the contexts "month" and
// "weekday", as "January [month]", "Jan [month]", "Monday [weekday]" and
// "Mon [weekday]".
func Date(t time.Time, style DateStyle) Message {
	return Message{kind: kindDate, args: &[]any{timeArg{t: t, style: style}}}
}

func (l *Localizer) date(tag string, t time.Time, style DateStyle) string {
	if int(style) >= len(dateLayouts) {
		style = DateMedium
	}
	layout := l.text(tag, dateLayouts[style], "")
	month, weekday := t.Month().String(), t.Weekday().String()
	names := []struct{ token, ctx, name string }{
		{"January", "month", month},
		{"Monday", "weekday", weekday},
		{"Jan", "month", month[:3]},
		{"Mon", "weekday", weekday[:3]},
	}
	// The layout is formatted a piece at a time, each name between the
	// pieces translated: a translated name put in the layout would be read
	// as a layout itself, as "1월" would be read as the month.
	var b strings.Builder
	start := 0
	for i := 0; i < len(layout); {
		matched := false
		for _, n := range names {
			if strings.HasPrefix(layout[i:], n.token) {
				b.WriteString(t.Format(layout[start:i]))
				b.WriteString(l.text(tag, n.name, n.ctx))
				i += len(n.token)
				start, matched = i, true
				break
			}
		}
		if !matched {
			i++
		}
	}
	b.WriteString(t.Format(layout[start:]))
	return b.String()
}
