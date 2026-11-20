//go:build darwin && !ios

package nativemenu

import (
	"reflect"
	"testing"

	"github.com/ironpark/ggwd"
)

func TestToRemoteItems(t *testing.T) {
	if got := toRemoteItems(nil); got != nil {
		t.Fatalf("toRemoteItems(nil) = %v, want nil", got)
	}
	in := []Item{
		{
			Title: "File",
			Items: []Item{
				{Title: "New", ID: 1, Key: "n", Cmd: true, Enabled: true},
				{Separator: true},
				{
					Title: "Export",
					Items: []Item{
						{Title: "PNG", ID: 2, Key: "e", Cmd: true, Shift: true, Alt: true, Ctrl: true, Enabled: false, Checked: true},
					},
				},
			},
		},
	}
	want := []ggwd.RemoteMenuItem{
		{
			Title: "File",
			Items: []ggwd.RemoteMenuItem{
				{Title: "New", ID: 1, Key: "n", Cmd: true, Enabled: true},
				{Separator: true},
				{
					Title: "Export",
					Items: []ggwd.RemoteMenuItem{
						{Title: "PNG", ID: 2, Key: "e", Cmd: true, Shift: true, Alt: true, Ctrl: true, Enabled: false, Checked: true},
					},
				},
			},
		},
	}
	if got := toRemoteItems(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("toRemoteItems =\n%+v\nwant\n%+v", got, want)
	}
}

func TestToRemoteTray(t *testing.T) {
	in := Tray{
		Icon:     []byte{0x89, 'P', 'N', 'G'},
		Template: true,
		Title:    "Status",
		Tooltip:  "Running",
		Menu:     []Item{{Title: "Show", ID: 10, Enabled: true}},
		ClickID:  0,
	}
	want := ggwd.RemoteTray{
		Icon:     []byte{0x89, 'P', 'N', 'G'},
		Template: true,
		Title:    "Status",
		Tooltip:  "Running",
		Menu:     []ggwd.RemoteMenuItem{{Title: "Show", ID: 10, Enabled: true}},
		ClickID:  0,
	}
	if got := toRemoteTray(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("toRemoteTray =\n%+v\nwant\n%+v", got, want)
	}
}
