// SPDX-License-Identifier: TODO

// Package ansi holds the SGR escape sequences robbe prints and the rule for
// when to print them.
package ansi

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/term"
)

// SGR (Select Graphic Rendition) escape sequences. Every sequence other
// than Reset must be closed with Reset on the same line.
const (
	Bold   = "\x1b[1m"
	Dim    = "\x1b[2m"
	Red    = "\x1b[31m"
	Green  = "\x1b[32m"
	Yellow = "\x1b[33m"
	Reset  = "\x1b[0m"
)

// Values of the --color flag.
const (
	// ModeAuto colors only when stdout is a terminal and neither NO_COLOR
	// nor TERM=dumb is set.
	ModeAuto = "auto"
	// ModeAlways colors unconditionally, even when piped or under NO_COLOR.
	ModeAlways = "always"
	// ModeNever prints plain text unconditionally.
	ModeNever = "never"
)

// ErrInvalidMode is returned for a --color value other than auto, always
// or never.
var ErrInvalidMode = errors.New("invalid color mode")

// ValidMode returns nil when mode is one of the three accepted values and
// an error wrapping ErrInvalidMode otherwise.
func ValidMode(mode string) error {
	switch mode {
	case ModeAuto, ModeAlways, ModeNever:
		return nil
	default:
		return fmt.Errorf("%w: %q (want auto, always or never)", ErrInvalidMode, mode)
	}
}

// Enabled reports whether output to f should be colored under mode. In
// auto mode an explicitly set NO_COLOR (any value, per no-color.org),
// TERM=dumb and a non-terminal f each disable color; f may be nil and is
// then treated as not a terminal.
func Enabled(mode string, f *os.File) (bool, error) {
	switch mode {
	case ModeAlways:
		return true, nil
	case ModeNever:
		return false, nil
	case ModeAuto:
		if _, set := os.LookupEnv("NO_COLOR"); set {
			return false, nil
		}

		if os.Getenv("TERM") == "dumb" {
			return false, nil
		}

		return f != nil && term.IsTerminal(int(f.Fd())), nil
	default:
		return false, ValidMode(mode)
	}
}
