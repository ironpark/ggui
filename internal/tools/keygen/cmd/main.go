// Command keygen writes ggui's key constant file. Run it with go generate
// after updating ggfx.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/ironpark/ggui/internal/tools/keygen"
)

func main() {
	out := flag.String("o", "keys_gen.go", "file to write")
	flag.Parse()
	src, err := keygen.Generate()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		log.Fatal(err)
	}
}
