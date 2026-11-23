package console

import "unicode/utf8"

// keyKind classifies one decoded keyboard event from the raw console input
// stream (design D4). keyNone means "bytes consumed but not meaningful"
// (ignored input); keyPending means "incomplete escape prefix, read more".
type keyKind int

const (
	keyNone    keyKind = iota
	keyUp              // arrow up (CSI A)
	keyDown            // arrow down (CSI B)
	keyRight           // arrow right (CSI C)
	keyLeft            // arrow left (CSI D)
	keySpace           // space (multi-select toggle)
	keyEnter           // Enter (\r or \n)
	keyEsc             // bare Escape (cancel)
	keyCancel          // Ctrl+C (0x03, cancel)
	keyRune            // letter command: 'a' (toggle all) or 'q' (quit)
	keyPending         // incomplete "\x1b[" prefix, more bytes needed
)

// key is one decoded input event: the kind plus the command letter for
// keyRune ('a' or 'q', normalized to lowercase).
type key struct {
	kind keyKind
	r    rune
}

// decodeKey decodes the first key from buf and returns it with the number
// of bytes consumed. A return of (keyPending, 0) means buf ends inside an
// escape sequence and the caller must read more bytes and retry; (keyNone,
// n>0) means n bytes were consumed as ignorable input (unknown letter,
// unknown CSI sequence, non-ASCII rune). Only the arrow/enter/esc/ctrl-c
// bytes and the two command letters carry meaning — menus ignore
// everything else rather than erroring, so odd terminals cannot crash a
// menu. Pure function, table-tested offline (design D4).
func decodeKey(buf []byte) (key, int) {
	if len(buf) == 0 {
		return key{kind: keyNone}, 0
	}
	switch buf[0] {
	case 0x1b:
		if len(buf) == 1 {
			return key{kind: keyEsc}, 1
		}
		if buf[1] != '[' {
			// ESC followed by something other than a CSI introducer is a
			// bare cancel (or an Alt-modified letter, which menus do not
			// use): consume only the ESC and let the next byte decode on
			// its own.
			return key{kind: keyEsc}, 1
		}
		if len(buf) == 2 {
			return key{kind: keyPending}, 0
		}
		switch buf[2] {
		case 'A':
			return key{kind: keyUp}, 3
		case 'B':
			return key{kind: keyDown}, 3
		case 'C':
			return key{kind: keyRight}, 3
		case 'D':
			return key{kind: keyLeft}, 3
		default:
			return key{kind: keyNone}, 3 // unknown CSI sequence: skip
		}
	case ' ':
		return key{kind: keySpace}, 1
	case '\r', '\n':
		return key{kind: keyEnter}, 1
	case 0x03:
		return key{kind: keyCancel}, 1
	}
	if buf[0] >= 0x80 {
		// Non-ASCII input: skip the whole UTF-8 rune so multibyte sequences
		// cannot desync the decoder.
		_, size := utf8.DecodeRune(buf)
		return key{kind: keyNone}, size
	}
	switch buf[0] {
	case 'a', 'A':
		return key{kind: keyRune, r: 'a'}, 1
	case 'q', 'Q':
		return key{kind: keyRune, r: 'q'}, 1
	}
	return key{kind: keyNone}, 1
}
