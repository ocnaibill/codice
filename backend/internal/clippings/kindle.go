package clippings

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The words a Kindle writes in the language of its screen. The file is written by the device in that language, so the kind of each entry and
// the date are read from what is there.
var (
	highlightWord = regexp.MustCompile(`(?i)\b(highlight|destaque|subrayado|surlignement|markierung|evidenziazione|evidenzia)\b`)
	noteWord      = regexp.MustCompile(`(?i)\b(note|nota|anota[cç][aã]o|annotation|notiz|annotazione)\b`)
	bookmarkWord  = regexp.MustCompile(`(?i)\b(bookmark|marcador|signet|lesezeichen|segnalibro|favorito)\b`)
	pageWord      = regexp.MustCompile(`(?i)(?:p[aá]gina|page|seite|pagina)\s+(\d+)`)
	locationWord  = regexp.MustCompile(`(?i)(?:posi[cç][aã]o|location|posici[oó]n|emplacement|position|posizione)\s+(\d+(?:\s*-\s*\d+)?)`)
	clipLimit     = regexp.MustCompile(`(?i)(clipping limit|limite de (?:recortes|trechos)|l[ií]mite de recortes|limite de coupures|abschnittslimit|limite dei ritagli)`)
	timeWord      = regexp.MustCompile(`(\d{1,2}):(\d{2})(?::(\d{2}))?\s*([AaPp][Mm])?`)
	yearWord      = regexp.MustCompile(`\b(20\d{2}|19\d{2})\b`)
)

var months = map[string]time.Month{
	"january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6, "july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12,
	"janeiro": 1, "fevereiro": 2, "março": 3, "marco": 3, "abril": 4, "maio": 5, "junho": 6, "julho": 7, "agosto": 8, "setembro": 9, "outubro": 10, "novembro": 11, "dezembro": 12,
	"enero": 1, "febrero": 2, "mayo": 5, "junio": 6, "julio": 7, "septiembre": 9, "octubre": 10, "noviembre": 11, "diciembre": 12,
	"janvier": 1, "février": 2, "fevrier": 2, "mars": 3, "avril": 4, "mai": 5, "juin": 6, "juillet": 7, "août": 8, "aout": 8, "septembre": 9, "octobre": 10, "décembre": 12, "decembre": 12,
	"januar": 1, "februar": 2, "märz": 3, "maerz": 3, "juni": 6, "juli": 7, "dezember": 12, "oktober": 10,
	"gennaio": 1, "febbraio": 2, "aprile": 4, "maggio": 5, "giugno": 6, "luglio": 7, "settembre": 9, "ottobre": 10, "dicembre": 12,
}

var dayWord = regexp.MustCompile(`(?:^|[^\d:])(\d{1,2})(?:[^\d:]|$)`)

// parseKindleDate reads the date of an entry as a Kindle writes it in any of its languages ("segunda-feira, 5 de outubro de 2026 12:00:00",
// "Monday, October 5, 2026 12:00:00 PM"). The device writes the time of its own clock without a zone, which is kept as UTC. Nil when it is not one.
func parseKindleDate(text string) *time.Time {
	lower := strings.ToLower(text)
	var month time.Month
	for name, m := range months {
		if strings.Contains(lower, name) {
			month = m
			break
		}
	}
	year := yearWord.FindString(text)
	if month == 0 || year == "" {
		return nil
	}
	// the day is the number that is neither the year nor part of the time
	rest := timeWord.ReplaceAllString(yearWord.ReplaceAllString(text, " "), " ")
	d := dayWord.FindStringSubmatch(rest)
	if d == nil {
		return nil
	}
	day, _ := strconv.Atoi(d[1])
	y, _ := strconv.Atoi(year)
	hour, minute, second := 0, 0, 0
	if t := timeWord.FindStringSubmatch(text); t != nil {
		hour, _ = strconv.Atoi(t[1])
		minute, _ = strconv.Atoi(t[2])
		second, _ = strconv.Atoi(t[3])
		switch strings.ToLower(t[4]) {
		case "pm":
			if hour < 12 {
				hour += 12
			}
		case "am":
			if hour == 12 {
				hour = 0
			}
		}
	}
	if day < 1 || day > 31 || hour > 23 || minute > 59 {
		return nil
	}
	at := time.Date(y, month, day, hour, minute, second, 0, time.UTC)
	return &at
}

