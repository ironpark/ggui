package runtime

import (
	"errors"
	"testing"
)

func TestStubAnswersEveryDialog(t *testing.T) {
	s := &StubFilePicker{Paths: []string{"/a.txt", "/b.txt"}}
	SetFilePicker(s)
	defer SetFilePicker(nil)

	if got, err := OpenFile(FileDialog{Title: "one"}); err != nil || got != "/a.txt" {
		t.Fatalf("OpenFile = %q, %v", got, err)
	}
	if got, err := OpenFiles(FileDialog{}); err != nil || len(got) != 2 {
		t.Fatalf("OpenFiles = %q, %v", got, err)
	}
	if got, err := PickFolder(FileDialog{}); err != nil || got != "/a.txt" {
		t.Fatalf("PickFolder = %q, %v", got, err)
	}
	if got, err := SaveFile(FileDialog{FileName: "x"}); err != nil || got != "/a.txt" {
		t.Fatalf("SaveFile = %q, %v", got, err)
	}
	if len(s.Asked) != 4 || s.Asked[0].Title != "one" || s.Asked[3].FileName != "x" {
		t.Fatalf("Asked = %+v", s.Asked)
	}
}

func TestStubWithoutPathsCancels(t *testing.T) {
	SetFilePicker(&StubFilePicker{})
	defer SetFilePicker(nil)
	if _, err := OpenFile(FileDialog{}); !errors.Is(err, ErrCanceled) {
		t.Fatalf("OpenFile err = %v, want ErrCanceled", err)
	}
	if _, err := OpenFiles(FileDialog{}); !errors.Is(err, ErrCanceled) {
		t.Fatalf("OpenFiles err = %v, want ErrCanceled", err)
	}
}

func TestStubErrWins(t *testing.T) {
	boom := errors.New("boom")
	SetFilePicker(&StubFilePicker{Paths: []string{"/a"}, Err: boom})
	defer SetFilePicker(nil)
	if _, err := SaveFile(FileDialog{}); !errors.Is(err, boom) {
		t.Fatalf("SaveFile err = %v, want boom", err)
	}
}

func TestSetFilePickerNilRestoresNative(t *testing.T) {
	SetFilePicker(&StubFilePicker{})
	SetFilePicker(nil)
	if _, ok := currentPicker().(nativePicker); !ok {
		t.Fatalf("picker after SetFilePicker(nil) = %T, want nativePicker", currentPicker())
	}
}

func TestAllAndFirstTurnNothingIntoACancel(t *testing.T) {
	if _, err := all(nil, nil); !errors.Is(err, ErrCanceled) {
		t.Fatalf("all(nil, nil) err = %v, want ErrCanceled", err)
	}
	if got, err := all([]string{"a"}, nil); err != nil || len(got) != 1 {
		t.Fatalf("all = %q, %v", got, err)
	}
	if got, err := first([]string{"a", "b"}, nil); err != nil || got != "a" {
		t.Fatalf("first = %q, %v", got, err)
	}
	if _, err := first(nil, nil); !errors.Is(err, ErrCanceled) {
		t.Fatalf("first(nil) err = %v", err)
	}
	boom := errors.New("boom")
	if _, err := first([]string{"a"}, boom); !errors.Is(err, boom) {
		t.Fatalf("first(err) = %v", err)
	}
}
