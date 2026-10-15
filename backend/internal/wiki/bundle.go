// Package wiki keeps the automatic wiki of a work or a series (DEC-177 to DEC-183, RF-050 to RF-052): the bundle a generator
// writes, how it is stored, and what a reader may see of it from where they are in the books.
//
// Everything a page shows carries the point of reading from which it may be shown (DEC-179). The rule that decides it is
// Cursor.Sees, and every read in this package goes through it: what lies beyond the reader's point is never returned.
package wiki

import (
	"fmt"
	"strings"
	"time"
)

// FormatVersion is the version of the bundle this package reads.
const FormatVersion = 1

// Limits of a bundle: a series of a dozen long books stays well under them.
const (
	maxBooks     = 64
	maxChapters  = 1000
	maxEntities  = 20000
	maxSentences = 200000
	maxEdges     = 400000
	maxText      = 4000
)

// Kinds of entity and groups of the relation box, as stored.
var (
	Kinds  = []string{"person", "place", "organization", "concept", "object", "event", "other"}
	Groups = []string{"family", "mentors", "romance", "allies", "enemies", "places", "things", "others"}
)

// Bundle is one generation of a wiki, as a generator writes it.
type Bundle struct {
	Format      int          `json:"format"`
	GeneratedAt time.Time    `json:"generated_at"`
	Provider    string       `json:"provider"`
	Model       string       `json:"model"`
	Remote      bool         `json:"remote"`  // the text of the books left the server (DEC-177)
	Partial     bool         `json:"partial"` // some step did not finish
	Books       []Book       `json:"books"`
	Entities    []Entity     `json:"entities"`
	Edges       []Edge       `json:"edges"`
	Suggestions []Suggestion `json:"suggestions"`
}

// Book is one book of the project, in reading order. SHA256 is the hash of the file the wiki was written from: it is how the
// bundle finds the book in the library.
type Book struct {
	Position int       `json:"position"`
	Title    string    `json:"title"`
	Side     bool      `json:"side"`
	SHA256   string    `json:"sha256"`
	Chapters []Chapter `json:"chapters"`
}

// Chapter is a chapter as the wiki counts them, from 1. Href is the resource of the EPUB it starts in.
type Chapter struct {
	Chapter   int    `json:"chapter"`
	Title     string `json:"title"`
	Part      string `json:"part"`
	Href      string `json:"href"`
	Editorial bool   `json:"editorial"`
}

// Point is a place of reading: a book of the project (its position) and one of its chapters.
type Point struct {
	Book    int `json:"book"`
	Chapter int `json:"chapter"`
}