// splitTitle takes the author out of the first line of an entry: "The Name of the Rose (Eco, Umberto)".
func splitTitle(line string) (title, author string) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
	if strings.HasSuffix(line, ")") {
		depth := 0
		for i := len(line) - 1; i >= 0; i-- {
			switch line[i] {
			case ')':
				depth++
			case '(':
				depth--
				if depth == 0 {
					if title = strings.TrimSpace(line[:i]); title != "" {
						return title, strings.TrimSpace(line[i+1 : len(line)-1])
					}
					return line, ""
				}
			}
		}
	}
	return line, ""
}

// locationRange is "345-346" as the two numbers (the second is the first when there is one).
func locationRange(s string) (int, int) {
	parts := strings.SplitN(strings.ReplaceAll(s, " ", ""), "-", 2)
	a, _ := strconv.Atoi(parts[0])
	b := a
	if len(parts) == 2 {
		if v, err := strconv.Atoi(parts[1]); err == nil {
			b = v
		}
	}
	return a, b
}

// ParseKindle reads a My Clippings.txt. A note a person wrote on a highlight is a separate entry in the file, at the place of the highlight: it
// is joined to it. Bookmarks (a place with nothing in it), clippings the Kindle did not give (a book with a limit of clippings) and repeated
// entries (the Kindle keeps the earlier writing of a highlight that was edited) are counted and left out.
func ParseKindle(text string) Result {
	res := Result{Format: Kindle, Skipped: map[string]int{}}
	text = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(strings.TrimPrefix(text, "\ufeff"))
	type clip struct {
		Entry
		from, to int
	}
	var clips []clip
	for _, block := range strings.Split(text, "==========") {
		lines := strings.Split(strings.Trim(block, "\n"), "\n")
		if len(lines) < 2 {
			continue
		}
		title, author := splitTitle(lines[0])
		meta := lines[1]
		if title == "" || !strings.HasPrefix(strings.TrimSpace(meta), "-") {
			continue
		}
		body := strings.TrimSpace(strings.Join(lines[2:], "\n"))
		kind := ""
		switch {
		case bookmarkWord.MatchString(meta):
			kind = "bookmark"
		case noteWord.MatchString(meta):
			kind = "note"
		case highlightWord.MatchString(meta):
			kind = "highlight"
		}
		if kind == "" || kind == "bookmark" {
			res.Skipped["bookmarks"]++
			continue
		}
		if body == "" {
			res.Skipped["empty"]++
			continue
		}
		if clipLimit.MatchString(body) {
			res.Skipped["limit"]++
			continue
		}
		e := clip{Entry: Entry{Origin: Kindle, Title: title, Author: author, Kind: kind}}
		if m := locationWord.FindStringSubmatch(meta); m != nil {
			e.Location = strings.ReplaceAll(m[1], " ", "")
			e.from, e.to = locationRange(m[1])
		}
		if m := pageWord.FindStringSubmatch(meta); m != nil {
			e.Place = m[1]
		}
		// the date is what follows the last "|"
		if i := strings.LastIndex(meta, "|"); i >= 0 {
			e.At = parseKindleDate(meta[i+1:])
		}
		if kind == "note" {
			e.Note = body
		} else {
			e.Text = body
		}
		clips = append(clips, e)
	}
	// a note written on a highlight is joined to it; a highlight kept again with more text replaces the shorter one at the same place
	var out []clip
	for _, c := range clips {
		joined := false
		if c.Kind == "note" {
			for i := len(out) - 1; i >= 0 && i >= len(out)-8; i-- {
				h := &out[i]
				if h.Kind == "highlight" && GroupKey(h.Title, h.Author) == GroupKey(c.Title, c.Author) && c.from >= h.from-1 && c.from <= h.to+1 {
					h.Note, h.Kind, joined = c.Note, "note", true
					break
				}
			}
		} else {
			for i := len(out) - 1; i >= 0 && i >= len(out)-8; i-- {
				h := &out[i]
				if h.Kind == "highlight" && h.Note == "" && GroupKey(h.Title, h.Author) == GroupKey(c.Title, c.Author) && h.from != 0 && h.from == c.from &&
					(strings.HasPrefix(c.Text, h.Text) || strings.HasPrefix(h.Text, c.Text)) {
					if len(c.Text) > len(h.Text) {
						h.Text, h.to = c.Text, c.to
					}
					joined = true
					res.Skipped["repeated"]++
					break
				}
			}
		}
		if !joined {
			out = append(out, c)
		}
	}
	for _, c := range out {
		res.Entries = append(res.Entries, c.Entry)
	}
	return res
}
