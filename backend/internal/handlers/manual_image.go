package handlers

import (
	"bytes"
	"image"
	"image/jpeg"
	"io"
	"net/http"
)

// manualImage is what the staff may send as a picture of the library by hand: the page cuts it to the proportion and the size the place wants,
// and what arrives here is the JPEG already cut, and it is checked, not trusted (DEC-173, DEC-201).
type manualImage struct {
	maxBytes                int
	minWidth, maxWidth      int
	maxHeight               int
	ratioWidth, ratioHeight int // the proportion, width:height
	// the words of the refusals, in the page's language
	tooBig, notJPEG, badSize, badRatio string
}

var (
	// coverImage is the cover of a work or of a series: 2:3, 600×900 when the page cuts it.
	coverImage = manualImage{
		maxBytes: 2 << 20, minWidth: 200, maxWidth: 2000, maxHeight: 3000, ratioWidth: 2, ratioHeight: 3,
		tooBig: "A capa tem até 2 MB.", notJPEG: "A capa deve ser um JPEG.", badSize: "A capa tem de 200 a 2000 pixels de largura.",
		badRatio: "A capa deve ter a proporção 2:3.",
	}
	// bannerImage is the banner of a series: 3:1, 1500×500 when the page cuts it.
	bannerImage = manualImage{
		maxBytes: 3 << 20, minWidth: 600, maxWidth: 3000, maxHeight: 1200, ratioWidth: 3, ratioHeight: 1,
		tooBig: "O fundo tem até 3 MB.", notJPEG: "O fundo deve ser um JPEG.", badSize: "O fundo tem de 600 a 3000 pixels de largura.",
		badRatio: "O fundo deve ter a proporção 3:1.",
	}
)

// ratioOK says whether a picture has the proportion width:height asked, within what rounding a cut in a browser leaves (2%).
func ratioOK(w, h, ratioWidth, ratioHeight int) bool {
	diff := h*ratioWidth - w*ratioHeight
	if diff < 0 {
		diff = -diff
	}
	return diff*50 <= w*ratioHeight
}

// proportionOK says whether a picture is 2:3.
func proportionOK(w, h int) bool { return ratioOK(w, h, 2, 3) }

// readManualJPEG reads the picture of the form field "image" of a request and checks it. When it is not good the answer is already written
// and the second value is false.
func readManualJPEG(w http.ResponseWriter, r *http.Request, spec manualImage) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(spec.maxBytes)+(64<<10))
	if err := r.ParseMultipartForm(int64(spec.maxBytes)); err != nil {
		http.Error(w, spec.tooBig, http.StatusRequestEntityTooLarge)
		return nil, false
	}
	file, _, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Envie a imagem no campo image.", http.StatusBadRequest)
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(spec.maxBytes)+1))
	if err != nil || len(data) > spec.maxBytes {
		http.Error(w, spec.tooBig, http.StatusRequestEntityTooLarge)
		return nil, false
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "jpeg" {
		http.Error(w, spec.notJPEG, http.StatusBadRequest)
		return nil, false
	}
	if cfg.Width < spec.minWidth || cfg.Width > spec.maxWidth || cfg.Height > spec.maxHeight {
		http.Error(w, spec.badSize, http.StatusBadRequest)
		return nil, false
	}
	if !ratioOK(cfg.Width, cfg.Height, spec.ratioWidth, spec.ratioHeight) {
		http.Error(w, spec.badRatio, http.StatusBadRequest)
		return nil, false
	}
	// The whole picture is read once, so that a file that has a good header and a broken body is not kept.
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		http.Error(w, spec.notJPEG, http.StatusBadRequest)
		return nil, false
	}
	return data, true
}
