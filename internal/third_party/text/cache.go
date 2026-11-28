// Copyright 2024 The Ebitengine Authors
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

package text

import (
	"cmp"
	"math"
	"slices"
	"sync"
)

const infTick = math.MaxInt64

// maxAge is how many ticks, frames, an entry may go unused before a cache
// lets it go however few entries it has, and sweepEvery how often, in
// ticks, a cache checks for aged entries. A window draws tens of frames a
// second while active, so an entry goes after ~20 seconds of active use
// without it, or several minutes at rest.
const (
	maxAge     = 1200
	sweepEvery = 300
)

type cacheValue[Value any] struct {
	value Value

	// atime is the last time when the value was accessed.
	atime int64
}

type cache[Key comparable, Value any] struct {
	// softLimit indicates the soft limit of the number of values in the cache.
	softLimit int

	values map[Key]*cacheValue[Value]

	// atime is the last time when the cache was accessed.
	atime int64

	// swept is when entries were last looked over for age (see maxAge).
	swept int64

	// own, when set, returns a key that shares no memory with the one
	// asked for. A key holding a substring keeps the whole string it was
	// cut from alive for as long as the entry lives: an editor's every
	// keystroke makes a new copy of its document, and the lines shaped
	// from each would keep one copy apiece. own runs only on a miss, so a
	// hit costs nothing.
	own func(Key) Key

	// onEvict, when set, is called outside c.m for each value removed from
	// the cache during aging, LRU eviction, or clear.
	onEvict func(Value)

	m sync.Mutex
}

func newCache[Key comparable, Value any](softLimit int) *cache[Key, Value] {
	return &cache[Key, Value]{
		softLimit: softLimit,
	}
}

func (c *cache[Key, Value]) hardLimit() int {
	return max(2*c.softLimit, c.softLimit+64)
}

func (c *cache[Key, Value]) getOrCreate(key Key, create func() (Value, bool)) Value {
	n := currentTick()

	c.m.Lock()
	var evicted []Value

	e, ok := c.values[key]
	if ok {
		e.atime = n
		if n-c.swept >= sweepEvery {
			evicted = c.sweepAgedLocked(n, evicted)
		}
		val := e.value
		onEvict := c.onEvict
		c.m.Unlock()
		if onEvict != nil {
			for _, v := range evicted {
				onEvict(v)
			}
		}
		return val
	}

	if c.values == nil {
		c.values = map[Key]*cacheValue[Value]{}
	}

	ent, canExpire := create()
	if c.own != nil {
		key = c.own(key)
	}
	e = &cacheValue[Value]{
		value: ent,
		atime: infTick,
	}
	if canExpire {
		e.atime = n
	}
	c.values[key] = e

	// Below the soft limit, entries go only once unused for maxAge: a UI
	// shows the same strings for hours, and those it showed once, a
	// conversation scrolled past, a file closed, are kept for nothing.
	if n-c.swept >= sweepEvery {
		evicted = c.sweepAgedLocked(n, evicted)
	}

	// Clean up old entries once past the soft limit.
	if c.atime < n && len(c.values) > c.softLimit {
		for k, v := range c.values {
			// 60 is an arbitrary number.
			if v.atime >= n-60 {
				continue
			}
			if c.onEvict != nil {
				evicted = append(evicted, v.value)
			}
			delete(c.values, k)
		}
	}

	// Enforce a hard LRU cap so rapid bursts (e.g. scrolling through many CJK
	// glyphs within 60 frames) cannot grow the cache unboundedly. Never evict
	// entries accessed on the current or immediately preceding tick (n-1) or
	// non-expirable entries, so in-flight GPU frames keep their glyph textures.
	if limit := c.hardLimit(); len(c.values) > limit {
		evicted = c.evictLRULocked(n, evicted)
	}

	c.atime = n
	val := e.value
	onEvict := c.onEvict
	c.m.Unlock()

	if onEvict != nil {
		for _, v := range evicted {
			onEvict(v)
		}
	}
	return val
}

func (c *cache[Key, Value]) sweepAgedLocked(n int64, evicted []Value) []Value {
	c.swept = n
	for k, v := range c.values {
		if v.atime < n-maxAge {
			if c.onEvict != nil {
				evicted = append(evicted, v.value)
			}
			delete(c.values, k)
		}
	}
	return evicted
}

func (c *cache[Key, Value]) evictLRULocked(n int64, evicted []Value) []Value {
	type victim struct {
		key   Key
		atime int64
	}
	candidates := make([]victim, 0, len(c.values))
	for k, v := range c.values {
		if v.atime < n-1 {
			candidates = append(candidates, victim{key: k, atime: v.atime})
		}
	}
	need := len(c.values) - c.softLimit
	if need <= 0 || len(candidates) == 0 {
		return evicted
	}
	slices.SortFunc(candidates, func(a, b victim) int {
		return cmp.Compare(a.atime, b.atime)
	})
	if need > len(candidates) {
		need = len(candidates)
	}
	for _, cand := range candidates[:need] {
		if v, ok := c.values[cand.key]; ok {
			if c.onEvict != nil {
				evicted = append(evicted, v.value)
			}
			delete(c.values, cand.key)
		}
	}
	return evicted
}

func (c *cache[Key, Value]) clear() {
	c.m.Lock()
	onEvict := c.onEvict
	var evicted []Value
	if onEvict != nil && len(c.values) > 0 {
		evicted = make([]Value, 0, len(c.values))
		for _, v := range c.values {
			evicted = append(evicted, v.value)
		}
	}
	clear(c.values)
	c.m.Unlock()
	if onEvict != nil {
		for _, v := range evicted {
			onEvict(v)
		}
	}
}
