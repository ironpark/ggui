package gfxtest

import (
	"fmt"

	"github.com/ebitengine/purego"
	"github.com/ironpark/ggfx/host"
)

// Mesa's surfaceless platform gives offscreen tests a context without X11,
// Wayland, or a GPU (Mesa can use its software renderer).
// https://registry.khronos.org/EGL/extensions/MESA/EGL_MESA_platform_surfaceless.txt
func newGLContext() (*eglContext, error) {
	lib, err := purego.Dlopen("libEGL.so.1", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, err
	}
	c := &eglContext{lib: lib}
	var (
		getDisplay    func(uint32, uintptr, *int) uintptr
		initialize    func(uintptr, *int32, *int32) uint32
		bindAPI       func(uint32) uint32
		chooseConfig  func(uintptr, *int32, *uintptr, int32, *int32) uint32
		createContext func(uintptr, uintptr, uintptr, *int32) uintptr
		createSurface func(uintptr, uintptr, *int32) uintptr
	)
	// Return a useful error instead of panicking when EGL 1.5 is unavailable.
	for name, fn := range map[string]any{
		"eglGetPlatformDisplay": &getDisplay, "eglInitialize": &initialize,
		"eglBindAPI": &bindAPI, "eglChooseConfig": &chooseConfig,
		"eglCreateContext": &createContext, "eglCreatePbufferSurface": &createSurface,
		"eglMakeCurrent": &c.makeCurrent, "eglGetError": &c.getError,
		"eglDestroyContext": &c.destroyContext, "eglDestroySurface": &c.destroySurface,
		"eglTerminate": &c.terminate,
	} {
		addr, err := purego.Dlsym(lib, name)
		if err != nil {
			purego.Dlclose(lib)
			return nil, err
		}
		purego.RegisterFunc(fn, addr)
	}
	success := false
	defer func() {
		if !success {
			c.Close()
		}
	}()
	c.display = getDisplay(0x31DD, 0, nil) // EGL_PLATFORM_SURFACELESS_MESA
	if c.display == 0 {
		return nil, c.err("eglGetPlatformDisplay")
	}
	if initialize(c.display, nil, nil) == 0 {
		return nil, c.err("eglInitialize")
	}
	const none = 0x3038                              // EGL_NONE
	api, renderable := uint32(0x30A2), int32(0x0008) // OpenGL
	ctxAttrs := []int32{none}
	if host.CurrentDriver().OpenGLES {
		api, renderable = 0x30A0, 0x0040    // OpenGL ES 3
		ctxAttrs = []int32{0x3098, 3, none} // EGL_CONTEXT_CLIENT_VERSION
	}
	if bindAPI(api) == 0 {
		return nil, c.err("eglBindAPI")
	}
	attrs := []int32{
		0x3033, 1, // EGL_SURFACE_TYPE: EGL_PBUFFER_BIT
		0x3040, renderable, // EGL_RENDERABLE_TYPE
		0x3024, 8, // EGL_RED_SIZE
		0x3023, 8, // EGL_GREEN_SIZE
		0x3022, 8, // EGL_BLUE_SIZE
		none,
	}
	var config uintptr
	var count int32
	if chooseConfig(c.display, &attrs[0], &config, 1, &count) == 0 || count == 0 {
		return nil, c.err("eglChooseConfig")
	}
	c.context = createContext(c.display, config, 0, &ctxAttrs[0])
	if c.context == 0 {
		return nil, c.err("eglCreateContext")
	}
	size := []int32{0x3057, 1, 0x3056, 1, none} // EGL_WIDTH, EGL_HEIGHT
	c.surface = createSurface(c.display, config, &size[0])
	if c.surface == 0 {
		return nil, c.err("eglCreatePbufferSurface")
	}
	success = true
	return c, nil
}

type eglContext struct {
	lib, display, context, surface uintptr
	makeCurrent                    func(uintptr, uintptr, uintptr, uintptr) uint32
	getError                       func() int32
	destroyContext, destroySurface func(uintptr, uintptr) uint32
	terminate                      func(uintptr) uint32
}

func (c *eglContext) err(op string) error {
	return fmt.Errorf("%s failed: EGL error %#x", op, c.getError())
}

func (c *eglContext) MakeContextCurrent() error {
	if c.makeCurrent(c.display, c.surface, c.surface, c.context) == 0 {
		return c.err("eglMakeCurrent")
	}
	return nil
}

func (c *eglContext) Close() {
	if c.display != 0 {
		c.makeCurrent(c.display, 0, 0, 0)
		if c.surface != 0 {
			c.destroySurface(c.display, c.surface)
		}
		if c.context != 0 {
			c.destroyContext(c.display, c.context)
		}
		c.terminate(c.display)
	}
	purego.Dlclose(c.lib)
}
