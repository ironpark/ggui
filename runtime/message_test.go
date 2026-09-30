package runtime

import (
	"errors"
	"slices"
	"testing"
)

func TestZenityMessageArgs(t *testing.T) {
	got := zenityMessageArgs(Message{Kind: MessageWarning, Title: "Delete?", Detail: "It cannot be undone.", Buttons: []string{"Delete", "Cancel", "Archive"}})
	want := []string{"--question", "--title=Delete?", "--text=Delete?\n\nIt cannot be undone.", "--icon-name=dialog-warning",
		"--ok-label=Delete", "--cancel-label=Cancel", "--extra-button=Archive"}
	if !slices.Equal(got, want) {
		t.Fatalf("zenityMessageArgs = %q, want %q", got, want)
	}
	got = zenityMessageArgs(Message{Kind: MessageError, Title: "Failed"})
	want = []string{"--error", "--title=Failed", "--text=Failed", "--icon-name=dialog-error", "--ok-label=OK"}
	if !slices.Equal(got, want) {
		t.Fatalf("zenityMessageArgs(one) = %q, want %q", got, want)
	}
}

func TestZenityAnswer(t *testing.T) {
	buttons := []string{"Save", "Discard", "Later"}
	for _, c := range []struct {
		code int
		out  string
		want int
		err  error
	}{
		{0, "", 0, nil},
		{1, "", 1, nil},
		{1, "Later", 2, nil},
		{5, "", 0, ErrCanceled},
	} {
		got, err := zenityAnswer(c.code, c.out, buttons)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("zenityAnswer(%d, %q) = %d, %v; want %d, %v", c.code, c.out, got, err, c.want, c.err)
		}
	}
	if _, err := zenityAnswer(1, "", []string{"OK"}); !errors.Is(err, ErrCanceled) {
		t.Errorf("closing a one-button notice = %v, want ErrCanceled", err)
	}
}

func TestKdialogMessage(t *testing.T) {
	got := kdialogMessageArgs(Message{Kind: MessageQuestion, Title: "Quit?", Buttons: []string{"Quit", "Stay"}})
	want := []string{"--yesno", "Quit?", "--title", "Quit?", "--yes-label", "Quit", "--no-label", "Stay"}
	if !slices.Equal(got, want) {
		t.Fatalf("kdialogMessageArgs = %q, want %q", got, want)
	}
	if i, err := kdialogAnswer(2, "", []string{"a", "b", "c"}); i != 2 || err != nil {
		t.Fatalf("kdialogAnswer(cancel) = %d, %v", i, err)
	}
	if _, err := kdialogAnswer(1, "", []string{"a"}); !errors.Is(err, ErrCanceled) {
		t.Fatalf("kdialogAnswer past the buttons = %v", err)
	}
}

func TestStubAnswersMessages(t *testing.T) {
	s := &StubFilePicker{Button: 1}
	var d Dialogs = s
	if i, err := d.Message(Message{Title: "?"}); i != 1 || err != nil {
		t.Fatalf("Message = %d, %v", i, err)
	}
	if len(s.Messages) != 1 || s.Messages[0].Title != "?" {
		t.Fatalf("Messages = %v", s.Messages)
	}
}
