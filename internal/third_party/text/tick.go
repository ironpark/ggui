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

import "sync/atomic"

var tick atomic.Int64

// AdvanceTick advances the clock that the glyph image and shaping caches expire against. ggui
// advances it once a frame.
func AdvanceTick() {
	tick.Add(1)
}

func currentTick() int64 {
	return tick.Load()
}
