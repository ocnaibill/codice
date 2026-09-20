package locator

import (
	"errors"
	"strings"
	"testing"
)

func TestValidate_AcceptsEachKindForItsFormat(t *testing.T) {
	cases := []struct{ format, in, want string }{
		{"epub", `{"type":"epub","cfi":"epubcfi(/6/4!/4/2)","progression":0.25}`, `{"type":"epub","cfi":"epubcfi(/6/4!/4/2)","progression":0.25}`},
		{"EPUB", `{"type":"epub","href":"ch3.xhtml"}`, `{"type":"epub","href":"ch3.xhtml"}`},
		{"pdf", `{"type":"pdf","page":11,"label":"xii","region":{"x":0.1,"y":0.2,"w":0.5,"h":0.3}}`, `{"type":"pdf","page":11,"label":"xii","region":{"x":0.1,"y":0.2,"w":0.5,"h":0.3}}`},
		{"cbz", `{"type":"image","index":0}`, `{"type":"image","index":0}`},
		{"cbr", `{"type":"image","index":4,"offset":0.5}`, `{"type":"image","index":4,"offset":0.5}`},
		{"mp3", `{"type":"audio","track":2,"ms":93500}`, `{"type":"audio","track":2,"ms":93500}`},
		{"md", `{"type":"text","offset":120}`, `{"type":"text","offset":120}`},
		// Whitespace and key order do not matter: the canonical form is what is stored.
		{"pdf", ` { "page" : 3 , "type" : "pdf" } `, `{"type":"pdf","page":3}`},
	}
	for _, c := range cases {
		got, err := Validate(c.format, []byte(c.in))
		if err != nil || string(got) != c.want {
			t.Errorf("%s %s\n got %s, %v\nwant %s", c.format, c.in, got, err, c.want)
		}
	}
}

func TestValidate_RejectsWhatDoesNotBelong(t *testing.T) {
	long := strings.Repeat("a", 2000)
	cases := map[string][2]string{
		"another kind for the format":    {"pdf", `{"type":"epub","cfi":"x"}`},
		"no type":                        {"pdf", `{"page":1}`},
		"format with no position":        {"mobi", `{"type":"epub","cfi":"x"}`},
		"an unknown field":               {"pdf", `{"type":"pdf","page":1,"script":"<b>"}`},
		"a field of another kind":        {"pdf", `{"type":"pdf","page":1,"cfi":"x"}`},
		"epub with nowhere to point":     {"epub", `{"type":"epub"}`},
		"epub field too long":            {"epub", `{"type":"epub","cfi":"` + long + `"}`},
		"progression above 1":            {"epub", `{"type":"epub","href":"a","progression":1.5}`},
		"negative page":                  {"pdf", `{"type":"pdf","page":-1}`},
		"page that is not a number":      {"pdf", `{"type":"pdf","page":"3"}`},
		"fractional page":                {"pdf", `{"type":"pdf","page":1.5}`},
		"region outside the page":        {"pdf", `{"type":"pdf","page":1,"region":{"x":0.8,"y":0,"w":0.5,"h":0.5}}`},
		"empty region":                   {"pdf", `{"type":"pdf","page":1,"region":{"x":0,"y":0,"w":0,"h":0.5}}`},
		"negative time":                  {"mp3", `{"type":"audio","track":0,"ms":-1}`},
		"offset above 1":                 {"cbz", `{"type":"image","index":0,"offset":2}`},
		"not an object":                  {"pdf", `[1,2]`},
		"a second value after the first": {"pdf", `{"type":"pdf","page":1}{"type":"pdf","page":2}`},
		"nothing":                        {"pdf", ``},
	}
	for name, c := range cases {
		if _, err := Validate(c[0], []byte(c[1])); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestPosition_KeepsTheFirstReadersWorking(t *testing.T) {
	cases := map[string]string{
		`{"type":"epub","cfi":"epubcfi(/6/2)"}`: "epubcfi(/6/2)",
		`{"type":"epub","href":"ch1.xhtml"}`:    "ch1.xhtml",
		`{"type":"pdf","page":0}`:               "1", // the PDF viewer counts pages from 1
		`{"type":"pdf","page":41}`:              "42",
		`{"type":"image","index":7}`:            "7", // the comic viewer counts from 0
		`{"type":"audio","track":0,"ms":93500}`: "93.5",
		`{"type":"text","offset":120}`:          "120",
	}
	for in, want := range cases {
		if got := Position([]byte(in)); got != want {
			t.Errorf("%s = %q, want %q", in, got, want)
		}
	}
}
