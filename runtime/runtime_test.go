package runtime

import (
	"errors"
	"testing"
)

func TestStubAnswersEveryDialog(t *testing.T) {
	s := &StubFilePicker{Paths: []string{"/a.txt", "/b.txt"}}

	if got, err := s.OpenFile(FileDialog{Title: "one"}); err != nil || got != "/a.txt" {
		t.Fatalf("OpenFile = %q, %v", got, err)
	}
	if got, err := s.OpenFiles(FileDialog{}); err != nil || len(got) != 2 {
		t.Fatalf("OpenFiles = %q, %v", got, err)
	}
	if got, err := s.PickFolder(FileDialog{}); err != nil || got != "/a.txt" {
		t.Fatalf("PickFolder = %q, %v", got, err)
	}
	if got, err := s.SaveFile(FileDialog{FileName: "x"}); err != nil || got != "/a.txt" {
		t.Fatalf("SaveFile = %q, %v", got, err)
	}
	if len(s.Asked) != 4 || s.Asked[0].Title != "one" || s.Asked[3].FileName != "x" {
		t.Fatalf("Asked = %+v", s.Asked)
	}
}

func TestStubWithoutPathsCancels(t *testing.T) {
	s := &StubFilePicker{}
	if _, err := s.OpenFile(FileDialog{}); !errors.Is(err, ErrCanceled) {
		t.Fatalf("OpenFile err = %v, want ErrCanceled", err)
	}
	if _, err := s.OpenFiles(FileDialog{}); !errors.Is(err, ErrCanceled) {
		t.Fatalf("OpenFiles err = %v, want ErrCanceled", err)
	}
}

func TestStubErrWins(t *testing.T) {
	boom := errors.New("boom")
	s := &StubFilePicker{Paths: []string{"/a"}, Err: boom}
	if _, err := s.SaveFile(FileDialog{}); !errors.Is(err, boom) {
		t.Fatalf("SaveFile err = %v, want boom", err)
	}
}

func TestNativeFilePickerIsThePlatforms(t *testing.T) {
	if _, ok := NativeFilePicker().(nativePicker); !ok {
		t.Fatalf("NativeFilePicker() = %T, want nativePicker", NativeFilePicker())
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
