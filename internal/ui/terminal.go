package ui

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Terminal struct {
	tty       *os.File
	savedMode string
}

func NewTerminal() (*Terminal, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	mode, err := runStty(tty, "-g")
	if err != nil {
		_ = tty.Close()
		return nil, err
	}
	terminal := &Terminal{tty: tty, savedMode: strings.TrimSpace(mode)}
	if err := terminal.EnableRaw(); err != nil {
		_ = tty.Close()
		return nil, err
	}
	terminal.EnterAltScreen()
	return terminal, nil
}

func runStty(tty *os.File, args ...string) (string, error) {
	command := exec.Command("stty", args...)
	command.Stdin = tty
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (terminal *Terminal) EnableRaw() error {
	_, err := runStty(terminal.tty, "raw", "-echo", "min", "0", "time", "1")
	return err
}

func (terminal *Terminal) RestoreMode() error {
	if terminal.savedMode == "" {
		return nil
	}
	_, err := runStty(terminal.tty, terminal.savedMode)
	return err
}

func (terminal *Terminal) EnterAltScreen() {
	fmt.Fprint(terminal.tty, "\x1b[?1049h\x1b[2J\x1b[H\x1b[?25l")
}

func (terminal *Terminal) ExitAltScreen() {
	fmt.Fprint(terminal.tty, "\x1b[?25h\x1b[?1049l")
}

func (terminal *Terminal) Close() error {
	terminal.ExitAltScreen()
	_ = terminal.RestoreMode()
	return terminal.tty.Close()
}

func (terminal *Terminal) Write(text string) error {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\n", "\r\n")
	_, err := fmt.Fprint(terminal.tty, normalized)
	return err
}

func (terminal *Terminal) Size() (int, int) {
	output, err := runStty(terminal.tty, "size")
	if err != nil {
		return 40, 120
	}
	var rows int
	var cols int
	if _, err := fmt.Sscanf(strings.TrimSpace(output), "%d %d", &rows, &cols); err != nil {
		return 40, 120
	}
	return rows, cols
}

func (terminal *Terminal) ReadKey() (string, error) {
	buffer := make([]byte, 1)
	count, err := terminal.tty.Read(buffer)
	if err != nil {
		if err == io.EOF {
			return "", nil
		}
		return "", err
	}
	if count == 0 {
		return "", nil
	}
	switch buffer[0] {
	case 13, 10:
		return "enter", nil
	case 27:
		sequence := make([]byte, 2)
		firstCount, err := terminal.tty.Read(sequence[:1])
		if err != nil {
			if err == io.EOF {
				return "esc", nil
			}
			return "esc", nil
		}
		if firstCount == 0 {
			return "esc", nil
		}
		if sequence[0] != '[' {
			return "esc", nil
		}
		secondCount, err := terminal.tty.Read(sequence[1:2])
		if err != nil {
			if err == io.EOF {
				return "esc", nil
			}
			return "esc", nil
		}
		if secondCount == 0 {
			return "esc", nil
		}
		switch sequence[1] {
		case 'A':
			return "up", nil
		case 'B':
			return "down", nil
		default:
			return "esc", nil
		}
	default:
		return string(buffer[0]), nil
	}
}

func (terminal *Terminal) Suspend(action func(*os.File) error) error {
	terminal.ExitAltScreen()
	if err := terminal.RestoreMode(); err != nil {
		return err
	}
	defer func() {
		_ = terminal.EnableRaw()
		terminal.EnterAltScreen()
	}()
	return action(terminal.tty)
}

func (terminal *Terminal) Prompt(label string, initial string, useDefaultOnBlank bool) (string, error) {
	var result string
	err := terminal.Suspend(func(tty *os.File) error {
		reader := bufio.NewReader(tty)
		fmt.Fprintln(tty)
		if initial != "" {
			fmt.Fprintf(tty, "%s [default: %s]: ", label, initial)
		} else {
			fmt.Fprintf(tty, "%s: ", label)
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		value := strings.TrimSpace(line)
		if value == "" && useDefaultOnBlank {
			value = initial
		}
		result = value
		return nil
	})
	return result, err
}

func (terminal *Terminal) Confirm(question string) (bool, error) {
	answer, err := terminal.Prompt(question+" [y/N]", "", false)
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return strings.HasPrefix(answer, "y"), nil
}
