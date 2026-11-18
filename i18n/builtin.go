package i18n

// builtin holds the wording of the formats in the languages that ship
// with the package, beside English, which is their source. A catalog of
// the same language is looked in first.
var builtin = map[string]*Catalog{
	"ko": {
		Lang: "ko",
		Messages: map[string]string{
			"just now": "방금",

			"1/2/2006":                "2006. 1. 2.",
			"Jan 2, 2006":             "2006년 1월 2일",
			"Monday, January 2, 2006": "2006년 1월 2일 Monday",

			"Sunday [weekday]":    "일요일",
			"Monday [weekday]":    "월요일",
			"Tuesday [weekday]":   "화요일",
			"Wednesday [weekday]": "수요일",
			"Thursday [weekday]":  "목요일",
			"Friday [weekday]":    "금요일",
			"Saturday [weekday]":  "토요일",
			"Sun [weekday]":       "일",
			"Mon [weekday]":       "월",
			"Tue [weekday]":       "화",
			"Wed [weekday]":       "수",
			"Thu [weekday]":       "목",
			"Fri [weekday]":       "금",
			"Sat [weekday]":       "토",
		},
		Plurals: map[string]Forms{
			"%d byte": {Other: "%d바이트"},

			"%d minute ago": {Other: "%d분 전"},
			"%d hour ago":   {Other: "%d시간 전"},
			"%d day ago":    {Other: "%d일 전"},
			"%d week ago":   {Other: "%d주 전"},
			"%d month ago":  {Other: "%d개월 전"},
			"%d year ago":   {Other: "%d년 전"},
			"in %d minute":  {Other: "%d분 후"},
			"in %d hour":    {Other: "%d시간 후"},
			"in %d day":     {Other: "%d일 후"},
			"in %d week":    {Other: "%d주 후"},
			"in %d month":   {Other: "%d개월 후"},
			"in %d year":    {Other: "%d년 후"},
		},
	},
}
