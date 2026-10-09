package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// starterGroup is a category of the list that is offered to start from, with the terms of its rules and its subcategories (DEC-140). The
// list is only offered: nothing of it exists until someone from the staff chooses it, and the terms are only rules, which do nothing to the
// library until they are applied after a preview.
type starterGroup struct {
	Name     string         `json:"name"`
	Terms    []string       `json:"terms"`
	Children []starterGroup `json:"children,omitempty"`
}

// starterList is in Portuguese, with the terms the files and the providers use in Portuguese, English, Spanish and French. A term that differs from
// another of its category only by accents or case is not there twice: they are compared without them. A term that
// would put a novel in a category of the comics is left out of their subcategories, since a rule cannot look at the format.
var starterList = []starterGroup{
	{Name: "Ficção científica", Terms: []string{"science fiction", "sci-fi", "scifi", "science-fiction", "ficção científica", "ciencia ficción"}},
	{Name: "Fantasia", Terms: []string{"fantasy", "fantasy fiction", "epic fantasy", "high fantasy", "fantasia", "fantastique"}},
	{Name: "Romance", Terms: []string{"romance", "romantic fiction", "love stories", "romance fiction", "romances", "romántica"}},
	{Name: "Terror", Terms: []string{"horror", "horror fiction", "ghost stories", "gothic", "terror", "horreur"}},
	{Name: "Policial e suspense", Terms: []string{"mystery", "mystery fiction", "detective and mystery stories", "detective fiction", "thriller", "thrillers", "suspense", "crime", "crime fiction", "policial", "mistério", "polar"}},
	{Name: "Aventura", Terms: []string{"adventure", "adventure stories", "action & adventure", "aventura", "aventure"}},
	{Name: "Clássicos", Terms: []string{"classics", "classic literature", "literary classics", "clássicos", "clásicos", "classiques"}},
	{Name: "Poesia", Terms: []string{"poetry", "poems", "poesia", "poésie"}},
	{Name: "Biografias", Terms: []string{"biography", "biographies", "biography & autobiography", "autobiography", "memoir", "memoirs", "biografia", "biografias", "memórias"}},
	{Name: "História", Terms: []string{"history", "história", "histoire"}},
	{Name: "Filosofia", Terms: []string{"philosophy", "filosofia", "philosophie"}},
	{Name: "Religião e espiritualidade", Terms: []string{"religion", "religion & spirituality", "spirituality", "christianity", "bible", "religião", "espiritualidade"}},
	{Name: "Ciência", Terms: []string{"science", "natural science", "physics", "chemistry", "biology", "mathematics", "astronomy", "ciência", "física", "química", "biologia", "matemática", "astronomia"}},
	{Name: "Computação", Terms: []string{"computers", "computer science", "computing", "programming", "software", "artificial intelligence", "machine learning", "computação", "programação", "informática", "inteligência artificial"}},
	{Name: "Negócios e economia", Terms: []string{"business", "business & economics", "economics", "finance", "management", "marketing", "negócios", "economia", "finanças", "administração"}},
	{Name: "Autoajuda", Terms: []string{"self-help", "self help", "personal development", "autoajuda", "desenvolvimento pessoal"}},
	{Name: "Artes", Terms: []string{"art", "arts", "music", "design", "photography", "architecture", "cinema", "arte", "artes", "música", "fotografia", "arquitetura"}},
	{Name: "Infantil e juvenil", Terms: []string{"juvenile fiction", "juvenile literature", "children's books", "children", "young adult", "young adult fiction", "infantil", "juvenil", "infanto-juvenil"}},
	{Name: "Quadrinhos", Terms: []string{"comics", "comics & graphic novels", "comic book", "quadrinhos", "histórias em quadrinhos", "hq", "hqs", "bande dessinée"}, Children: []starterGroup{
		{Name: "Super-heróis", Terms: []string{"superhero", "superheroes", "super heroes", "super-heróis", "super heróis", "super-herói"}},
		{Name: "Graphic novels", Terms: []string{"graphic novel", "graphic novels"}},
	}},
	{Name: "Mangá", Terms: []string{"manga", "mangas"}, Children: []starterGroup{
		{Name: "Shounen", Terms: []string{"shounen", "shonen"}},
		{Name: "Shoujo", Terms: []string{"shoujo", "shojo"}},
		{Name: "Seinen", Terms: []string{"seinen"}},
		{Name: "Josei", Terms: []string{"josei"}},
		{Name: "Isekai", Terms: []string{"isekai"}},
		{Name: "Slice of life", Terms: []string{"slice of life", "slice-of-life"}},
	}},
}

// StarterList answers GET /admin/categories/starter: the list that is offered to start from, with what each category would get.
func (h *CategoryRulesHandler) StarterList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": starterList})
}

// CreateFromStarter answers POST /admin/categories/starter {"groups": [names]}: makes the categories of the chosen groups, with their
// subcategories and the terms of their rules. One that already exists (same name in the same place) is used, not made again, so it
// can be asked for again and what is there is kept. It only makes categories and terms: nothing is put in any category.
func (h *CategoryRulesHandler) CreateFromStarter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Groups []string `json:"groups"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req) != nil || len(req.Groups) == 0 {
		http.Error(w, "groups are the categories of the list to make", http.StatusBadRequest)
		return
	}
	// A name asked for twice is made once the first time and found the second: asking again changes nothing.
	chosen := []starterGroup{}
	for _, name := range req.Groups {
		found := false
		for _, g := range starterList {
			if g.Name == name {
				found = true
				chosen = append(chosen, g)
			}
		}
		if !found {
			http.Error(w, "That category is not in the list", http.StatusBadRequest)
			return
		}
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	made, rules := 0, 0
	var place func(g starterGroup, parent *int64) error
	place = func(g starterGroup, parent *int64) error {
		var id int64
		err := tx.QueryRowContext(r.Context(), `SELECT id FROM categories WHERE COALESCE(parent_id, 0) = COALESCE($1::bigint, 0) AND lower(name) = lower($2)`, parent, g.Name).Scan(&id)
		if err == sql.ErrNoRows {
			if err := tx.QueryRowContext(r.Context(), `INSERT INTO categories (parent_id, name, created_by) VALUES ($1, $2, NULLIF($3, '')::uuid) RETURNING id`,
				parent, g.Name, currentUserID(r)).Scan(&id); err != nil {
				return err
			}
			made++
		} else if err != nil {
			return err
		}
		for _, term := range g.Terms {
			res, err := tx.ExecContext(r.Context(), `
				INSERT INTO category_rules (category_id, term, term_key, created_by) VALUES ($1, $2::text, `+termKeySQL("$2::text")+`, NULLIF($3, '')::uuid)
				ON CONFLICT DO NOTHING`, id, term, currentUserID(r))
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				rules += int(n)
			}
		}
		for _, child := range g.Children {
			if err := place(child, &id); err != nil {
				return err
			}
		}
		return nil
	}
	for _, g := range chosen {
		if err := place(g, nil); err != nil {
			log.Println("Error making the categories of the list:", err)
			http.Error(w, "Error making the categories", http.StatusInternalServerError)
			return
		}
	}
	if err := audit.Record(r.Context(), tx, currentUserID(r), "category.starter", "category", "", map[string]any{"groups": strings.Join(req.Groups, ", "), "categories": made, "rules": rules}); err != nil {
		http.Error(w, "Error recording audit entry", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error making the categories", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": made, "rules": rules})
}
