package runtime

// MessageKind is the tone of a message dialog, which picks its icon.
type MessageKind uint8

// The kinds of message.
const (
	MessageInfo MessageKind = iota
	MessageWarning
	MessageError
	MessageQuestion
)

// Message configures a message dialog: a short headline, the detail below
// it and the buttons to answer with.
type Message struct {
	Kind MessageKind
	// Title is the headline, one short sentence.
	Title string
	// Detail explains, below the headline. It may be empty.
	Detail string
	// Buttons are the answers, the default first; none is a single OK.
	// On Windows the message box has fixed buttons whatever the labels
	// say: one is OK; two are OK and Cancel, or Yes and No for a
	// question; three or more are Yes, No and Cancel.
	Buttons []string
}

// Dialogs opens the platform's dialogs: the file dialogs of FilePicker,
// and message dialogs. NativeDialogs is the platform's; StubFilePicker
// answers from fixed values, for tests.
type Dialogs interface {
	FilePicker
	// Message shows m and returns the index in m.Buttons of the button the
	// user pressed, or ErrCanceled when the dialog was dismissed without
	// one, which only some platforms allow.
	Message(m Message) (int, error)
}

// buttons is m's buttons, or the single OK a message with none has.
func (m Message) buttons() []string {
	if len(m.Buttons) == 0 {
		return []string{"OK"}
	}
	return m.Buttons
}

// NativeDialogs returns the platform's dialogs, attached to no window;
// NativeDialogsForWindow attaches them to one.
func NativeDialogs() Dialogs { return NativeDialogsForWindow(nil) }

// NativeDialogsForWindow binds the dialogs to the caller's native window.
// The getter runs before entering the native main-thread callback.
func NativeDialogsForWindow(window func() uintptr) Dialogs { return nativePicker{window: window} }

// Message implements Dialogs.
func (p nativePicker) Message(m Message) (int, error) {
	var owner uintptr
	if p.window != nil {
		owner = p.window()
	}
	return showMessage(m, owner)
}

// Message implements Dialogs: it records m and answers with Button, or
// Err when set.
func (s *StubFilePicker) Message(m Message) (int, error) {
	s.Messages = append(s.Messages, m)
	if s.Err != nil {
		return 0, s.Err
	}
	return s.Button, nil
}
