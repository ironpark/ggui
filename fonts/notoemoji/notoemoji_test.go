package notoemoji_test

import (
	"testing"

	"github.com/ironpark/ggui/fonts/notoemoji"
)

func TestFontParsesTheEmbeddedFontOnce(t *testing.T) {
	if f := notoemoji.Font(); f == nil || f != notoemoji.Font() {
		t.Fatal("Font did not parse the embedded font once")
	}
}
