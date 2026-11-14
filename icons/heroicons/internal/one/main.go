// Command one names a single icon, for the test that a program is linked
// with only the icons it names.
package main

import "github.com/ironpark/ggui/icons/heroicons"

func main() { println(heroicons.ArrowDownTray.Name, len(heroicons.ArrowDownTray.Data)) }
