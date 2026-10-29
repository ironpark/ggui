// Copyright 2026 The ggfx Authors
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

// Package pool provides a typed wrapper of sync.Pool.
package pool

import (
	"sync"
)

// Pool is a sync.Pool that holds values of T only.
//
// A Pool must not be copied after first use.
type Pool[T any] struct {
	// New creates a value when the pool is empty.
	New func() T

	p sync.Pool
}

// Get takes a value from the pool, or creates one with New when the pool is empty.
func (p *Pool[T]) Get() T {
	if v := p.p.Get(); v != nil {
		return v.(T)
	}
	return p.New()
}

// Put returns v to the pool. Resetting v is the caller's responsibility.
func (p *Pool[T]) Put(v T) {
	p.p.Put(v)
}
