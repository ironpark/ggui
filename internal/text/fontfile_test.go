// Copyright 2026 The Ebitengine Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package text_test

import (
	"bytes"
	"image/color"
	"os"
	"runtime"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/text"
)

// openFont opens a font file for NewGoTextFaceSourceAt, skipping the test
// where the machine has no such file.
func openFont(t *testing.T, path string) (*os.File, int64) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("no %s: %v", path, err)
	}
	t.Cleanup(func() { f.Close() })
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return f, st.Size()
}

// drawn renders s with face, and returns the image's pixels.
func drawn(face text.Face, s string, w, h int) []byte {
	dst := ggfx.NewImage(w, h)
	defer dst.Deallocate()
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(color.White)
	text.Draw(dst, s, face, op)
	px := make([]byte, 4*w*h)
	dst.ReadPixels(px)
	return px
}

func heapAlloc() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// A color emoji font read from where it is draws as the same font read
// into memory does, holding none of its bitmaps until they are drawn.
func TestSourceAtReadsBitmapsOnDemand(t *testing.T) {
	const path = "/System/Library/Fonts/Apple Color Emoji.ttc"
	f, size := openFont(t, path)

	before := heapAlloc()
	lazy, err := text.NewGoTextFaceSourceAt(f, size, text.SourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	held := heapAlloc() - before
	if held > 64<<20 {
		t.Errorf("the source holds %d MB; want it to leave the bitmaps in the file", held>>20)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	eager, err := text.NewGoTextFaceSourcesFromCollection(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, sz := range []float64{20, 64, 160} {
		for _, s := range []string{"😀", "👨‍👩‍👧", "🇰🇷", "a😀b"} {
			w, h := int(sz)*4, int(sz)*2
			want := drawn(&text.GoTextFace{Source: eager[0], Size: sz}, s, w, h)
			got := drawn(&text.GoTextFace{Source: lazy, Size: sz}, s, w, h)
			if !bytes.Equal(got, want) {
				t.Errorf("size %v, %q: the lazy source draws differently", sz, s)
			}
			if bytes.Count(got, []byte{0}) == len(got) {
				t.Errorf("size %v, %q: nothing drawn", sz, s)
			}
		}
	}
	runtime.KeepAlive(lazy)
}

// A metrics-only source measures and reports coverage as the full one does.
func TestMetricsOnlySourceAgrees(t *testing.T) {
	check := func(t *testing.T, full, light *text.GoTextFaceSource) {
		for _, sz := range []float64{12, 16.5, 40} {
			fm := (&text.GoTextFace{Source: full, Size: sz}).Metrics()
			lm := (&text.GoTextFace{Source: light, Size: sz}).Metrics()
			if fm != lm {
				t.Errorf("size %v: metrics %+v, want %+v", sz, lm, fm)
			}
		}
		if full.Metadata() != light.Metadata() {
			t.Errorf("metadata %+v, want %+v", light.Metadata(), full.Metadata())
		}
		if a, b := len(full.AppendVariationAxes(nil)), len(light.AppendVariationAxes(nil)); a != b {
			t.Errorf("%d variation axes, want %d", b, a)
		}
		for _, r := range []rune("aZ한글漢字かな❯⏺😀\u0000") {
			fg := text.HasGlyph(&text.GoTextFace{Source: full, Size: 12}, r)
			lg := text.HasGlyph(&text.GoTextFace{Source: light, Size: 12}, r)
			if fg != lg {
				t.Errorf("%q: has glyph %v, want %v", r, lg, fg)
			}
		}
	}
	t.Run("Go Regular", func(t *testing.T) {
		r := bytes.NewReader(goregular.TTF)
		full, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
		if err != nil {
			t.Fatal(err)
		}
		light, err := text.NewGoTextFaceSourceAt(r, int64(len(goregular.TTF)), text.SourceOptions{MetricsOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		check(t, full, light)
	})
	for _, c := range []struct {
		path  string
		index int
	}{
		{"/System/Library/Fonts/AppleSDGothicNeo.ttc", 0},
		{"/System/Library/Fonts/AppleSDGothicNeo.ttc", 3},
		{"/System/Library/Fonts/SFNS.ttf", 0},
		{"/System/Library/Fonts/Hiragino Sans GB.ttc", 1},
	} {
		t.Run(c.path, func(t *testing.T) {
			f, size := openFont(t, c.path)
			full, err := text.NewGoTextFaceSourceAt(f, size, text.SourceOptions{Index: c.index})
			if err != nil {
				t.Fatal(err)
			}
			light, err := text.NewGoTextFaceSourceAt(f, size, text.SourceOptions{Index: c.index, MetricsOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			check(t, full, light)
			data, err := os.ReadFile(c.path)
			if err != nil {
				t.Fatal(err)
			}
			// A face read from the file draws as the collection's own.
			var eager *text.GoTextFaceSource
			if c.path[len(c.path)-3:] == "ttc" {
				all, err := text.NewGoTextFaceSourcesFromCollection(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				eager = all[c.index]
			} else if eager, err = text.NewGoTextFaceSource(bytes.NewReader(data)); err != nil {
				t.Fatal(err)
			}
			const s = "Hello 한글 漢字"
			want := drawn(&text.GoTextFace{Source: eager, Size: 24}, s, 300, 40)
			if got := drawn(&text.GoTextFace{Source: full, Size: 24}, s, 300, 40); !bytes.Equal(got, want) {
				t.Error("the face read from the file draws differently")
			}
		})
	}
}

// A LazyFace in a fallback chain loads its face only when text needs one
// of its glyphs, and draws as the face itself would.
func TestLazyFaceLoadsWhenNeeded(t *testing.T) {
	const path = "/System/Library/Fonts/AppleSDGothicNeo.ttc"
	f, size := openFont(t, path)
	head, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}
	light, err := text.NewGoTextFaceSourceAt(f, size, text.SourceOptions{MetricsOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	full, err := text.NewGoTextFaceSourceAt(f, size, text.SourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	loads := 0
	lazy := text.NewLazyFace(&text.GoTextFace{Source: light, Size: 20}, func() text.Face {
		loads++
		return &text.GoTextFace{Source: full, Size: 20}
	})
	chain, err := text.NewMultiFace(&text.GoTextFace{Source: head, Size: 20}, lazy)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := text.NewMultiFace(&text.GoTextFace{Source: head, Size: 20}, &text.GoTextFace{Source: full, Size: 20})
	if err != nil {
		t.Fatal(err)
	}
	if chain.Metrics() != direct.Metrics() {
		t.Errorf("metrics %+v, want %+v", chain.Metrics(), direct.Metrics())
	}
	_ = drawn(chain, "Latin only", 200, 40)
	_ = text.AdvanceAt("Latin only", len("Latin only"), chain)
	if loads != 0 || lazy.Loaded() {
		t.Fatalf("Latin text loaded the Korean font (%d loads)", loads)
	}
	const s = "Latin 한글"
	if got, want := drawn(chain, s, 200, 40), drawn(direct, s, 200, 40); !bytes.Equal(got, want) {
		t.Error("the chain draws differently from the loaded faces")
	}
	_ = text.AdvanceAt(s, len(s), chain)
	if loads != 1 || !lazy.Loaded() {
		t.Fatalf("%d loads; want one, for the Korean", loads)
	}
}
