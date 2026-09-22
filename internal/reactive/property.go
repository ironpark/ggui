package reactive

import "github.com/ironpark/ggui/internal/property"

// Explicit widget properties are layout sources, just like signal versions.
// Keeping this separate from signals avoids owning effects for configuration.
type propertySource struct{ owner *property.Owner }

func (s propertySource) LayoutVersion() uint64 { return s.owner.Version() }
func init() {
	property.OnRead = func(owner *property.Owner) {
		Record(propertySource{owner}, owner.Version())
	}
	property.OnChange = RequestLayout
}

type constant[T any] struct{ value T }

func (c constant[T]) Get() T      { return c.value }
func (c constant[T]) GetAny() any { return c.value }

// Const supplies a fixed value to an API that takes a Readable. Ordinary widget
// setters take literals directly. Referenced data in v is not deep-copied.
func Const[T any](v T) Readable[T] { return constant[T]{v} }
