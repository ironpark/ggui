package reactive

import (
	"log"
	"sync"
)

// reported holds the keys ReportOnce has logged.
var reported sync.Map

// ReportOnce logs a diagnostic the first time it is made for key, such as
// the place a mistake was made, and never again for that key.
func ReportOnce(key string, format string, args ...any) {
	if _, seen := reported.LoadOrStore(key, true); !seen {
		log.Printf(format, args...)
	}
}
