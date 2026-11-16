//go:build !ggwd_remote || !unix || android || ios || nintendosdk || playstation5

package remotedisplay

func on() bool { return false }
