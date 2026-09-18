//go:build !js

package ggui

// wheelUnit converts a desktop wheel delta to wheel units. GLFW already
// reports notches, so one delta is one unit.
const wheelUnit = 1.0
