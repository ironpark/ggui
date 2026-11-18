package windows

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"text/template"
)

// manifestTemplate is the application manifest Name.exe carries. Without
// one Windows runs a program as one written for Vista and older:
//
//   - DPI awareness. ggwd asks for per-monitor (v2) awareness itself when
//     it starts, but a process's awareness is best set before any window
//     exists, by the manifest, and Windows treats a program without it as
//     unaware until then. dpiAware is what Windows 8.1 and older read;
//     dpiAwareness, which Windows 10 1607 and later read instead, asks for
//     v2 and falls back to per-monitor where v2 is unknown.
//   - Common controls 6, the themed controls of Windows XP and later, for
//     the message boxes, task dialogs and file dialogs a program opens;
//     without it they are drawn as in Windows 2000.
//   - supportedOS, Windows 10 and 11's ID, without which GetVersionEx and
//     some compatibility shims treat the program as written for Windows 8.
//     Go programs need Windows 10, so it is the only one.
//   - The UTF-8 code page, so that the ANSI functions a dependency might
//     call, and the C runtime, take and give UTF-8 as Go does, on Windows
//     10 1903 and later.
//   - Long paths: Windows lets the program open paths past MAX_PATH when
//     the system allows them, which takes a policy or registry setting
//     the program cannot change.
//   - asInvoker: the program runs with the user's rights and is not
//     guessed to be an installer that wants an administrator's.
var manifestTemplate = template.Must(template.New("manifest").Parse(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="{{.ID}}" version="{{.Version}}"/>
  <dependency>
    <dependentAssembly>
      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
    </dependentAssembly>
  </dependency>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2, PerMonitor</dpiAwareness>
      <longPathAware xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">true</longPathAware>
      <activeCodePage xmlns="http://schemas.microsoft.com/SMI/2019/WindowsSettings">UTF-8</activeCodePage>
    </windowsSettings>
  </application>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
</assembly>
`))

// manifest returns the generated manifest for the application id, a
// reverse-DNS identifier, at version v. An assembly's name takes the
// letters, digits, dots and hyphens such an identifier is made of; the
// configuration has checked it is one.
func manifest(id string, v [4]uint16) []byte {
	var b bytes.Buffer
	var esc bytes.Buffer
	xml.EscapeText(&esc, []byte(id))
	err := manifestTemplate.Execute(&b, map[string]string{"ID": esc.String(), "Version": versionString(v)})
	if err != nil {
		panic(err) // the template is fixed, and its data strings
	}
	return b.Bytes()
}

// ReadManifest reads a manifest given to replace the generated one, and
// checks that it is well-formed XML whose root is an assembly: Windows
// refuses to start a program whose manifest it cannot parse, with an
// error about its side-by-side configuration that names no line.
func ReadManifest(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	root := ""
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			line, _ := d.InputPos()
			return nil, fmt.Errorf("%s:%d: not well-formed XML: %w", path, line, err)
		}
		if se, ok := tok.(xml.StartElement); ok && root == "" {
			root = se.Name.Local
			if root != "assembly" || se.Name.Space != "urn:schemas-microsoft-com:asm.v1" {
				return nil, fmt.Errorf(`%s: the root element is <%s>; an application manifest's is <assembly xmlns="urn:schemas-microsoft-com:asm.v1">`, path, root)
			}
		}
	}
	if root == "" {
		return nil, fmt.Errorf("%s: no XML elements; an application manifest is an <assembly>", path)
	}
	return data, nil
}
