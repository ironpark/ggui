// Copyright 2026 The Ebiten Authors
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

package gfxtest

import (
	"syscall/js"

	"github.com/ironpark/ggfx/host"
)

// newCanvasSurface makes a surface of a new canvas in the page, for which the WebGL driver is
// created and which the tests draw to.
func newCanvasSurface() error {
	c := js.Global().Get("document").Call("createElement", "canvas")
	js.Global().Get("document").Get("body").Call("appendChild", c)
	_, err := host.NewSurface(host.Canvas(c), false)
	return err
}
