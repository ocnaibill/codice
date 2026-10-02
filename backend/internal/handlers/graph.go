package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	"github.com/ocnaibill/codice/backend/internal/graph"
)

// GraphHandler serves a person's manual graph (#83, DEC-109): their concepts and the relations they draw between works,
// concepts and notes. All of it is personal (RN-006): everything is scoped to the caller, and what belongs to someone
// else is not found, never forbidden, so that it is not even known to exist.
type GraphHandler struct {
	DB *sql.DB
}

const (
	maxConceptName  = 120
	maxDescription  = 4000
	maxAliases      = 20
	maxComment      = 1000
	maxConcepts     = 5000
	maxRelations    = 50000
	maxGraphList    = 1000
	labelExcerpt    = 80
	stateAvailable  = "available"
	stateRetired    = "retired"
	stateDeleted    = "deleted"
	errNoNode       = "Não achei um dos lados da relação."
	errSameRelation = "Essa relação já existe."
	errFromWikilink = "Essa relação vem de um [[link]] no texto de uma nota: para mudá-la, mude o texto da nota."
)

// Types is the list of the types of relation: what each says from both ends and between which kinds of node it can be
// drawn. The client shows what a person may pick from it.
func (h *GraphHandler) Types(w http.ResponseWriter, r *http.Request) {
	type pair struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	type item struct {
		Key       string `json:"key"`
		Label     string `json:"label"`
		Inverse   string `json:"inverse,omitempty"`
		Symmetric bool   `json:"symmetric"`
		Pairs     []pair `json:"pairs"`
	}
	out := []item{}
	for _, t := range graph.Types {
		it := item{Key: t.Key, Label: t.Label, Inverse: t.Inverse, Symmetric: t.Symmetric, Pairs: []pair{}}
		for _, p := range t.Pairs {
			it.Pairs = append(it.Pairs, pair{p.Source, p.Target})
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": out})
}

// --- concepts ---

// ConceptOut is a concept as it is shown.
type ConceptOut struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Aliases       []string  `json:"aliases"`
	RelationCount int       `json:"relationCount"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type conceptRequest struct {
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	Aliases     *[]string `json:"aliases"`
}

// cleanConcept makes what a person typed fit to keep. A nil field is left as it was (for an update).
func cleanConcept(req conceptRequest) (name, description *string, aliases *[]string, problem string) {
	if req.Name != nil {
		v, err := cleanText(*req.Name)
		v = strings.Join(strings.Fields(v), " ")
		if err != nil || v == "" || graph.Key(v) == "" {
			return nil, nil, nil, "O conceito precisa de um nome."
		}
		if utf8.RuneCountInString(v) > maxConceptName {
			return nil, nil, nil, "O nome do conceito tem no máximo 120 caracteres."
		}
		name = &v
	}
	if req.Description != nil {
		v, err := cleanText(*req.Description)
		if err != nil {
			return nil, nil, nil, "A descrição não é um texto válido."
		}
		if utf8.RuneCountInString(v) > maxDescription {
			return nil, nil, nil, "A descrição tem no máximo 4000 caracteres."
		}
		description = &v
	}
	if req.Aliases != nil {
		out := []string{}
		seen := map[string]bool{}
		for _, a := range *req.Aliases {
			v, err := cleanText(a)
			v = strings.Join(strings.Fields(v), " ")
			if err != nil || v == "" {
				continue
			}
			key := graph.Key(v)
			if key == "" || seen[key] {
				continue
			}
			if utf8.RuneCountInString(v) > maxConceptName {
				return nil, nil, nil, "Cada apelido tem no máximo 120 caracteres."
			}
			seen[key] = true
			out = append(out, v)
		}
		if len(out) > maxAliases {
			return nil, nil, nil, "No máximo 20 apelidos por conceito."
		}
		aliases = &out
	}
	return name, description, aliases, ""
}

// saveKeys makes the names a concept answers to (its name and its aliases) the keys of the person's concepts. It
// reports the id of another concept that already answers to one of them, when there is one.
func saveKeys(r *http.Request, tx *sql.Tx, user string, concept int64, name string, aliases []string) (taken int64, err error) {
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM concept_keys WHERE concept_id = $1`, concept); err != nil {
		return 0, err
	}
	for _, n := range append([]string{name}, aliases...) {
		res, err := tx.ExecContext(r.Context(),
			`INSERT INTO concept_keys (user_id, key, concept_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, user, graph.Key(n), concept)
		if err != nil {
			return 0, err
		}
		if inserted, _ := res.RowsAffected(); inserted == 0 {
			// The keys of this concept were taken out first, so the one that holds it now is another concept's.
			var other int64
			if err := tx.QueryRowContext(r.Context(), `SELECT concept_id FROM concept_keys WHERE user_id = $1 AND key = $2`, user, graph.Key(n)).Scan(&other); err != nil {
				return 0, err
			}
			return other, nil
		}
	}
	return 0, nil
}

// conceptKeys is what a concept's name and aliases read as.
func conceptKeys(name string, aliases []string) []string {
	keys := []string{graph.Key(name)}
	for _, a := range aliases {
		keys = append(keys, graph.Key(a))
	}
	return keys
}

func idParam(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

const conceptColumns = `c.id, c.name, c.description, c.aliases, c.created_at, c.updated_at,
	(SELECT count(*) FROM relations x WHERE x.user_id = c.user_id AND ((x.source_kind = 'concept' AND x.source_id = c.id) OR (x.target_kind = 'concept' AND x.target_id = c.id)))`

func scanConcept(row interface{ Scan(...any) error }) (ConceptOut, error) {
	var c ConceptOut
	var aliases pq.StringArray
	err := row.Scan(&c.ID, &c.Name, &c.Description, &aliases, &c.CreatedAt, &c.UpdatedAt, &c.RelationCount)
	c.Aliases = append([]string{}, aliases...)
	return c, err
}

func (h *GraphHandler) concept(r *http.Request, id int64) (ConceptOut, error) {
	return scanConcept(h.DB.QueryRowContext(r.Context(), `SELECT `+conceptColumns+` FROM concepts c WHERE c.id = $1 AND c.user_id = $2`, id, currentUserID(r)))
}

// CreateConcept keeps a new concept of the person.
func (h *GraphHandler) CreateConcept(w http.ResponseWriter, r *http.Request) {
	var req conceptRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || req.Name == nil {
		http.Error(w, "O conceito precisa de um nome.", http.StatusBadRequest)
		return
	}
	name, description, aliases, problem := cleanConcept(req)
	if problem != "" {
		http.Error(w, problem, http.StatusBadRequest)
		return
	}
	user := currentUserID(r)
	var count int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM concepts WHERE user_id = $1`, user).Scan(&count); err != nil {
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	if count >= maxConcepts {
		http.Error(w, "Você chegou ao limite de conceitos.", http.StatusConflict)
		return
	}
	if aliases == nil {
		aliases = &[]string{}
	}
	desc := ""
	if description != nil {
		desc = *description
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(r.Context(), `INSERT INTO concepts (user_id, name, description, aliases) VALUES ($1, $2, $3, $4) RETURNING id`,
		user, *name, desc, pq.Array(*aliases)).Scan(&id); err != nil {
		log.Println("Error creating concept:", err)
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	taken, err := saveKeys(r, tx, user, id, *name, *aliases)
	if err != nil {
		log.Println("Error keeping the names of a concept:", err)
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	if taken != 0 {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "Já existe um conceito com esse nome ou apelido.", "conceptId": taken})
		return
	}
	// The notes that already cited one of its names (a pending [[link]]) now point to it.
	if err := resyncNotesCiting(r.Context(), tx, user, conceptKeys(*name, *aliases)); err != nil {
		log.Println("Error linking notes to a concept:", err)
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	c, err := h.concept(r, id)
	if err != nil {
		http.Error(w, "Error reading the concept", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// ListConcepts lists the person's concepts by name; q keeps those whose name or an alias begins with it.
func (h *GraphHandler) ListConcepts(w http.ResponseWriter, r *http.Request) {
	query := `SELECT ` + conceptColumns + ` FROM concepts c WHERE c.user_id = $1`
	args := []any{currentUserID(r)}
	if raw := r.URL.Query().Get("q"); raw != "" {
		// A key has only letters, digits and spaces, so it has nothing the database would take for a wildcard. A search
		// of nothing but punctuation finds nothing.
		q := graph.Key(raw)
		if q == "" {
			writeJSON(w, http.StatusOK, map[string]any{"data": []ConceptOut{}})
			return
		}
		query += ` AND EXISTS (SELECT 1 FROM concept_keys k WHERE k.concept_id = c.id AND k.key LIKE $2 || '%')`
		args = append(args, q)
	}
	rows, err := h.DB.QueryContext(r.Context(), query+` ORDER BY lower(c.name), c.id LIMIT 1000`, args...)
	if err != nil {
		log.Println("Error listing concepts:", err)
		http.Error(w, "Error listing concepts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []ConceptOut{}
	for rows.Next() {
		c, err := scanConcept(rows)
		if err != nil {
			http.Error(w, "Error listing concepts", http.StatusInternalServerError)
			return
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// ResolveConcept finds the concept a name (or an alias) stands for: what a [[Concept]] in a note points to.
func (h *GraphHandler) ResolveConcept(w http.ResponseWriter, r *http.Request) {
	key := graph.Key(r.URL.Query().Get("name"))
	if key == "" {
		http.Error(w, "Concept not found", http.StatusNotFound)
		return
	}
	var id int64
	err := h.DB.QueryRowContext(r.Context(), `SELECT concept_id FROM concept_keys WHERE user_id = $1 AND key = $2`, currentUserID(r), key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Concept not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error reading the concept", http.StatusInternalServerError)
		return
	}
	c, err := h.concept(r, id)
	if err != nil {
		http.Error(w, "Error reading the concept", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// GetConcept is a concept with the relations it is in.
func (h *GraphHandler) GetConcept(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		http.Error(w, "Concept not found", http.StatusNotFound)
		return
	}
	c, err := h.concept(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Concept not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error reading the concept", http.StatusInternalServerError)
		return
	}
	relations, err := h.relationsOf(r, graph.Concept, id)
	if err != nil {
		http.Error(w, "Error reading the relations", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"concept": c, "relations": relations})
}

// UpdateConcept changes the name, the description or the aliases of a concept.
func (h *GraphHandler) UpdateConcept(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		http.Error(w, "Concept not found", http.StatusNotFound)
		return
	}
	var req conceptRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid concept", http.StatusBadRequest)
		return
	}
	name, description, aliases, problem := cleanConcept(req)
	if problem != "" {
		http.Error(w, problem, http.StatusBadRequest)
		return
	}
	user := currentUserID(r)
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var current string
	var currentAliases pq.StringArray
	var currentDescription string
	err = tx.QueryRowContext(r.Context(), `SELECT name, description, aliases FROM concepts WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, user).
		Scan(&current, &currentDescription, &currentAliases)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Concept not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	if name == nil {
		name = &current
	}
	if description == nil {
		description = &currentDescription
	}
	if aliases == nil {
		kept := []string(currentAliases)
		aliases = &kept
	}
	before := conceptKeys(current, currentAliases)
	if _, err := tx.ExecContext(r.Context(), `UPDATE concepts SET name = $1, description = $2, aliases = $3, updated_at = now() WHERE id = $4`,
		*name, *description, pq.Array(*aliases), id); err != nil {
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	taken, err := saveKeys(r, tx, user, id, *name, *aliases)
	if err != nil {
		log.Println("Error keeping the names of a concept:", err)
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	if taken != 0 {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "Já existe um conceito com esse nome ou apelido.", "conceptId": taken})
		return
	}
	// A name it no longer answers to leaves the notes that cite it (they become pending, the text untouched), and one
	// it newly answers to takes the notes that already cited it.
	if err := resyncNotesCiting(r.Context(), tx, user, symmetricDifference(before, conceptKeys(*name, *aliases))); err != nil {
		log.Println("Error linking notes to a concept:", err)
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error saving the concept", http.StatusInternalServerError)
		return
	}
	c, err := h.concept(r, id)
	if err != nil {
		http.Error(w, "Error reading the concept", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// DeleteConcept removes a concept and the relations it is in. It says how many, so that a client can have asked first.
func (h *GraphHandler) DeleteConcept(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		http.Error(w, "Concept not found", http.StatusNotFound)
		return
	}
	c, err := h.concept(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Concept not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Error reading the concept", http.StatusInternalServerError)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM concepts WHERE id = $1 AND user_id = $2`, id, currentUserID(r)); err != nil {
		log.Println("Error deleting concept:", err)
		http.Error(w, "Error deleting the concept", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removedRelations": c.RelationCount})
}

// --- relations ---

// NodeOut is one end of a relation as it is read: what it is called now, or when the relation was drawn if it is no
// longer there, and whether it is still there. A work in the trash is "retired" and comes back with it; one deleted
// for good is "deleted".
type NodeOut struct {
	Kind   string `json:"kind"`
	ID     int64  `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"` // the author of a work, the work a note is from
	State  string `json:"state"`
}

// RelationOut is a relation as it is read.
type RelationOut struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Label     string    `json:"label"`   // how it reads from its source
	Inverse   string    `json:"inverse"` // and from its target (the same label for a type with no direction)
	Symmetric bool      `json:"symmetric"`
	Source    NodeOut   `json:"source"`
	Target    NodeOut   `json:"target"`
	Direction string    `json:"direction,omitempty"` // when asked from a node: "out" if it is the source, "in" if it is the target
	Reads     string    `json:"reads,omitempty"`     // how the relation reads from that node
	Comment   string    `json:"comment"`
	Origin    string    `json:"origin"`
	CreatedAt time.Time `json:"createdAt"`
}

type relationRow struct {
	out          RelationOut
	sourceLabel  []byte
	targetLabel  []byte
	typeOfRecord graph.Type
}

type label struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}

func excerpt(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= labelExcerpt {
		return s
	}
	return string([]rune(s)[:labelExcerpt]) + "…"
}

// nodeLabels reads what the nodes are called now, for the given ids of each kind. A work is "retired" or "deleted" when it is
// not an active one; a note or a concept that is not found has been deleted (its relations go with it, so this is only a race).
func (h *GraphHandler) nodeLabels(r *http.Request, want map[string][]int64) (map[string]map[int64]NodeOut, error) {
	out := map[string]map[int64]NodeOut{graph.Work: {}, graph.Concept: {}, graph.Note: {}}
	user := currentUserID(r)
	if ids := want[graph.Work]; len(ids) > 0 {
		rows, err := h.DB.QueryContext(r.Context(), `
			SELECT w.id, w.original_title, w.retired_at IS NOT NULL,
			       COALESCE((SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id
			                 WHERE c.work_id = w.id AND c.role = 'author' ORDER BY c.position, p.name LIMIT 1), '')
			FROM works w WHERE w.id = ANY($1)`, pq.Array(ids))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var n NodeOut
			var retired bool
			if err := rows.Scan(&n.ID, &n.Label, &retired, &n.Detail); err != nil {
				rows.Close()
				return nil, err
			}
			n.Kind, n.State = graph.Work, stateAvailable
			if retired {
				n.State = stateRetired
			}
			out[graph.Work][n.ID] = n
		}
		rows.Close()
	}
	if ids := want[graph.Concept]; len(ids) > 0 {
		rows, err := h.DB.QueryContext(r.Context(), `SELECT id, name FROM concepts WHERE id = ANY($1) AND user_id = $2`, pq.Array(ids), user)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			n := NodeOut{Kind: graph.Concept, State: stateAvailable}
			if err := rows.Scan(&n.ID, &n.Label); err != nil {
				rows.Close()
				return nil, err
			}
			out[graph.Concept][n.ID] = n
		}
		rows.Close()
	}
	if ids := want[graph.Note]; len(ids) > 0 {
		rows, err := h.DB.QueryContext(r.Context(), `
			SELECT id, COALESCE(NULLIF(body, ''), quote, ''), source_title FROM notes WHERE id = ANY($1) AND user_id = $2`, pq.Array(ids), user)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			n := NodeOut{Kind: graph.Note, State: stateAvailable}
			var text string
			if err := rows.Scan(&n.ID, &text, &n.Detail); err != nil {
				rows.Close()
				return nil, err
			}
			n.Label = excerpt(text)
			out[graph.Note][n.ID] = n
		}
		rows.Close()
	}
	return out, nil
}

const relationColumns = `r.id, r.type, r.source_kind, r.source_id, r.target_kind, r.target_id, r.origin, r.comment, r.source_label, r.target_label, r.created_at`

// readRelations turns rows of relations into what is shown, looking the ends up in as few queries as there are kinds.
func (h *GraphHandler) readRelations(r *http.Request, rows *sql.Rows, from *struct {
	kind string
	id   int64
}) ([]RelationOut, error) {
	defer rows.Close()
	var list []relationRow
	want := map[string][]int64{}
	for rows.Next() {
		var row relationRow
		if err := rows.Scan(&row.out.ID, &row.out.Type, &row.out.Source.Kind, &row.out.Source.ID, &row.out.Target.Kind, &row.out.Target.ID,
			&row.out.Origin, &row.out.Comment, &row.sourceLabel, &row.targetLabel, &row.out.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, row)
		want[row.out.Source.Kind] = append(want[row.out.Source.Kind], row.out.Source.ID)
		want[row.out.Target.Kind] = append(want[row.out.Target.Kind], row.out.Target.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	nodes, err := h.nodeLabels(r, want)
	if err != nil {
		return nil, err
	}
	out := make([]RelationOut, 0, len(list))
	fill := func(n *NodeOut, snapshot []byte) {
		if live, ok := nodes[n.Kind][n.ID]; ok {
			*n = live
			return
		}
		// Gone: what it was called when the relation was drawn.
		var l label
		_ = json.Unmarshal(snapshot, &l)
		n.Label, n.Detail, n.State = l.Label, l.Detail, stateDeleted
	}
	for _, row := range list {
		o := row.out
		fill(&o.Source, row.sourceLabel)
		fill(&o.Target, row.targetLabel)
		if t, ok := graph.TypeByKey(o.Type); ok {
			o.Label, o.Symmetric = t.Label, t.Symmetric
			o.Inverse = t.Inverse
			if t.Symmetric {
				o.Inverse = t.Label
			}
		}
		if from != nil {
			o.Direction, o.Reads = "in", o.Inverse
			if o.Source.Kind == from.kind && o.Source.ID == from.id {
				o.Direction, o.Reads = "out", o.Label
			}
		}
		out = append(out, o)
	}
	return out, nil
}

// relationsOf is every relation a node is in, newest first.
func (h *GraphHandler) relationsOf(r *http.Request, kind string, id int64) ([]RelationOut, error) {
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT `+relationColumns+` FROM relations r
		WHERE r.user_id = $1 AND ((r.source_kind = $2 AND r.source_id = $3) OR (r.target_kind = $2 AND r.target_id = $3))
		ORDER BY r.created_at DESC, r.id DESC LIMIT $4`, currentUserID(r), kind, id, maxGraphList)
	if err != nil {
		return nil, err
	}
	return h.readRelations(r, rows, &struct {
		kind string
		id   int64
	}{kind, id})
}

// ListRelations lists the relations of one node (kind and id), or, with neither, the most recent of the person's graph.
func (h *GraphHandler) ListRelations(w http.ResponseWriter, r *http.Request) {
	kind, rawID := r.URL.Query().Get("kind"), r.URL.Query().Get("id")
	if kind == "" && rawID == "" {
		rows, err := h.DB.QueryContext(r.Context(), `SELECT `+relationColumns+` FROM relations r WHERE r.user_id = $1 ORDER BY r.created_at DESC, r.id DESC LIMIT $2`,
			currentUserID(r), maxGraphList)
		if err != nil {
			http.Error(w, "Error listing relations", http.StatusInternalServerError)
			return
		}
		list, err := h.readRelations(r, rows, nil)
		if err != nil {
			log.Println("Error reading relations:", err)
			http.Error(w, "Error listing relations", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list})
		return
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if !graph.ValidKind(kind) || err != nil || id <= 0 {
		http.Error(w, "Informe o tipo e o número do nó.", http.StatusBadRequest)
		return
	}
	list, err := h.relationsOf(r, kind, id)
	if err != nil {
		log.Println("Error reading relations:", err)
		http.Error(w, "Error listing relations", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

// snapshot is what a node is called now, to be kept on the relation. It is not found when the node is not the caller's, or a
// work that is not active.
func (h *GraphHandler) snapshot(r *http.Request, kind string, id int64) ([]byte, bool, error) {
	nodes, err := h.nodeLabels(r, map[string][]int64{kind: {id}})
	if err != nil {
		return nil, false, err
	}
	n, ok := nodes[kind][id]
	if !ok || (kind == graph.Work && n.State != stateAvailable) {
		return nil, false, nil
	}
	raw, _ := json.Marshal(label{Label: n.Label, Detail: n.Detail})
	return raw, true, nil
}

type relationRequest struct {
	SourceKind string  `json:"sourceKind"`
	SourceID   int64   `json:"sourceId"`
	Type       string  `json:"type"`
	TargetKind string  `json:"targetKind"`
	TargetID   int64   `json:"targetId"`
	Comment    *string `json:"comment"`
}

func cleanComment(c *string) (string, string) {
	if c == nil {
		return "", ""
	}
	v, err := cleanText(*c)
	if err != nil {
		return "", "O comentário não é um texto válido."
	}
	if utf8.RuneCountInString(v) > maxComment {
		return "", "O comentário tem no máximo 1000 caracteres."
	}
	return v, ""
}

// CreateRelation draws a relation between two nodes of the person.
func (h *GraphHandler) CreateRelation(w http.ResponseWriter, r *http.Request) {
	var req relationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid relation", http.StatusBadRequest)
		return
	}
	t, ok := graph.TypeByKey(req.Type)
	if !ok || !graph.ValidKind(req.SourceKind) || !graph.ValidKind(req.TargetKind) || req.SourceID <= 0 || req.TargetID <= 0 {
		http.Error(w, "Tipo de relação ou de nó desconhecido.", http.StatusBadRequest)
		return
	}
	if req.SourceKind == req.TargetKind && req.SourceID == req.TargetID {
		http.Error(w, "Um nó não se relaciona consigo mesmo.", http.StatusBadRequest)
		return
	}
	if !t.Allows(req.SourceKind, req.TargetKind) {
		http.Error(w, "\""+t.Label+"\" não liga "+kindName(req.SourceKind)+" a "+kindName(req.TargetKind)+".", http.StatusBadRequest)
		return
	}
	comment, problem := cleanComment(req.Comment)
	if problem != "" {
		http.Error(w, problem, http.StatusBadRequest)
		return
	}
	user := currentUserID(r)
	var count int
	if err := h.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM relations WHERE user_id = $1`, user).Scan(&count); err != nil {
		http.Error(w, "Error saving the relation", http.StatusInternalServerError)
		return
	}
	if count >= maxRelations {
		http.Error(w, "Você chegou ao limite de relações.", http.StatusConflict)
		return
	}
	source, ok, err := h.snapshot(r, req.SourceKind, req.SourceID)
	if err != nil {
		http.Error(w, "Error saving the relation", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, errNoNode, http.StatusNotFound)
		return
	}
	target, ok, err := h.snapshot(r, req.TargetKind, req.TargetID)
	if err != nil {
		http.Error(w, "Error saving the relation", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, errNoNode, http.StatusNotFound)
		return
	}
	var id int64
	err = h.DB.QueryRowContext(r.Context(), `
		INSERT INTO relations (user_id, source_kind, source_id, type, target_kind, target_id, comment, source_label, target_label)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT DO NOTHING RETURNING id`,
		user, req.SourceKind, req.SourceID, req.Type, req.TargetKind, req.TargetID, comment, source, target).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, errSameRelation, http.StatusConflict)
		return
	}
	if err != nil {
		log.Println("Error creating relation:", err)
		http.Error(w, "Error saving the relation", http.StatusInternalServerError)
		return
	}
	h.respondRelation(w, r, id, http.StatusCreated)
}

func kindName(kind string) string {
	switch kind {
	case graph.Work:
		return "uma obra"
	case graph.Concept:
		return "um conceito"
	}
	return "uma nota"
}

func (h *GraphHandler) respondRelation(w http.ResponseWriter, r *http.Request, id int64, status int) {
	rows, err := h.DB.QueryContext(r.Context(), `SELECT `+relationColumns+` FROM relations r WHERE r.id = $1 AND r.user_id = $2`, id, currentUserID(r))
	if err != nil {
		http.Error(w, "Error reading the relation", http.StatusInternalServerError)
		return
	}
	list, err := h.readRelations(r, rows, nil)
	if err != nil || len(list) != 1 {
		http.Error(w, "Error reading the relation", http.StatusInternalServerError)
		return
	}
	writeJSON(w, status, list[0])
}

// UpdateRelation changes the comment of a relation, or its type (to another that can join the same two kinds).
func (h *GraphHandler) UpdateRelation(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		http.Error(w, "Relation not found", http.StatusNotFound)
		return
	}
	var req struct {
		Type    *string `json:"type"`
		Comment *string `json:"comment"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid relation", http.StatusBadRequest)
		return
	}
	comment, problem := cleanComment(req.Comment)
	if problem != "" {
		http.Error(w, problem, http.StatusBadRequest)
		return
	}
	user := currentUserID(r)
	var sk, tk, current, origin string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT source_kind, target_kind, type, origin FROM relations WHERE id = $1 AND user_id = $2`, id, user).Scan(&sk, &tk, &current, &origin); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Relation not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Error reading the relation", http.StatusInternalServerError)
		return
	}
	if origin == graph.Wikilink {
		http.Error(w, errFromWikilink, http.StatusConflict)
		return
	}
	newType := current
	if req.Type != nil {
		t, ok := graph.TypeByKey(*req.Type)
		if !ok {
			http.Error(w, "Tipo de relação desconhecido.", http.StatusBadRequest)
			return
		}
		if !t.Allows(sk, tk) {
			http.Error(w, "\""+t.Label+"\" não liga "+kindName(sk)+" a "+kindName(tk)+".", http.StatusBadRequest)
			return
		}
		newType = t.Key
	}
	var set []string
	args := []any{id, user}
	if req.Comment != nil {
		args = append(args, comment)
		set = append(set, "comment = $"+strconv.Itoa(len(args)))
	}
	if newType != current {
		args = append(args, newType)
		set = append(set, "type = $"+strconv.Itoa(len(args)))
	}
	if len(set) > 0 {
		res, err := h.DB.ExecContext(r.Context(), `UPDATE relations SET `+strings.Join(set, ", ")+`, updated_at = now() WHERE id = $1 AND user_id = $2`, args...)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			http.Error(w, errSameRelation, http.StatusConflict)
			return
		}
		if err != nil {
			log.Println("Error updating relation:", err)
			http.Error(w, "Error saving the relation", http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, "Relation not found", http.StatusNotFound)
			return
		}
	}
	h.respondRelation(w, r, id, http.StatusOK)
}

// DeleteRelation takes a relation away. What it joined is left as it was.
func (h *GraphHandler) DeleteRelation(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		http.Error(w, "Relation not found", http.StatusNotFound)
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `DELETE FROM relations WHERE id = $1 AND user_id = $2 AND origin <> 'wikilink'`, id, currentUserID(r))
	if err != nil {
		log.Println("Error deleting relation:", err)
		http.Error(w, "Error deleting the relation", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Nothing was deleted: it is not the caller's, or it comes from a link and is not deleted by hand.
		var found bool
		if h.DB.QueryRowContext(r.Context(), `SELECT true FROM relations WHERE id = $1 AND user_id = $2`, id, currentUserID(r)).Scan(&found) == nil {
			http.Error(w, errFromWikilink, http.StatusConflict)
			return
		}
		http.Error(w, "Relation not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}
