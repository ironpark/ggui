package framestats

import (
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// The counters are the process's, so the tests read what they add.

func TestReadCountsWhatWasAdded(t *testing.T) {
	before := Read()
	Frame(3 * time.Millisecond)
	For(Animation)
	For(Input)
	For(Input)
	CountPartial()
	CountSkipped()
	CountContained()
	got := Read()
	if got.Frames-before.Frames != 1 || got.Busy-before.Busy != 3*time.Millisecond ||
		got.ForAnimation-before.ForAnimation != 1 || got.ForInput-before.ForInput != 2 ||
		got.ForTime != before.ForTime || got.Partial-before.Partial != 1 ||
		got.Skipped-before.Skipped != 1 || got.Contained-before.Contained != 1 {
		t.Fatalf("from %+v to %+v", before, got)
	}
}

func TestReportNamesWhatRefusedAndDirtied(t *testing.T) {
	Refuse("")
	Refuse("test refusal")
	Refuse("test refusal")
	pc := reflect.ValueOf(TestReportNamesWhatRefusedAndDirtied).Pointer()
	Dirtied("dirty(test)", pc)
	SetMovers([]string{"a", "b"})
	r := ReadReport()
	if r.Refusals["test refusal"] != 2 || r.Refusals[""] != 0 {
		t.Fatalf("refusals %v", r.Refusals)
	}
	name := "dirty(test) " + "github.com/ironpark/ggui/internal/framestats.TestReportNamesWhatRefusedAndDirtied"
	if r.Dirtiers[name] != 1 {
		t.Fatalf("dirtiers %v, want %q", r.Dirtiers, name)
	}
	if strings.Join(r.Movers, ",") != "a,b" {
		t.Fatalf("movers %v", r.Movers)
	}
	// The report is a copy.
	r.Movers[0], r.Refusals["test refusal"] = "x", 0
	if again := ReadReport(); again.Movers[0] != "a" || again.Refusals["test refusal"] != 2 {
		t.Fatal("the report shares the counters")
	}
}

func TestDirtiersStopGrowingPastTheirBound(t *testing.T) {
	for pc := range uintptr(mostDirtiers + 10) {
		Dirtied("dirty(bound)", pc+1)
	}
	mu.Lock()
	n := len(dirtiers)
	mu.Unlock()
	if n > mostDirtiers {
		t.Fatalf("%d dirtiers kept, bound %d", n, mostDirtiers)
	}
}

// Windows on several goroutines count at once: run with -race.
func TestConcurrentCounting(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				Frame(time.Microsecond)
				Refuse("concurrent")
				Dirtied("dirty(concurrent)", 1)
				SetMovers([]string{"m"})
				_ = ReadReport()
			}
		})
	}
	wg.Wait()
	if ReadReport().Refusals["concurrent"] != 800 {
		t.Fatal("refusals lost")
	}
}

// Every frame counts itself and why it was asked for.
func BenchmarkFrame(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		Frame(time.Millisecond)
		For(Time)
	}
}

// Each running animation counts where it was started, every frame.
func BenchmarkDirtied(b *testing.B) {
	pc := reflect.ValueOf(BenchmarkDirtied).Pointer()
	b.ReportAllocs()
	for b.Loop() {
		Dirtied("dirty(anim)", pc)
	}
}

// A partial frame notes what moved.
func BenchmarkSetMovers(b *testing.B) {
	m := []string{"Caret", "Spinner", "Progress"}
	b.ReportAllocs()
	for b.Loop() {
		SetMovers(m)
	}
}

func TestTexturesGaugeWhatIsKept(t *testing.T) {
	before := ReadTextures()
	AddTexture(Layers, ImageBytes(10, 10))
	AddTexture(Icons, 64)
	AddTexture(Layers, -ImageBytes(10, 5))
	got := ReadTextures()
	if got.Layers-before.Layers != 200 || got.Icons-before.Icons != 64 || got.Total()-before.Total() != 264 {
		t.Fatalf("from %+v to %+v", before, got)
	}
}