type Entity struct {
	Slug        string     `json:"slug"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	Page        bool       `json:"page"`
	Appearances []Point    `json:"appearances"`
	Aliases     []Alias    `json:"aliases"`
	States      []State    `json:"states"`
	Sections    []Section  `json:"sections"`
	Relations   []Relation `json:"relations"`
}

type Alias struct {
	Alias   string `json:"alias"`
	Book    int    `json:"book"`
	Chapter int    `json:"chapter"`
}

type State struct {
	Book    int    `json:"book"`
	Chapter int    `json:"chapter"`
	Text    string `json:"text"`
}

// Section is a written part of a page. who and bio have a book and the chapters of a part; personality has neither.
type Section struct {
	Kind        string     `json:"kind"`
	Book        *int       `json:"book,omitempty"`
	ChapterFrom int        `json:"chapter_from,omitempty"`
	ChapterTo   int        `json:"chapter_to,omitempty"`
	Part        string     `json:"part"`
	Sentences   []Sentence `json:"sentences"`
}

type Sentence struct {
	Paragraph int         `json:"paragraph"`
	Text      string      `json:"text"`
	Refs      []Reference `json:"refs"`
}

type Reference struct {
	Book    int    `json:"book"`
	Chapter int    `json:"chapter"`
	Fact    string `json:"fact"`
	Quote   string `json:"quote,omitempty"`
}

type Relation struct {
	Other   string `json:"other,omitempty"` // the slug, when the other side is an entity of the wiki
	Name    string `json:"name"`
	Group   string `json:"group"`
	Role    string `json:"role"`
	Book    int    `json:"book"`
	Chapter int    `json:"chapter"`
}

type Edge struct {
	A       string `json:"a"`
	B       string `json:"b"`
	Label   string `json:"label"`
	Book    int    `json:"book"`
	Chapter int    `json:"chapter"`
}

// Suggestion is an identity the generator could not prove (DEC-181).
type Suggestion struct {
	Kind     string `json:"kind"` // merge or split
	Entity   string `json:"entity"`
	Other    string `json:"other,omitempty"`
	Evidence string `json:"evidence"`
	Book     int    `json:"book"`
	Chapter  int    `json:"chapter"`
}

func in(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Validate checks that the bundle is whole: every point names a chapter of a book of the bundle, every slug an entity, every
// kind and group a known one. A bundle that fails is rejected as a whole; nothing of it is stored.
func (b *Bundle) Validate() error {
	if b.Format != FormatVersion {
		return fmt.Errorf("formato %d desconhecido (esperado %d)", b.Format, FormatVersion)
	}
	if strings.TrimSpace(b.Provider) == "" || strings.TrimSpace(b.Model) == "" {
		return fmt.Errorf("faltam o provedor e o modelo que geraram a wiki")
	}
	if b.GeneratedAt.IsZero() {
		return fmt.Errorf("falta a data da geração")
	}
	if len(b.Books) == 0 || len(b.Books) > maxBooks {
		return fmt.Errorf("número de livros fora do limite: %d", len(b.Books))
	}
	chapters := map[int]map[int]bool{}
	for i, bk := range b.Books {
		if bk.Position != i {
			return fmt.Errorf("os livros devem vir em ordem, com posições 0, 1, 2…: o %dº tem posição %d", i+1, bk.Position)
		}
		if len(bk.SHA256) != 64 {
			return fmt.Errorf("o livro %q não tem o hash do arquivo", bk.Title)
		}
		if len(bk.Chapters) == 0 || len(bk.Chapters) > maxChapters {
			return fmt.Errorf("o livro %q tem %d capítulos", bk.Title, len(bk.Chapters))
		}
		chapters[i] = map[int]bool{}
		for _, c := range bk.Chapters {
			if c.Chapter < 1 || chapters[i][c.Chapter] {
				return fmt.Errorf("o livro %q repete ou numera mal o capítulo %d", bk.Title, c.Chapter)
			}
			chapters[i][c.Chapter] = true
		}
	}
	point := func(where string, book, chapter int) error {
		if !chapters[book][chapter] {
			return fmt.Errorf("%s aponta para um capítulo que não existe (livro %d, capítulo %d)", where, book, chapter)
		}
		return nil
	}
	if len(b.Entities) > maxEntities {
		return fmt.Errorf("entidades demais: %d", len(b.Entities))
	}
	slugs := map[string]bool{}
	for _, e := range b.Entities {
		if e.Slug == "" || len(e.Slug) > 160 || slugs[e.Slug] {
			return fmt.Errorf("identificador de entidade vazio, longo demais ou repetido: %q", e.Slug)
		}
		slugs[e.Slug] = true
	}
	sentences := 0
	for _, e := range b.Entities {
		where := "a entidade " + e.Slug
		if strings.TrimSpace(e.Name) == "" || !in(Kinds, e.Kind) {
			return fmt.Errorf("%s não tem nome ou tem um tipo desconhecido (%q)", where, e.Kind)
		}
		if len(e.Appearances) == 0 {
			return fmt.Errorf("%s não aparece em nenhum capítulo", where)
		}
		for _, p := range e.Appearances {
			if err := point(where, p.Book, p.Chapter); err != nil {
				return err
			}
		}
		for _, a := range e.Aliases {
			if err := point(where+", apelido "+a.Alias, a.Book, a.Chapter); err != nil {
				return err
			}
		}
		for _, s := range e.States {
			if err := point(where+", situação", s.Book, s.Chapter); err != nil {
				return err
			}
			if len(s.Text) > maxText {
				return fmt.Errorf("%s tem uma situação longa demais", where)
			}
		}
		for _, sec := range e.Sections {
			switch sec.Kind {
			case "who", "bio":
				if sec.Book == nil || sec.ChapterFrom < 1 || sec.ChapterTo < sec.ChapterFrom {
					return fmt.Errorf("%s tem uma seção %s sem livro e capítulos", where, sec.Kind)
				}
				if err := point(where+", seção "+sec.Kind, *sec.Book, sec.ChapterTo); err != nil {
					return err
				}
			case "personality":
				if sec.Book != nil {
					return fmt.Errorf("%s: personalidade não pertence a um livro", where)
				}
			default:
				return fmt.Errorf("%s tem uma seção de tipo desconhecido: %q", where, sec.Kind)
			}
			for _, st := range sec.Sentences {
				sentences++
				if strings.TrimSpace(st.Text) == "" || len(st.Text) > maxText {
					return fmt.Errorf("%s tem uma frase vazia ou longa demais", where)
				}
				if len(st.Refs) == 0 {
					return fmt.Errorf("%s tem uma frase sem referência (DEC-178)", where)
				}
				for _, r := range st.Refs {
					if err := point(where+", referência", r.Book, r.Chapter); err != nil {
						return err
					}
				}
			}
		}
		for _, r := range e.Relations {
			if !in(Groups, r.Group) {
				return fmt.Errorf("%s tem uma relação de grupo desconhecido: %q", where, r.Group)
			}
			if r.Other != "" && !slugs[r.Other] {
				return fmt.Errorf("%s se relaciona com uma entidade que não está no pacote: %q", where, r.Other)
			}
			if err := point(where+", relação", r.Book, r.Chapter); err != nil {
				return err
			}
		}
	}
	if sentences > maxSentences {
		return fmt.Errorf("frases demais: %d", sentences)
	}
	if len(b.Edges) > maxEdges {
		return fmt.Errorf("relações demais no grafo: %d", len(b.Edges))
	}
	for _, e := range b.Edges {
		if !slugs[e.A] || !slugs[e.B] || e.A == e.B {
			return fmt.Errorf("uma relação do grafo liga entidades que não estão no pacote: %q, %q", e.A, e.B)
		}
		if err := point("uma relação do grafo", e.Book, e.Chapter); err != nil {
			return err
		}
	}
	for _, s := range b.Suggestions {
		if (s.Kind != "merge" && s.Kind != "split") || !slugs[s.Entity] || (s.Other != "" && !slugs[s.Other]) {
			return fmt.Errorf("sugestão inválida: %q entre %q e %q", s.Kind, s.Entity, s.Other)
		}
		if err := point("uma sugestão", s.Book, s.Chapter); err != nil {
			return err
		}
	}
	return nil
}
