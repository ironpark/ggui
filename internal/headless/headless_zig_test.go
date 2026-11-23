//go:build ggfxzig && ((darwin && !ios) || windows || (linux && !android)) && (amd64 || arm64)

package headless

import (
	"image/color"
	"os"
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/host"
)

func TestZigDriverHeadless(t *testing.T) {
	if os.Getenv("GGFX_DRIVER") == "go" {
		t.Skip("GGFX_DRIVER=go explicitly disables the Zig driver")
	}
	stop, err := Start()
	if err != nil {
		t.Fatalf("headless.Start() failed: %v", err)
	}
	defer stop()

	if got := host.DriverImplementation(); got != "zig" {
		t.Fatalf("host.DriverImplementation() = %q, want %q (library=%s)", got, "zig", host.CurrentDriver().Library)
	}

	img := ggfx.NewImage(16, 16)
	defer img.Deallocate()
	want := color.RGBA{R: 0x34, G: 0x78, B: 0xbc, A: 0xff}
	img.Fill(want)

	f, err := host.BeginFrame()
	if err != nil {
		t.Fatalf("BeginFrame: %v", err)
	}
	pix := make([]byte, 16*16*4)
	img.ReadPixels(pix)
	if err := f.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if _, err := host.Flush(false); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	got := color.RGBA{R: pix[0], G: pix[1], B: pix[2], A: pix[3]}
	if got != want {
		t.Fatalf("pixel[0,0] = %v, want %v", got, want)
	}
}
