package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The place inside a chapter, in the two ways it is written (DEC-190). KOReader says it with an xpointer, a path of element names down the
// chapter ("/body/DocFragment[6]/body/div/p[166]/text().0": the sixth item of the reading order, then the 166th p of the first div of its
// body); the reader of the web says it with a CFI, a path of positions ("epubcfi(/6/12!/4/2/332)": the sixth item, then the second child of
// the body, then its 332nd child). Both are read against the chapter itself, so that the same paragraph is found on either side.

// xnode is an element of a chapter, with where it starts (in characters of text, from the start of the chapter).
type xnode struct {
	name   string
	parent *xnode
	kids   []*xnode
	start  int
}

// parseChapter reads the XHTML of a chapter as a tree of elements. It is read as the web does: entities of HTML are known and what is left
// open is closed.
func parseChapter(data []byte) (*xnode, int, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	dec.AutoClose = xml.HTMLAutoClose
	dec.CharsetReader = func(label string, in io.Reader) (io.Reader, error) { return in, nil }
	root := &xnode{name: "#document"}
	cur, pos := root, 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xnode{name: strings.ToLower(t.Name.Local), parent: cur, start: pos}
			cur.kids = append(cur.kids, n)
			cur = n
		case xml.EndElement:
			if cur.parent != nil {
				cur = cur.parent
			}
		case xml.CharData:
			pos += utf8.RuneCount(t)
		}
	}
	if len(root.kids) == 0 {
		return nil, 0, errors.New("no elements")
	}
	return root.kids[0], pos, nil // the html element
}

// position is the place of the node among the children of its parent: from 1, among all the elements (what a CFI counts) or among those
// of its own name (what an xpointer counts).
func (n *xnode) position(sameName bool) int {
	i := 0
	for _, k := range n.parent.kids {
		if !sameName || k.name == n.name {
			i++
		}
		if k == n {
			return i
		}
	}
	return 0
}

// spineItem is an item of the reading order of an EPUB: the href as the OPF writes it (decoded) and where it is in the archive.
type spineItem struct {
	Href string
	Zip  string
}

// readChapter reads and parses the item of the reading order from the archive.
func readChapter(full string, item spineItem) (*xnode, int, error) {
	zr, err := zip.OpenReader(full)
	if err != nil {
		return nil, 0, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == item.Zip {
			rc, err := f.Open()
			if err != nil {
				return nil, 0, err
			}
			defer rc.Close()
			data, err := io.ReadAll(io.LimitReader(rc, 8<<20))
			if err != nil {
				return nil, 0, err
			}
			return parseChapter(data)
		}
	}
	return nil, 0, errors.New("the chapter is not in the archive")
}

var xpointerStep = regexp.MustCompile(`^([A-Za-z][\w.-]*)(?:\[(\d+)\])?$`)

// cfiOfXPointer turns what a device says (the part after DocFragment[N]/) into the CFI that opens the same element in the reader, and how
// far into the chapter it is (by the characters before it). False when the path does not lead to an element of the chapter.
func cfiOfXPointer(html *xnode, total, item int, tail string) (string, float64, bool) {
	steps := strings.Split(strings.Trim(tail, "/"), "/")
	// KOReader writes the places of its highlights with the index of every step, even the first ("body[1]/div[1]/p[7]"); its progress leaves the
	// first out. Both are the same element.
	if len(steps) == 0 || (steps[0] != "body" && steps[0] != "body[1]") {
		return "", 0, false
	}
	var body *xnode
	for _, k := range html.kids {
		if k.name == "body" {
			body = k
			break
		}
	}
	if body == nil {
		return "", 0, false
	}
	cfi := "epubcfi(/6/" + strconv.Itoa(2*item) + "!/" + strconv.Itoa(2*body.position(false))
	cur := body
	for _, step := range steps[1:] {
		if strings.HasPrefix(step, "text(") || strings.HasPrefix(step, "@") {
			break // the text inside the element, or an attribute: the element is the place
		}
		m := xpointerStep.FindStringSubmatch(step)
		if m == nil {
			return "", 0, false
		}
		want := 1
		if m[2] != "" {
			want, _ = strconv.Atoi(m[2])
		}
		var next *xnode
		seen := 0
		for _, k := range cur.kids {
			if k.name == strings.ToLower(m[1]) {
				seen++
				if seen == want {
					next = k
					break
				}
			}
		}
		if next == nil {
			return "", 0, false
		}
		cfi += "/" + strconv.Itoa(2*next.position(false))
		cur = next
	}
	progression := 0.0
	if total > 0 {
		progression = float64(cur.start) / float64(total)
	}
	return cfi + ")", progression, true
}

var (
	cfiWrapper   = regexp.MustCompile(`^epubcfi\((.*)\)$`)
	cfiAssertion = regexp.MustCompile(`\[[^\]]*\]`)
)

// itemOfCFI is the item of the reading order a CFI is in (from 1), and the path inside it; false when it has none.
func itemOfCFI(cfi string) (int, string, bool) {
	m := cfiWrapper.FindStringSubmatch(strings.TrimSpace(cfi))
	if m == nil {
		return 0, "", false
	}
	inner := cfiAssertion.ReplaceAllString(m[1], "")
	spine, inside, found := strings.Cut(inner, "!")
	if !found {
		return 0, "", false
	}
	parts := strings.Split(strings.Trim(spine, "/"), "/")
	if len(parts) < 2 {
		return 0, "", false
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 2 || n%2 != 0 {
		return 0, "", false
	}
	return n / 2, inside, true
}

// xpointerOfCFI turns the path inside a chapter (what follows the "!" of a CFI) into what a device understands: the path of names down to
// the element, with no text inside it (the element is the place). False when it does not lead to an element of the chapter.
func xpointerOfCFI(html *xnode, item int, inside string) (string, bool) {
	inside = strings.SplitN(strings.SplitN(inside, ":", 2)[0], "~", 2)[0]
	steps := strings.Split(strings.Trim(inside, "/"), "/")
	cur := html
	var names []string
	for i, s := range steps {
		k, err := strconv.Atoi(s)
		if err != nil || k < 1 {
			return "", false
		}
		if k%2 != 0 { // an odd step is a piece of text: the place is the element that holds it
			break
		}
		if k/2 > len(cur.kids) {
			return "", false
		}
		cur = cur.kids[k/2-1]
		if i == 0 && cur.name != "body" {
			return "", false
		}
		if i > 0 {
			name := cur.name
			if n := cur.position(true); n > 1 {
				name += "[" + strconv.Itoa(n) + "]"
			}
			names = append(names, name)
		}
	}
	if len(steps) == 0 || cur == html {
		return "", false
	}
	out := "/body/DocFragment[" + strconv.Itoa(item) + "]/body"
	if len(names) > 0 {
		out += "/" + strings.Join(names, "/")
	}
	return out, true
}
