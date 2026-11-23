package fswatch_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ironpark/ggui/pkg/fswatch"
)

// Watch a directory tree, leaving out editor temporary files, version
// control and node_modules, and print each batch of changes.
func Example() {
	dir, _ := os.MkdirTemp("", "fswatch-example")
	defer os.RemoveAll(dir)

	w := fswatch.New(fswatch.Options{
		Recursive: true,
		Ignore:    fswatch.DefaultIgnore().Match,
	})
	w.Add(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out := make(chan fswatch.Batch)
	go w.Run(ctx, out)
	time.Sleep(100 * time.Millisecond) // for the example: let Run start

	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt~"), []byte("backup"), 0o644)

	b := <-out
	for _, e := range b.Changes {
		fmt.Println(e.Op, filepath.Base(e.Path))
	}
	// Output:
	// create notes.txt
}
