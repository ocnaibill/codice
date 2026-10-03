package reading

import (
	"errors"
	"strings"
	"testing"
)

func valid() Settings {
	return Settings{Theme: "papel", Font: "livro", Size: 100, Spacing: "livro", Margins: "livro"}
}

func TestValidate_EveryValueOfTheListsIsAccepted(t *testing.T) {
	for _, theme := range Themes {
		for _, font := range Fonts {
			s := valid()
			s.Theme, s.Font = theme, font
			if err := s.Validate(); err != nil {
				t.Errorf("%s/%s: %v", theme, font, err)
			}
		}
	}
	for _, spacing := range Spacings {
		for _, margins := range Margins {
			s := valid()
			s.Spacing, s.Margins, s.Justify = spacing, margins, true
			if err := s.Validate(); err != nil {
				t.Errorf("%s/%s: %v", spacing, margins, err)
			}
		}
	}
	for size := SizeMin; size <= SizeMax; size += SizeStep {
		s := valid()
		s.Size = size
		if err := s.Validate(); err != nil {
			t.Errorf("size %d: %v", size, err)
		}
	}
}

func TestValidate_RefusesWhatIsNotInTheLists(t *testing.T) {
	cases := map[string]func(*Settings){
		"theme":   func(s *Settings) { s.Theme = "rosa" },
		"theme2":  func(s *Settings) { s.Theme = "" },
		"font":    func(s *Settings) { s.Font = "comic-sans" },
		"font2":   func(s *Settings) { s.Font = "Livro" },
		"size":    func(s *Settings) { s.Size = 105 },
		"size2":   func(s *Settings) { s.Size = 70 },
		"size3":   func(s *Settings) { s.Size = 210 },
		"size4":   func(s *Settings) { s.Size = 0 },
		"spacing": func(s *Settings) { s.Spacing = "dupla" },
		"margins": func(s *Settings) { s.Margins = "enorme" },
	}
	for name, change := range cases {
		s := valid()
		change(&s)
		if err := s.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for field, change := range map[string]func(*Settings){
		"theme": func(s *Settings) { s.Theme = "x" }, "font": func(s *Settings) { s.Font = "x" }, "size": func(s *Settings) { s.Size = 1 },
		"spacing": func(s *Settings) { s.Spacing = "x" }, "margins": func(s *Settings) { s.Margins = "x" },
	} {
		s := valid()
		change(&s)
		if err := s.Validate(); err == nil || !strings.HasSuffix(err.Error(), field) {
			t.Errorf("the error of %s says which it is: %v", field, err)
		}
	}
}

func TestParse(t *testing.T) {
	s, err := Parse([]byte(`{"theme":"escuro","font":"dislexia","size":120,"spacing":"ampla","margins":"larga","justify":true}`))
	if err != nil || s != (Settings{Theme: "escuro", Font: "dislexia", Size: 120, Spacing: "ampla", Margins: "larga", Justify: true}) {
		t.Fatalf("%+v %v", s, err)
	}
	for name, raw := range map[string]string{
		"an unknown field": `{"theme":"papel","font":"livro","size":100,"spacing":"livro","margins":"livro","color":"#fff"}`,
		"not JSON":         `nope`,
		"a missing field":  `{"theme":"papel"}`,
		"a number as text": `{"theme":"papel","font":"livro","size":"100","spacing":"livro","margins":"livro"}`,
		"two values":       `{"theme":"papel","font":"livro","size":100,"spacing":"livro","margins":"livro"} {}`,
		"empty":            ``,
		"a list":           `[]`,
	} {
		if _, err := Parse([]byte(raw)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
