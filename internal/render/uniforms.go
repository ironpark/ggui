package render

import (
	"sync"

	"github.com/ironpark/ggfx"
)

// uniformBlocks hands out uniform blocks for one shader, so that a draw
// repeated every frame fills a block instead of building a map. A draw
// copies its block, so a block goes back as soon as the draw returns; the
// pool only keeps two draws on different goroutines, such as two probes,
// from sharing one. Every draw sets every member, so what a block held
// before does not leak into the next.
type uniformBlocks struct {
	pool sync.Pool
}

func newUniformBlocks(shader func() *ggfx.Shader) *uniformBlocks {
	b := &uniformBlocks{}
	b.pool.New = func() any { return shader().NewUniforms() }
	return b
}

func (b *uniformBlocks) get() *ggfx.Uniforms  { return b.pool.Get().(*ggfx.Uniforms) }
func (b *uniformBlocks) put(u *ggfx.Uniforms) { b.pool.Put(u) }

// compile returns a shader built from source on first use. A source that
// fails to compile panics then, naming what.
func compile(what, source string) func() *ggfx.Shader {
	return sync.OnceValue(func() *ggfx.Shader {
		shader, err := ggfx.NewShader([]byte(source))
		if err != nil {
			panic("ggui: compile " + what + ": " + err.Error())
		}
		return shader
	})
}
