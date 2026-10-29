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

package pool_test

import (
	"testing"

	"github.com/ironpark/ggui/internal/pool"
)

func TestPoolNew(t *testing.T) {
	var created int
	p := pool.Pool[*[]int]{
		New: func() *[]int {
			created++
			s := make([]int, 0, 4)
			return &s
		},
	}
	s := p.Get()
	if s == nil || cap(*s) != 4 {
		t.Fatalf("Get() = %v, want a new slice of capacity 4", s)
	}
	if created != 1 {
		t.Errorf("created = %d, want 1", created)
	}
	p.Put(s)
	_ = p.Get()
}
