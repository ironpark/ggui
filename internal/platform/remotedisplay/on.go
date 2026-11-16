// The constraint is ggwd's for linking its remote backend in.

//go:build ggwd_remote && unix && !android && !ios && !nintendosdk && !playstation5

package remotedisplay

import "os"

// envSocket is github.com/ironpark/ggwd/remote.EnvSocket, spelt out so
// that importing the root package does not pull ggwd/remote into every
// module graph: go mod tidy follows the imports of every build tag.
const envSocket = "GGWD_REMOTE_SOCKET"

// on reads the environment on every call, which is rare, so that a test
// can set it.
func on() bool { return os.Getenv(envSocket) != "" }
