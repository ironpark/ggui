//go:build !ggwd_remote || !((unix && !android && !ios) || windows) || nintendosdk || playstation5

package remotedisplay

func on() bool { return false }
