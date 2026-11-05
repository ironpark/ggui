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
	"math"
	"sync"
)

const infTick = math.MaxInt64

// maxAge is how many ticks, frames, an entry may go unused before a cache
// lets it go however few entries it has, and sweepEvery how often, in
// ticks, a cache that adds an entry looks for those. A window draws a few
// frames a second at rest, tens while working, so an entry goes after
// minutes of use without it, or about an hour at rest.
const (
	maxAge     = 3600
	sweepEvery = 600
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

	m sync.Mutex
}

func newCache[Key comparable, Value any](softLimit int) *cache[Key, Value] {
	return &cache[Key, Value]{
		softLimit: softLimit,
	}
}

func (c *cache[Key, Value]) getOrCreate(key Key, create func() (Value, bool)) Value {
	n := currentTick()

	c.m.Lock()
	defer c.m.Unlock()

	e, ok := c.values[key]
	if ok {
		e.atime = n
		return e.value
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
		c.swept = n
		for key, e := range c.values {
			if e.atime < n-maxAge {
				delete(c.values, key)
			}
		}
	}

	// Clean up old entries.
	if c.atime < n {
		// If the number of values exceeds the soft limits, old values are removed.
		// Even after cleaning up the cache, the number of values might still exceed the soft limit,
		// but this is fine.
		if len(c.values) > c.softLimit {
			for key, e := range c.values {
				// 60 is an arbitrary number.
				if e.atime >= n-60 {
					continue
				}
				delete(c.values, key)
			}
		}
	}

	c.atime = n

	return e.value
}
