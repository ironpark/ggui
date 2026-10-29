// Copyright 2020 The Ebiten Authors
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
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/ironpark/ggfx/host"
)

func init() {
	// The renderer is initialized on the main thread, which init runs on.
	runtime.LockOSThread()
}

// MainInFrame runs m inside one frame of the renderer, with no window: that is what makes
// (*Image).At and the rest of the graphics API available to a test. OpenGL needs a context to
// draw in, which the test gets of its own where it can (macOS); elsewhere a test runs with the
// driver that needs none.
func MainInFrame(m *testing.M) {
	code, err := runInFrame(m)
	if err != nil {
		panic(err)
	}
	os.Exit(code)
}

func runInFrame(m *testing.M) (int, error) {
	if err := host.Init(nil); err != nil {
		return 0, err
	}
	defer host.Terminate()

	if err := newCanvasSurface(); err != nil {
		return 0, err
	}
	if host.CurrentDriver().Library == host.GraphicsLibraryOpenGL && runtime.GOOS != "js" {
		c, err := newGLContext()
		if err != nil {
			return 0, fmt.Errorf("testing: OpenGL needs a context: %w", err)
		}
		host.SetGLContext(c)
	}

	f, err := host.BeginFrame()
	if err != nil {
		return 0, err
	}
	code := m.Run()
	if err := f.End(); err != nil {
		return 0, err
	}
	if _, err := host.Flush(false); err != nil {
		return 0, err
	}
	return code, nil
}
