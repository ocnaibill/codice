package equivalence

import "testing"

func TestChapterNumber_ReadsArabicRomanAndWrittenOutNumbers(t *testing.T) {
	cases := map[string]int{
		"Capítulo 3":      3,
		"CAPÍTULO 12":     12,
		"Chapter III":     3,
		"Capítulo três":   3,
		"Chapter Seven":   7,
		"Capitolo 4":      4,
		"Cap. 5":          5,
		"Chapitre IV":     4,
		"Introdução":      0,
		"Capítulo do Fim": 0, // "Fim" is not a number in any list
		"321":             0, // a bare number is not trusted
		"III":             0, // nor a bare roman numeral
	}
	for title, want := range cases {
		n, ok := ChapterNumber(title)
		if want == 0 {
			if ok {
				t.Errorf("%q: got a number %d, want none", title, n)
			}
			continue
		}
		if !ok || n != want {
			t.Errorf("%q: got %d,%v want %d", title, n, ok, want)
		}
	}
}

func TestSameTitle_IgnoresCaseAccentsAndSpacing(t *testing.T) {
	if !SameTitle("Capítulo Um: O Início", "capitulo um: o inicio") {
		t.Error("same title, different case and accents")
	}
	if SameTitle("Capítulo Um", "Capítulo Dois") {
		t.Error("different titles")
	}
	if SameTitle("", "") {
		t.Error("two empty titles are not a match: neither says anything")
	}
}

func TestEqualCount_NeedsBothEnoughAndEqual(t *testing.T) {
	three := make([]Chapter, 3)
	four := make([]Chapter, 4)
	one := make([]Chapter, 1)
	if !EqualCount(three, make([]Chapter, 3)) {
		t.Error("three and three: equal and enough")
	}
	if EqualCount(three, four) {
		t.Error("different counts")
	}
	if EqualCount(one, one) {
		t.Error("one chapter each proves nothing")
	}
}
