//go:build js

package ggui

// wheelUnit converts a browser wheel delta to wheel units. Browsers report
// deltaY in CSS pixels (a mouse notch is 100 in Chrome, and a trackpad
// delivers the pixels it would scroll), while desktop backends report
// notches. One unit moves a Scroll by its default Speed of 20px, so dividing
// by 20 makes a page scroll by the same pixels the browser itself would.
const wheelUnit = 1.0 / 20
