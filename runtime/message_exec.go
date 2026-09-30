package runtime

// The message dialog of a desktop with no toolkit to ask: zenity or
// kdialog, as for the file dialogs.

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// execMessage runs the first dialog program found and returns the button
// pressed, or ErrUnsupported where neither program is on PATH.
func execMessage(m Message) (int, error) {
	if path, err := exec.LookPath("zenity"); err == nil {
		return runMessage(path, zenityMessageArgs(m), m.buttons(), zenityAnswer)
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		return runMessage(path, kdialogMessageArgs(m), m.buttons(), kdialogAnswer)
	}
	return 0, ErrUnsupported
}

// runMessage runs the program and turns its exit code and output into the
// index of the button pressed.
func runMessage(path string, args, buttons []string, answer func(code int, out string, buttons []string) (int, error)) (int, error) {
	cmd := exec.Command(path, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return 0, err
		}
		code = exit.ExitCode()
	}
	return answer(code, strings.TrimSpace(out.String()), buttons)
}

// messageText is what the dialog body says: the headline, and the detail
// below it.
func messageText(m Message) string {
	if m.Detail == "" {
		return m.Title
	}
	return m.Title + "\n\n" + m.Detail
}

// zenityMessageArgs is the command line for a zenity message. One button
// is a plain notice; with more the dialog is a question whose OK and Cancel
// are the first two buttons and whose extra buttons are the rest.
func zenityMessageArgs(m Message) []string {
	buttons := m.buttons()
	verb := map[MessageKind]string{MessageInfo: "--info", MessageWarning: "--warning", MessageError: "--error", MessageQuestion: "--question"}[m.Kind]
	icon := map[MessageKind]string{MessageInfo: "dialog-information", MessageWarning: "dialog-warning", MessageError: "dialog-error", MessageQuestion: "dialog-question"}[m.Kind]
	if len(buttons) > 1 {
		verb = "--question"
	}
	args := []string{verb, "--title=" + m.Title, "--text=" + messageText(m), "--icon-name=" + icon, "--ok-label=" + buttons[0]}
	if len(buttons) > 1 {
		args = append(args, "--cancel-label="+buttons[1])
		for _, b := range buttons[2:] {
			args = append(args, "--extra-button="+b)
		}
	}
	return args
}

// zenityAnswer reads zenity's answer: zero for OK, one for Cancel, or one
// with the label of an extra button on standard output.
func zenityAnswer(code int, out string, buttons []string) (int, error) {
	switch code {
	case 0:
		return 0, nil
	case 1:
		if out != "" {
			for i := 2; i < len(buttons); i++ {
				if buttons[i] == out {
					return i, nil
				}
			}
		}
		if len(buttons) > 1 {
			return 1, nil
		}
	}
	return 0, ErrCanceled
}

// kdialogMessageArgs is the command line for a kdialog message: a notice
// for one button, a yes/no question for two, yes/no/cancel for three or
// more, with the buttons' labels.
func kdialogMessageArgs(m Message) []string {
	buttons := m.buttons()
	switch len(buttons) {
	case 1:
		verb := map[MessageKind]string{MessageInfo: "--msgbox", MessageWarning: "--sorry", MessageError: "--error", MessageQuestion: "--msgbox"}[m.Kind]
		return []string{verb, messageText(m), "--title", m.Title, "--ok-label", buttons[0]}
	case 2:
		verb := "--yesno"
		if m.Kind == MessageWarning || m.Kind == MessageError {
			verb = "--warningyesno"
		}
		return []string{verb, messageText(m), "--title", m.Title, "--yes-label", buttons[0], "--no-label", buttons[1]}
	default:
		verb := "--yesnocancel"
		if m.Kind == MessageWarning || m.Kind == MessageError {
			verb = "--warningyesnocancel"
		}
		return []string{verb, messageText(m), "--title", m.Title, "--yes-label", buttons[0], "--no-label", buttons[1], "--cancel-label", buttons[2]}
	}
}

// kdialogAnswer reads kdialog's exit code: zero for yes, one for no, two
// for cancel.
func kdialogAnswer(code int, _ string, buttons []string) (int, error) {
	if code >= 0 && code < len(buttons) && code <= 2 {
		return code, nil
	}
	return 0, ErrCanceled
}
