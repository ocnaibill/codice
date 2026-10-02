package graph

import (
	"strings"
	"unicode/utf8"
)

// MaxLinkName is the longest name a [[link]] can hold: the longest name a concept has.
const MaxLinkName = 120

// maxLinkText bounds everything between the brackets, name and shown text together.
const maxLinkText = 300

// Link is one [[Name]] or [[Name|shown text]] in the text of a note.
type Link struct {
	Name  string // what it points to, as written (trimmed): a concept's name or alias, read by Key
	Label string // what is shown: the text after the bar, or the name
	Start int    // byte offsets of the whole "[[...]]" in the text: Start is the "[", End is just past the last "]"
	End   int
}

// Links finds the [[links]] in the text of a note (Markdown), in order. They are written by the person, so the text is
// never changed: this only reads it. What is not a link:
//   - a "[[" with no "]]" on the same line, or with a bracket inside, or with nothing but spaces or punctuation as the
//     name, or a name longer than MaxLinkName;
//   - one inside a code span or a fenced code block (``` or ~~~), where text is just text, or inside a formula
//     ($$...$$ in a line, or a block between two lines of $$), which is read by something else (#21, DEC-111);
//   - one whose first bracket is escaped with a backslash ("\[[Name]]").
//
// A block of code that is only indented is not told apart: it can sit in a list, where indentation is not code.
//
// The client reads the same way (frontend/src/features/notes/wikilinks.js) and both are tested against the same cases
// (testdata/wikilinks.json).
func Links(text string) []Link {
	var out []Link
	var fence string // the fence that is open, as it was written ("```", "~~~~"), or ""
	start := -1      // where the paragraph being read began: a paragraph is read as a whole, because a code span can run over its lines
	flush := func(end int) {
		if start >= 0 {
			out = append(out, paragraphLinks(text, start, end)...)
			start = -1
		}
	}
	for pos := 0; pos < len(text); {
		next := len(text)
		if eol := strings.IndexByte(text[pos:], '\n'); eol >= 0 {
			next = pos + eol + 1
		}
		line := text[pos:next]
		switch open := opensFence(line); {
		case fence != "":
			if closesFence(line, fence) {
				fence = ""
			}
		case open != "":
			flush(pos)
			fence = open
		case strings.TrimSpace(line) == "":
			flush(pos)
		case start < 0:
			start = pos
		}
		pos = next
	}
	flush(len(text))
	return out
}

// opensFence says whether a line starts a fenced block (up to three spaces, then three or more ` or ~, or two or more
// $ for a block of formula), and gives the fence. A fence of ` or $ cannot hold its character in its info text: a line
// like "$$x$$" is a formula in the text, not the start of a block.
func opensFence(line string) string {
	s := strings.TrimLeft(line, " ")
	if len(line)-len(s) > 3 || len(s) < 2 || (s[0] != '`' && s[0] != '~' && s[0] != '$') {
		return ""
	}
	n := 0
	for n < len(s) && s[n] == s[0] {
		n++
	}
	min := 3
	if s[0] == '$' {
		min = 2
	}
	if n < min || (s[0] != '~' && strings.IndexByte(s[n:], s[0]) >= 0) {
		return ""
	}
	return s[:n]
}

// closesFence says whether a line closes the open fence: the same character, at least as long, and nothing else.
func closesFence(line, fence string) bool {
	s := strings.TrimLeft(line, " ")
	if len(line)-len(s) > 3 || !strings.HasPrefix(s, fence[:1]) {
		return false
	}
	n := 0
	for n < len(s) && s[n] == fence[0] {
		n++
	}
	return n >= len(fence) && strings.TrimSpace(s[n:]) == ""
}

func isPunct(b byte) bool {
	return (b >= '!' && b <= '/') || (b >= ':' && b <= '@') || (b >= '[' && b <= '`') || (b >= '{' && b <= '~')
}

// paragraphLinks reads text[from:to], which holds no fence and no blank line.
func paragraphLinks(text string, from, to int) []Link {
	var out []Link
	for i := from; i < to; {
		switch c := text[i]; {
		case c == '\\' && i+1 < to && isPunct(text[i+1]):
			i += 2
		case c == '`' || c == '$':
			n := 0
			for i+n < to && text[i+n] == c {
				n++
			}
			// A span ends at the next run of exactly as many of the same character; with none, these are just
			// characters. A single $ is a dollar sign (a price), not a formula: it takes two.
			if c == '`' || n >= 2 {
				if close := spanEnd(text, i+n, to, n, c); close >= 0 {
					i = close
					continue
				}
			}
			i += n
		case c == '[' && i+1 < to && text[i+1] == '[':
			if l, ok := linkAt(text, i, to); ok {
				out = append(out, l)
				i = l.End
			} else {
				i++
			}
		default:
			i++
		}
	}
	return out
}

// spanEnd finds where a span opened by n of the character c (a backtick or a dollar) ends: just past the next run of
// exactly n. -1 if it never does.
func spanEnd(text string, from, to, n int, c byte) int {
	for i := from; i < to; {
		if text[i] != c {
			i++
			continue
		}
		run := 0
		for i+run < to && text[i+run] == c {
			run++
		}
		if run == n {
			return i + run
		}
		i += run
	}
	return -1
}

// linkAt reads a link that starts at text[i], which is "[[".
func linkAt(text string, i, to int) (Link, bool) {
	j := i + 2
	for j < to && text[j] != ']' && text[j] != '[' && text[j] != '\n' {
		j++
	}
	if j+1 >= to || text[j] != ']' || text[j+1] != ']' {
		return Link{}, false
	}
	inner := text[i+2 : j]
	if utf8.RuneCountInString(inner) > maxLinkText {
		return Link{}, false
	}
	name, label := inner, ""
	if bar := strings.IndexByte(inner, '|'); bar >= 0 {
		name, label = inner[:bar], inner[bar+1:]
	}
	name, label = strings.TrimSpace(name), strings.TrimSpace(label)
	if utf8.RuneCountInString(name) > MaxLinkName || Key(name) == "" {
		return Link{}, false
	}
	if label == "" {
		label = name
	}
	return Link{Name: name, Label: label, Start: i, End: j + 2}, true
}
