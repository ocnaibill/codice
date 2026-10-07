package textenc

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func write(t *testing.T, content []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "texto.txt")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func utf16Bytes(s string, bigEndian bool, bom bool) []byte {
	var out []byte
	put := func(u uint16) {
		if bigEndian {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	if bom {
		put(0xfeff)
	}
	for _, u := range utf16.Encode([]rune(s)) {
		put(u)
	}
	return out
}

// cp1252 encodes the characters these tests use.
func cp1252(s string) []byte {
	var out []byte
	for _, r := range s {
		switch {
		case r < 0x80:
			out = append(out, byte(r))
		case r >= 0xa0 && r <= 0xff:
			out = append(out, byte(r))
		case r == '€':
			out = append(out, 0x80)
		case r == '“':
			out = append(out, 0x93)
		case r == '”':
			out = append(out, 0x94)
		case r == '—':
			out = append(out, 0x97)
		case r == 'Ÿ':
			out = append(out, 0x9f)
		default:
			panic("not in the table: " + string(r))
		}
	}
	return out
}

const sample = "Coração, ação e emoção: “aspas” e travessão — também €5 e Ÿ.\r\nSegunda linha.\n"

func TestDetect(t *testing.T) {
	cases := map[string]struct {
		content []byte
		want    string
		err     bool
	}{
		"ascii":                      {[]byte("plain ascii\n"), "", false},
		"utf-8":                      {[]byte(sample), "", false},
		"utf-8 with a BOM":           {append([]byte{0xef, 0xbb, 0xbf}, sample...), "", false},
		"empty":                      {nil, "", false},
		"windows-1252":               {cp1252(sample), Windows1252, false},
		"UTF-16 LE with a BOM":       {utf16Bytes(sample, false, true), UTF16LE, false},
		"UTF-16 BE with a BOM":       {utf16Bytes(sample, true, true), UTF16BE, false},
		"UTF-32 LE with a BOM":       {[]byte{0xff, 0xfe, 0, 0, 'a', 0, 0, 0}, "", true},
		"UTF-16 without a BOM":       {utf16Bytes(sample, false, false), "", true},
		"binary":                     {[]byte("abc\x00\x01\x02def"), "", true},
		"a byte windows-1252 lacks":  {append(cp1252("ação "), 0x81), "", true},
		"a control character":        {append(cp1252("ação "), 0x07), "", true},
		"a damaged utf-8":            {append([]byte(sample), 0xe9), "", true},
		"a lone invalid byte, ascii": {[]byte("ca\xe7a"), Windows1252, false},
	}
	for name, c := range cases {
		got, err := Detect(write(t, c.content))
		if got != c.want || (err != nil) != c.err {
			t.Errorf("%s: got %q, %v; want %q, error=%v", name, got, err, c.want, c.err)
		}
	}
}

func TestDetect_EachByteWindows1252LacksIsNotWindows1252(t *testing.T) {
	for _, b := range []byte{0x81, 0x8d, 0x8f, 0x90, 0x9d} {
		if got, err := Detect(write(t, append(cp1252("ação "), b))); got != "" || err == nil {
			t.Errorf("byte %#x: %q, %v; want it refused", b, got, err)
		}
	}
}

func TestDetect_ReadsTheWholeFileAcrossBlocks(t *testing.T) {
	// A UTF-8 character that straddles the 64 KiB block of the reader is not a damaged one.
	body := strings.Repeat("a", 64<<10-1) + "ç" + strings.Repeat("ação ", 5000)
	if got, err := Detect(write(t, []byte(body))); got != "" || err != nil {
		t.Errorf("utf-8 across a block: %q, %v", got, err)
	}
	// A late byte in Windows-1252 is found, after a long run of plain text.
	late := append([]byte(strings.Repeat("plain text ", 20000)), cp1252("final: ação")...)
	if got, err := Detect(write(t, late)); got != Windows1252 || err != nil {
		t.Errorf("a late accent: %q, %v", got, err)
	}
}

func TestConvert(t *testing.T) {
	for name, in := range map[string][]byte{
		"windows-1252": cp1252(sample),
		"UTF-16 LE":    utf16Bytes(sample, false, true),
		"UTF-16 BE":    utf16Bytes(sample, true, true),
	} {
		p := write(t, in)
		from, err := Convert(p)
		if err != nil || from == "" {
			t.Fatalf("%s: %q, %v", name, from, err)
		}
		got, _ := os.ReadFile(p)
		if string(got) != sample {
			t.Errorf("%s: got %q, want %q", name, got, sample)
		}
		if left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), "conv-*")); len(left) != 0 {
			t.Errorf("%s: temporary files left: %v", name, left)
		}
	}
}

func TestConvert_LeavesAlone_WhatIsUTF8_AndWhatItCannotTell(t *testing.T) {
	for name, in := range map[string][]byte{
		"utf-8":          []byte(sample),
		"damaged utf-8":  append([]byte(sample), 0xe9),
		"binary":         []byte("abc\x00def"),
		"a lone control": append(cp1252("ação"), 0x07),
	} {
		p := write(t, in)
		from, _ := Convert(p)
		got, _ := os.ReadFile(p)
		if from != "" || !bytes.Equal(got, in) {
			t.Errorf("%s: converted from %q, bytes changed: %v", name, from, !bytes.Equal(got, in))
		}
	}
}

func TestConvert_ALargeFile(t *testing.T) {
	var in bytes.Buffer
	for in.Len() < 3<<20 {
		in.Write(cp1252(sample))
	}
	p := write(t, in.Bytes())
	if from, err := Convert(p); from != Windows1252 || err != nil {
		t.Fatalf("%q %v", from, err)
	}
	got, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(got), sample) || strings.Count(string(got), "Coração") != strings.Count(string(in.Bytes()), "Cora\xe7\xe3o") {
		t.Errorf("the large file was not converted whole")
	}
}
