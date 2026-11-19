//go:build !(darwin && !ios) && !windows && !js && !((linux && !android) || freebsd || netbsd || openbsd || dragonfly)

package notify

import "fmt"

// The platforms left, such as iOS and Android, have no notifications ggui
// posts.

func setup() {}

func check() error { return fmt.Errorf("%w on this platform", ErrUnavailable) }

func post(Notification) error { return check() }

func remove(string) {}

func permission(fn func(bool)) { go fn(false) }
