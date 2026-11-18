package ggui

import (
	"fmt"
	"os"

	"github.com/ironpark/ggui/internal/kernel"
)

// recordEnv is the environment variable ggui record starts an app with, to
// have it draw a tour registered with package record rather than open its
// windows. Its value is a request that package reads.
const recordEnv = "GGUI_RECORD"

// recordIfAsked runs the recording recordEnv asks for and exits, when it
// is set, and returns when it is not. A program that does not import
// package record has no tours: it says so and exits with status 2.
func recordIfAsked() {
	req, ok := os.LookupEnv(recordEnv)
	if !ok {
		return
	}
	if kernel.Record == nil {
		fmt.Fprintf(os.Stderr, "ggui: %s is set, but this program has no tours to record: "+
			"it does not import github.com/ironpark/ggui/record; "+
			"register one with record.Register (see docs/recording.md)\n", recordEnv)
		os.Exit(2)
	}
	os.Exit(kernel.Record(req))
}
