// Command installer is a runnable example of a Go-authored ggui installer.
//
// Run it directly to preview the setup wizard in an interactive sandbox
// (no real files, shortcuts, PATH entries, or registry keys are modified):
//
//	go run ./docs/examples/installer
//	go run ./docs/examples/installer --uninstall
//
// Or run it with live hot reload using the ggui CLI:
//
//	ggui dev ./docs/examples/installer
package main

import (
	"fmt"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/installer"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

const sampleLicense = `MIT License

Copyright (c) 2026 Example Ltd

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.`

func newInstallerApp() *installer.App {
	return &installer.App{
		Name:        "Acme Studio",
		ID:          "com.example.acme-studio",
		Version:     "1.4.0",
		Description: "Desktop workspace built with ggui.",
		Publisher:   "Example Ltd",
		Homepage:    "https://example.com/acme-studio",
		License:     sampleLicense,
		Width:       560,
		Height:      420,
		Windows: installer.WindowsConfig{
			Scope:           installer.ScopeUser,
			DesktopShortcut: installer.Bool(true),
			Env: map[string]string{
				"ACME_HOME": "%INSTALL_DIR%",
			},
			Registry: []installer.RegistryEntry{
				{Key: `Software\Example\Acme Studio`, Name: "InstallDir", Value: installer.StringValue("%INSTALL_DIR%")},
				{Key: `Software\Example\Acme Studio`, Name: "Version", Value: installer.StringValue("%VERSION%")},
				{Key: `Software\Example\Acme Studio`, Name: "AutoUpdate", Value: installer.DWORDValue(1)},
			},
		},
		MacOS: installer.MacOSConfig{
			InstallLocation: "/Applications",
		},
		Steps: []installer.Step{
			installer.WelcomeStep("", ""),
			installer.LicenseStep(),
			installer.LocationStep(),
			installer.CustomStep("options", "Additional Options", func(ctx *installer.Context) ggui.Widget {
				return widget.Column(
					ui.Caption("Configure system integration and initial defaults."),
					ui.Checkbox(ctx.Bool("add_path", true), "Add install folder to user PATH"),
					ui.Checkbox(ctx.Bool("file_assoc", true), "Register .acme file association"),
					ui.Field("Release channel",
						ui.TextField(ctx.String("channel", "stable")).Placeholder("stable, beta, or nightly"),
					).Help("Saved to config.json and ACME_CHANNEL environment variable."),
				).Space(1).Align(widget.AlignStretch)
			}),
			installer.ProgressStep(),
			installer.FinishStep(),
		},
		AfterInstall: func(ctx *installer.Context) error {
			if ctx.GetBool("add_path") {
				ctx.ReportProgress(0.91, "Updating PATH…")
				if err := ctx.AddToPath(ctx.Dir()); err != nil {
					return err
				}
			}
			if ctx.GetBool("file_assoc") {
				ctx.ReportProgress(0.92, "Registering .acme file association…")
				root := ctx.RegistryRoot()
				if err := ctx.SetRegistryString(root, `Software\Classes\.acme`, "", "AcmeStudio.Document"); err != nil {
					return err
				}
				exeCmd := fmt.Sprintf(`"%s\%s" "%%1"`, ctx.Dir(), ctx.Meta().Executable)
				if err := ctx.SetRegistryString(root, `Software\Classes\AcmeStudio.Document\shell\open\command`, "", exeCmd); err != nil {
					return err
				}
			}
			channel := ctx.GetString("channel")
			if channel == "" {
				channel = "stable"
			}
			if err := ctx.SetEnv("ACME_CHANNEL", channel); err != nil {
				return err
			}
			cfg := fmt.Sprintf("{\n  \"channel\": %q\n}\n", channel)
			return ctx.WriteFile("config.json", []byte(cfg), 0o644)
		},
	}
}

func main() {
	installer.Main(newInstallerApp())
}
