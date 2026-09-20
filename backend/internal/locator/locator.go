// Package locator is the typed, versioned address of a place inside a publication
// (spec 5.3). A reading position, a bookmark and a note all point somewhere with one.
//
// The address depends on the kind of file: an EPUB page is not a PDF page, and an audio
// position is a time, so a locator says what it is (`type`) and only that kind's fields
// are accepted. What is stored is the canonical form this package writes, never the
// caller's bytes, so an unknown field cannot ride along. The version is the contract's
// own: it changes only if a kind's meaning changes, and old ones stay readable.
package locator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Version is the current locator contract.
const Version = 1

const (
	maxRef     = 1024 // href, CFI, item name
	maxExcerpt = 300
	maxLabel   = 64
)

// Kinds of locator.
const (
	EPUB  = "epub"
	PDF   = "pdf"
	Image = "image" // CBZ, CBR and webtoon: images in a fixed order
	Audio = "audio"
	Text  = "text"
)

// ErrInvalid wraps every rejection, so a handler can answer 400 without reading messages.
var ErrInvalid = errors.New("invalid locator")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// KindFor says which kind of locator addresses a file format, and false for a format the
// reader has no position for.
func KindFor(format string) (string, bool) {
	switch strings.ToLower(format) {
	case "epub":
		return EPUB, true
	case "pdf":
		return PDF, true
	case "cbz", "cbr":
		return Image, true
	case "mp3", "m4a", "m4b", "ogg", "wav", "flac":
		return Audio, true
	case "txt", "md":
		return Text, true
	}
	return "", false
}

// Region is a rectangle inside a page or image, in fractions (0 to 1) of its size, so it
// survives a change of zoom or screen.
type Region struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

func (r *Region) check() error {
	if r == nil {
		return nil
	}
	for _, v := range []float64{r.X, r.Y, r.W, r.H} {
		if !unit(v) {
			return invalid("a region is given in fractions between 0 and 1")
		}
	}
	if r.W == 0 || r.H == 0 || r.X+r.W > 1.000001 || r.Y+r.H > 1.000001 {
		return invalid("the region does not fit inside the page")
	}
	return nil
}

func unit(v float64) bool { return !math.IsNaN(v) && v >= 0 && v <= 1 }

type epub struct {
	Type        string   `json:"type"`
	Href        string   `json:"href,omitempty"`        // the resource inside the book
	CFI         string   `json:"cfi,omitempty"`         // exact place, when the reader gives one
	Progression *float64 `json:"progression,omitempty"` // 0..1 inside the resource
	Excerpt     string   `json:"excerpt,omitempty"`     // a few words there, to notice a changed source
}

type pdf struct {
	Type   string  `json:"type"`
	Page   int     `json:"page"`            // index from 0: not the printed label
	Label  string  `json:"label,omitempty"` // the printed page number, when different
	Region *Region `json:"region,omitempty"`
}

type image struct {
	Type   string   `json:"type"`
	Index  int      `json:"index"`            // position in the natural order, from 0
	Item   string   `json:"item,omitempty"`   // the name inside the archive, to notice a replaced file
	Offset *float64 `json:"offset,omitempty"` // 0..1 down a tall image (webtoon)
	Region *Region  `json:"region,omitempty"`
}

type audio struct {
	Type  string `json:"type"`
	Track int    `json:"track"` // from 0; a single-file book is track 0
	Ms    int64  `json:"ms"`
}

type text struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"` // characters from the start
}

// Validate checks that raw is a locator of the kind that fits the file format and returns
// its canonical JSON. A format the reader cannot address, a different kind, an unknown
// field or an out-of-range value are all ErrInvalid.
func Validate(format string, raw []byte) ([]byte, error) {
	want, ok := KindFor(format)
	if !ok {
		return nil, invalid("files of format %q have no reading position", format)
	}
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, invalid("not a JSON object")
	}
	if head.Type != want {
		return nil, invalid("a %s file takes a %q locator, not %q", strings.ToLower(format), want, head.Type)
	}

	var v any
	switch want {
	case EPUB:
		v = &epub{}
	case PDF:
		v = &pdf{}
	case Image:
		v = &image{}
	case Audio:
		v = &audio{}
	default:
		v = &text{}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return nil, invalid("%v", err)
	}
	if dec.More() {
		return nil, invalid("more than one value")
	}

	switch l := v.(type) {
	case *epub:
		if l.Href == "" && l.CFI == "" {
			return nil, invalid("an epub locator needs an href or a cfi")
		}
		if len(l.Href) > maxRef || len(l.CFI) > maxRef || len(l.Excerpt) > maxExcerpt {
			return nil, invalid("a field is too long")
		}
		if l.Progression != nil && !unit(*l.Progression) {
			return nil, invalid("progression is a fraction between 0 and 1")
		}
	case *pdf:
		if l.Page < 0 || l.Page > 1_000_000 {
			return nil, invalid("page is an index from 0")
		}
		if len(l.Label) > maxLabel {
			return nil, invalid("the page label is too long")
		}
		if err := l.Region.check(); err != nil {
			return nil, err
		}
	case *image:
		if l.Index < 0 || l.Index > 1_000_000 {
			return nil, invalid("index is a position from 0")
		}
		if len(l.Item) > maxRef {
			return nil, invalid("a field is too long")
		}
		if l.Offset != nil && !unit(*l.Offset) {
			return nil, invalid("offset is a fraction between 0 and 1")
		}
		if err := l.Region.check(); err != nil {
			return nil, err
		}
	case *audio:
		if l.Track < 0 || l.Track > 100_000 || l.Ms < 0 || l.Ms > 1000*3600*24*30 {
			return nil, invalid("track and time are out of range")
		}
	case *text:
		if l.Offset < 0 || l.Offset > 1<<31-1 {
			return nil, invalid("offset is a number of characters from 0")
		}
	}
	return json.Marshal(v)
}

// Position is the plain-text form of a locator that the first readers stored and still
// read: the CFI of an EPUB, the page number of a PDF (from 1), the image index of a CBZ
// (from 0) and the seconds of an audio file. It keeps them working while they move over
// to locators. The input is already canonical, as Validate returns it.
func Position(canonical []byte) string {
	var head struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(canonical, &head) != nil {
		return ""
	}
	switch head.Type {
	case EPUB:
		var l epub
		json.Unmarshal(canonical, &l)
		if l.CFI != "" {
			return l.CFI
		}
		return l.Href
	case PDF:
		var l pdf
		json.Unmarshal(canonical, &l)
		return strconv.Itoa(l.Page + 1)
	case Image:
		var l image
		json.Unmarshal(canonical, &l)
		return strconv.Itoa(l.Index)
	case Audio:
		var l audio
		json.Unmarshal(canonical, &l)
		return strconv.FormatFloat(float64(l.Ms)/1000, 'f', -1, 64)
	case Text:
		var l text
		json.Unmarshal(canonical, &l)
		return strconv.Itoa(l.Offset)
	}
	return ""
}
