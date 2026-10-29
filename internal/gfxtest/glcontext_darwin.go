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
	"fmt"

	"github.com/ebitengine/purego"
)

// glContext is a CGL context with no drawable: the renderer draws to textures alone.
type glContext struct {
	ctx uintptr
}

var cglSetCurrentContext func(ctx uintptr) int32

func newGLContext() (*glContext, error) {
	lib, err := purego.Dlopen("/System/Library/Frameworks/OpenGL.framework/OpenGL", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, err
	}
	var (
		cglChoosePixelFormat func(attribs *int32, pix *uintptr, npix *int32) int32
		cglCreateContext     func(pix uintptr, share uintptr, ctx *uintptr) int32
	)
	purego.RegisterLibFunc(&cglChoosePixelFormat, lib, "CGLChoosePixelFormat")
	purego.RegisterLibFunc(&cglCreateContext, lib, "CGLCreateContext")
	purego.RegisterLibFunc(&cglSetCurrentContext, lib, "CGLSetCurrentContext")

	const (
		kCGLPFAAccelerated       = 73
		kCGLPFAOpenGLProfile     = 99
		kCGLOGLPVersion_3_2_Core = 0x3200
	)
	// A machine without a GPU, like a CI runner, has only Apple's software renderer, which is not
	// accelerated: no accelerated format is kCGLBadPixelFormat there.
	var pix uintptr
	var npix int32
	var e int32
	for _, attribs := range [][]int32{
		{kCGLPFAOpenGLProfile, kCGLOGLPVersion_3_2_Core, kCGLPFAAccelerated, 0},
		{kCGLPFAOpenGLProfile, kCGLOGLPVersion_3_2_Core, 0},
	} {
		if e = cglChoosePixelFormat(&attribs[0], &pix, &npix); e == 0 && pix != 0 {
			break
		}
	}
	if e != 0 || pix == 0 {
		return nil, fmt.Errorf("CGLChoosePixelFormat failed: %d", e)
	}
	var ctx uintptr
	if e := cglCreateContext(pix, 0, &ctx); e != 0 {
		return nil, fmt.Errorf("CGLCreateContext failed: %d", e)
	}
	return &glContext{ctx: ctx}, nil
}

func (c *glContext) MakeContextCurrent() error {
	if e := cglSetCurrentContext(c.ctx); e != 0 {
		return fmt.Errorf("CGLSetCurrentContext failed: %d", e)
	}
	return nil
}
