package ggui

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

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

func TestAwaitRequestIdentityAndReactiveValue(t *testing.T) {
	source := State(AsyncState[int]{RequestID: 1, Status: Ready, Value: 1})
	builds, cleanups := 0, 0
	var value Readable[int]
	view := Await(source).Then(func(v Readable[int]) Widget {
		builds++
		value = v
		OnCleanup(func() { cleanups++ })
		return Textf("%d", v)
	})
	p := NewProbe(view, Sz(100, 30))
	defer p.Close()
	p.Frame()
	source.Set(AsyncState[int]{RequestID: 1, Status: Ready, Value: 2})
	p.Frame()
	if builds != 1 || value.Get() != 2 {
		t.Fatal("same request rebuilt or value froze")
	}
	source.Set(AsyncState[int]{RequestID: 2, Status: Ready, Value: 3})
	p.Frame()
	if builds != 2 || cleanups != 1 {
		t.Fatal("fast request failed to remount")
	}
	assertPanics(t, func() { view.Pending(func() Widget { return Box() }) })
}

func TestResourceEqualityAndSharedAwait(t *testing.T) {
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
