package console

import (
	"io"
	"testing"
)

func TestDecodeKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want key
		n    int
	}{
		{"arrow up", "\x1b[A", key{kind: keyUp}, 3},
		{"arrow down", "\x1b[B", key{kind: keyDown}, 3},
		{"arrow right", "\x1b[C", key{kind: keyRight}, 3},
		{"arrow left", "\x1b[D", key{kind: keyLeft}, 3},
		{"arrow with trailing bytes", "\x1b[Ax", key{kind: keyUp}, 3},
		{"space", " ", key{kind: keySpace}, 1},
		{"carriage return", "\r", key{kind: keyEnter}, 1},
		{"line feed", "\n", key{kind: keyEnter}, 1},
		{"letter a", "a", key{kind: keyRune, r: 'a'}, 1},
		{"letter A normalized", "A", key{kind: keyRune, r: 'a'}, 1},
		{"letter q", "q", key{kind: keyRune, r: 'q'}, 1},
		{"letter Q normalized", "Q", key{kind: keyRune, r: 'q'}, 1},
		{"bare escape", "\x1b", key{kind: keyEsc}, 1},
		{"ctrl-c", "\x03", key{kind: keyCancel}, 1},
		{"split csi prefix is pending", "\x1b[", key{kind: keyPending}, 0},
		{"unknown letter skipped", "z", key{kind: keyNone}, 1},
		{"unknown csi final skipped", "\x1b[Z", key{kind: keyNone}, 3},
		{"escape before other byte is cancel", "\x1bx", key{kind: keyEsc}, 1},
		{"multibyte rune skipped whole", "é", key{kind: keyNone}, 2},
		{"control byte skipped", "\x00", key{kind: keyNone}, 1},
		{"empty buffer", "", key{kind: keyNone}, 0},
	}
	for _, c := range cases {
		got, n := decodeKey([]byte(c.in))
		if got != c.want || n != c.n {
			t.Errorf("%s: decodeKey(%q) = %+v, %d; want %+v, %d", c.name, c.in, got, n, c.want, c.n)
		}
	}
}

// chunkReader serves its data one chunk per Read, mimicking a console that
// delivers a split escape sequence across reads (the pending path).
type chunkReader struct {
	chunks [][]byte
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[0])
	r.chunks = r.chunks[1:]
	return n, nil
}

func TestKeyReaderSplitSequence(t *testing.T) {
	kr := &keyReader{r: &chunkReader{chunks: [][]byte{[]byte("\x1b["), []byte("A")}}}
	k, err := kr.next()
	if err != nil || k.kind != keyUp {
		t.Errorf("split \\x1b[A over reads must decode to up, got %+v, %v", k, err)
	}
}

func TestKeyReaderSkipsIgnorableBytes(t *testing.T) {
	kr := &keyReader{r: &chunkReader{chunks: [][]byte{[]byte("z\x00"), []byte("\x1b[B")}}}
	k, err := kr.next()
	if err != nil || k.kind != keyDown {
		t.Errorf("ignored bytes must be skipped transparently, got %+v, %v", k, err)
	}
}

func TestKeyReaderEOF(t *testing.T) {
	kr := &keyReader{r: &chunkReader{}}
	if _, err := kr.next(); err != io.EOF {
		t.Errorf("exhausted stream must surface EOF, got %v", err)
	}
}
