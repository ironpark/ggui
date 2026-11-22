package main

import (
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/linux"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
)

// linuxPackage is the package name, and the program's name in /usr/bin:
// made from the app's name, else the main package's, else the last part
// of the ID.
func linuxPackage(m *project.Main) string {
	for _, s := range []string{m.Name, m.Executable, m.ID[strings.LastIndexByte(m.ID, '.')+1:]} {
		if p := linux.PackageName(s); p != "" {
			return p
		}
	}
	return "app"
}
