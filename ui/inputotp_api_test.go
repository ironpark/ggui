package ui_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

// InputOTP draws its own slots, so it is the handler for its editor's input
// and forwards it. Every input interface the editor answers -- the methods
// named for handling, consuming, claiming or capturing input -- must be
// forwarded too, or the field silently loses what the editor gained, as a
// chord the editor claims going to a menu instead.
func TestInputOTPForwardsTheEditorsInput(t *testing.T) {
	t.Parallel()
	editor := reflect.TypeFor[*widget.TextInputWidget]()
	otp := reflect.TypeFor[*ui.InputOTPWidget]()
	for m := range editor.Methods() {
		if !strings.HasPrefix(m.Name, "Handle") && !strings.HasPrefix(m.Name, "Consumes") &&
			!strings.HasPrefix(m.Name, "Claims") && !strings.HasPrefix(m.Name, "Capture") {
			continue
		}
		if _, ok := otp.MethodByName(m.Name); !ok {
			t.Errorf("widget.TextInputWidget.%s is not forwarded by InputOTPWidget", m.Name)
		}
	}
}
