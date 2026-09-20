package equivalence

import "testing"

func TestWords_FoldsCaseAndAccentsAndSplitsOnPunctuation(t *testing.T) {
	got := Words("Ação, coração! Nº 12-3... e café-com-leite")
	want := []string{"acao", "coracao", "nº", "12", "3", "e", "cafe", "com", "leite"}
	if !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFold_IsCaseAndAccentInsensitive(t *testing.T) {
	if Fold("Constantinopla") != Fold("CONSTANTINOPLA") || Fold("café") != Fold("CAFE") {
		t.Error("case and accents must not matter")
	}
	if Fold("a") == Fold("b") {
		t.Error("different words must differ")
	}
}

func TestShingles_FindsSharedSequencesAcrossPunctuationAndOrder(t *testing.T) {
	a := Shingles(Words("A cidade de Constantinopla caiu em 1453."), 4)
	b := Shingles(Words("Diziam que a cidade de Constantinopla caiu, enfim, em 1453 aos turcos."), 4)
	n, share := Shared(a, b)
	if n == 0 || share == 0 {
		t.Fatalf("shared sequences across a rewording: n=%d share=%v", n, share)
	}
	unrelated := Shingles(Words("O gato dormiu no sofá da sala inteira a tarde toda."), 4)
	if n, _ := Shared(a, unrelated); n != 0 {
		t.Errorf("unrelated text shares %d sequences", n)
	}
}

func TestShared_IsAFractionOfTheSourceNotOfTheCandidate(t *testing.T) {
	source := Shingles(Words("um dois tres quatro cinco"), 4)
	// the candidate is much longer, but contains the source's sequences
	candidate := Shingles(Words("zero um dois tres quatro cinco seis sete oito nove dez onze doze treze"), 4)
	_, share := Shared(source, candidate)
	if share < 0.9 {
		t.Errorf("share = %v, want close to 1 (all of the short source is in the long candidate)", share)
	}
	_, reverseShare := Shared(candidate, source)
	if reverseShare >= share {
		t.Errorf("the fraction is not symmetric: %v vs %v", share, reverseShare)
	}
}

func TestAnchors_KeepsNamesAndNumbersNotTheFirstWordOfASentence(t *testing.T) {
	got := Anchors("A cidade de Constantinopla caiu em 1453 pelos turcos. Maomé venceu.")
	if _, ok := got["1453"]; !ok {
		t.Errorf("a number is an anchor: %v", got)
	}
	if _, ok := got[Fold("Constantinopla")]; !ok {
		t.Errorf("a name mid-sentence is an anchor: %v", got)
	}
	if _, ok := got[Fold("A")]; ok {
		t.Errorf("the first word of a sentence is not trusted as a name: %v", got)
	}
}

func TestAnchors_LeavesOutAllCapsAndShortWords(t *testing.T) {
	got := Anchors("O LIVRO fala de Roma. A ONU não existia ainda.")
	if _, ok := got[Fold("LIVRO")]; ok {
		t.Errorf("an all-caps word is not a name: %v", got)
	}
	if _, ok := got[Fold("ONU")]; ok {
		t.Errorf("an acronym is not trusted either: %v", got)
	}
	if _, ok := got[Fold("Roma")]; !ok {
		t.Errorf("a real name mid-sentence: %v", got)
	}
}

func TestExcerpt_CutsAtAWordAndMarksIt(t *testing.T) {
	text := "Esta é uma frase relativamente longa para testar o corte no meio de uma palavra e não em outro lugar qualquer."
	got := Excerpt(text, 30)
	if len([]rune(got)) > 32 {
		t.Errorf("excerpt too long: %q", got)
	}
	if got[len(got)-1] == ' ' {
		t.Errorf("must not end in a cut word or trailing space: %q", got)
	}
	if short := Excerpt("curto", 30); short != "curto" {
		t.Errorf("a short text is not touched: %q", short)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
