package ggui

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/ironpark/ggui/internal/reactive"
)

// The reactive tests here read what only the package sees: the rows an
// Each keeps, a layout's alignment and what a Probe has posted but not yet
// run. Those that go through the public surface alone are in
// internal/apitest, and how often a Store published is in
// internal/reactive, with the Store.

func TestForRetainEvictsOffscreenRows(t *testing.T) {
	t.Parallel()
	var items []int
	for i := range 100 {
		items = append(items, i)
	}
	src := State(items)
	var f *EachWidget[int, int]
	off := State(0.0)
	dispose := reactive.Observe(func() {
		f = EachKeyed(src, func(i int) int { return i }, func(EachItem[int]) Widget { return Box().Size(10, 10) }).ItemExtent(10).Retain(2)
	})
	defer dispose()
	p := NewProbe(Scroll(f).BindOffset(off), Sz(10, 50))
	p.Frame()
	off.Set(500)
	p.Frame()
	if n := len(f.entries); n != 5+2 {
		t.Fatalf("mounted rows = %d, want 7", n)
	}
}

// Stretch is Align(AlignStretch).
func TestStretchIsAlignStretch(t *testing.T) {
	if Column().Stretch().align != AlignStretch || Row().Stretch().align != AlignStretch {
		t.Fatal("Stretch did not set AlignStretch")
	}
}

type resourceReply struct {
	value int
	err   error
}
type resourceRequest struct {
	input int
	ctx   context.Context
	reply chan resourceReply
}

func receiveRequest(t *testing.T, requests <-chan resourceRequest) resourceRequest {
	t.Helper()
	select {
	case r := <-requests:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("resource did not start")
		return resourceRequest{}
	}
}
func waitResourcePost(t *testing.T, p *Probe) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		p.postMu.Lock()
		n := len(p.posted)
		p.postMu.Unlock()
		if n > 0 {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("resource did not post completion")
}

func TestResourceCancellationLateResultsReloadAndErrors(t *testing.T) {
	t.Parallel()
	input := State(1)
	requests := make(chan resourceRequest, 10)
	var r *ResourceValue[int]
	p := ProbeBuilder(func() Widget {
		r = Resource(input.Get, func(ctx context.Context, n int) (int, error) {
			req := resourceRequest{n, ctx, make(chan resourceReply, 1)}
			requests <- req
			result := <-req.reply // Deliberately ignore cancellation.
			return result.value, result.err
		})
		return Await(r).Pending(func() Widget { return Text("pending") }).Then(func(v Readable[int]) Widget { return Textf("value %d", v) }).Catch(func(e Readable[error]) Widget { return Textf("error %v", e) })
	}, Sz(200, 30))
	defer p.Close()
	p.Frame()
	a := receiveRequest(t, requests)
	input.Set(2)
	p.Frame()
	b := receiveRequest(t, requests)
	if a.ctx.Err() != context.Canceled || b.input != 2 {
		t.Fatal("input change did not cancel/restart")
	}
	b.reply <- resourceReply{value: 0}
	waitResourcePost(t, p)
	p.Frame()
	if r.Get().Status != Ready || r.Get().Value != 0 {
		t.Fatal("zero value was not a success")
	}
	id := r.Get().RequestID
	a.reply <- resourceReply{value: 99}
	waitResourcePost(t, p)
	p.Frame()
	if r.Get().RequestID != id || r.Get().Value != 0 {
		t.Fatal("old completion overwrote latest")
	}
	r.Reload()
	p.Frame()
	c := receiveRequest(t, requests)
	if c.input != 2 || r.Get().RequestID == id {
		t.Fatal("reload did not start a new request")
	}
	c.reply <- resourceReply{err: errors.New("failed")}
	waitResourcePost(t, p)
	p.Frame()
	if r.Get().Status != Failed || r.Get().Err.Error() != "failed" {
		t.Fatal("error was not published")
	}
	r.Reload()
	p.Frame()
	d := receiveRequest(t, requests)
	p.Close()
	if d.ctx.Err() != context.Canceled {
		t.Fatal("dispose did not cancel")
	}
	// Completion after Close is dropped by the dispatcher; join the worker
	// through an independent channel in the disposal-specific test below.
	d.reply <- resourceReply{value: 7}
}

func TestResourceInputChangedBeforePostedCompletion(t *testing.T) {
	t.Parallel()
	input := State(1)
	requests := make(chan resourceRequest, 2)
	var r *ResourceValue[int]
	p := ProbeBuilder(func() Widget {
		r = Resource(input.Get, func(ctx context.Context, n int) (int, error) {
			req := resourceRequest{n, ctx, make(chan resourceReply, 1)}
			requests <- req
			v := <-req.reply
			return v.value, v.err
		})
		return Box()
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	a := receiveRequest(t, requests)
	a.reply <- resourceReply{value: 1}
	waitResourcePost(t, p)
	input.Set(2)
	p.Frame()
	if r.Get().Status != Pending || r.Get().RequestID != 2 {
		t.Fatal("stale posted value committed")
	}
	b := receiveRequest(t, requests)
	b.reply <- resourceReply{value: 2}
	waitResourcePost(t, p)
	p.Frame()
	if r.Get().Value != 2 {
		t.Fatal("new result lost")
	}
}

func TestResourceEqualityAndSharedAwait(t *testing.T) {
	t.Parallel()
	input := State(1)
	requests := make(chan resourceRequest, 3)
	show := State(true)
	var r *ResourceValue[int]
	p := ProbeBuilder(func() Widget {
		r = Resource(input.Get, func(ctx context.Context, n int) (int, error) {
			req := resourceRequest{n, ctx, make(chan resourceReply, 1)}
			requests <- req
			v := <-req.reply
			return v.value, v.err
		}, ResourceEqual(func(a, b int) bool { return a%2 == b%2 }))
		return Column(If(show, func() Widget { return Await(r) }), Await(r))
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	a := receiveRequest(t, requests)
	input.Set(3)
	show.Set(false)
	p.Frame()
	if a.ctx.Err() != nil || r.Get().RequestID != 1 {
		t.Fatal("equal input or consumer exit canceled shared resource")
	}
	a.reply <- resourceReply{value: 8}
	waitResourcePost(t, p)
	p.Frame()
	if r.Get().Value != 8 || len(requests) != 0 {
		t.Fatal("shared resource duplicated work")
	}
}

func TestResourceDisposedComponentIgnoresCompletionWhileAppLives(t *testing.T) {
	t.Parallel()
	shown := State(true)
	requests := make(chan resourceRequest, 1)
	var resource *ResourceValue[int]
	p := ProbeBuilder(func() Widget {
		return If(shown, func() Widget {
			resource = Resource(func() int { return 1 }, func(ctx context.Context, n int) (int, error) {
				req := resourceRequest{n, ctx, make(chan resourceReply, 1)}
				requests <- req
				result := <-req.reply
				return result.value, result.err
			})
			return Await(resource)
		})
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	request := receiveRequest(t, requests)
	shown.Set(false)
	p.Frame()
	if request.ctx.Err() != context.Canceled {
		t.Fatal("component exit did not cancel")
	}
	before := resource.Get()
	request.reply <- resourceReply{value: 99}
	waitResourcePost(t, p)
	p.Frame()
	if resource.Get() != before {
		t.Fatal("disposed resource accepted a result")
	}
}
