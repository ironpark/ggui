package reactive

type constant[T any] struct{ value T }

func (c constant[T]) Get() T      { return c.value }
func (c constant[T]) GetAny() any { return c.value }

// Const supplies a fixed value to an API that takes a Readable. Ordinary widget
// setters take literals directly. Referenced data in v is not deep-copied.
func Const[T any](v T) Readable[T] { return constant[T]{v} }
