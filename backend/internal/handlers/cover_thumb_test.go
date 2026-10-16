package handlers

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	appMiddleware "github.com/ocnaibill/codice/backend/internal/middleware"
)

// picture is a cover with lettering-like detail: stripes, so that a reduction that only samples would lose them.
func picture(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(40)
			if (x/3+y/3)%2 == 0 {
				v = 215
			}
			img.Set(x, y, color.RGBA{v, v, uint8(x * 255 / w), 255})
		}
	}
	return img
}

type thumbStack struct {
	t      *testing.T
	covers string
	cache  string
	h      http.Handler
	whole  int // how many requests went to the covers as they are
}

func newThumbStack(t *testing.T) *thumbStack {
	t.Helper()
	s := &thumbStack{t: t, covers: t.TempDir(), cache: filepath.Join(t.TempDir(), "cache", "thumbs")}
	lookup := func(ctx context.Context, name string) (FileAccess, error) {
		switch name {
		case "retired.jpg":
			return FileAccess{WorkRetired: true}, nil
		case "big.jpg", "small.jpg", "photo.png", "broken.jpg", "weird.webp":
			return FileAccess{}, nil
		}
		return FileAccess{}, ErrFileNotFound
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.whole++
		(&FilesHandler{Root: s.covers, Lookup: lookup}).ServeHTTP(w, r)
	})
	r := chi.NewRouter()
	r.Method("GET", "/covers/*", &CoverThumbnailHandler{Next: next, Root: s.covers, Cache: s.cache, Lookup: lookup, CacheHeader: "public, max-age=60"})
	s.h = r
	return s
}

func (s *thumbStack) write(name string, data []byte) string {
	s.t.Helper()
	p := filepath.Join(s.covers, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		s.t.Fatal(err)
	}
	return p
}

func thumbJPEG(img image.Image) []byte {
	var b bytes.Buffer
	jpeg.Encode(&b, img, &jpeg.Options{Quality: 95})
	return b.Bytes()
}

func (s *thumbStack) get(target string, role string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", target, nil)
	req = req.WithContext(context.WithValue(req.Context(), appMiddleware.UserRoleKey, role))
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	return rec
}

func thumbConfig(t *testing.T, rec *httptest.ResponseRecorder) image.Config {
	t.Helper()
	cfg, format, err := image.DecodeConfig(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("the answer is not a picture: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("format %s, want jpeg", format)
	}
	return cfg
}

func TestCoverThumbnail_ACoverComesReducedAndKeptForTheNextTime(t *testing.T) {
	s := newThumbStack(t)
	original := thumbJPEG(picture(800, 1200))
	s.write("big.jpg", original)

	rec := s.get("/covers/big.jpg?w=240", "reader")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("%d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if cfg := thumbConfig(t, rec); cfg.Width != 240 || cfg.Height != 360 {
		t.Errorf("size %dx%d, want 240x360 (the proportions are kept)", cfg.Width, cfg.Height)
	}
	if rec.Body.Len() >= len(original)/4 {
		t.Errorf("%d bytes for a picture of %d: not reduced", rec.Body.Len(), len(original))
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=60" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers: %v", rec.Header())
	}
	if _, err := os.Stat(filepath.Join(s.cache, "240", "big.jpg.jpg")); err != nil {
		t.Errorf("the reduced one was not kept: %v", err)
	}
	if s.whole != 0 {
		t.Errorf("the cover as it is was asked for %d times", s.whole)
	}
	// the covers folder, which goes into the backup, is left as it was
	if entries, _ := os.ReadDir(s.covers); len(entries) != 1 {
		t.Errorf("the covers folder has %d files", len(entries))
	}
	// the second time it comes from what was kept, and a cover that is replaced is made again
	again := s.get("/covers/big.jpg?w=240", "reader")
	if again.Code != 200 || !bytes.Equal(again.Body.Bytes(), rec.Body.Bytes()) {
		t.Error("the second answer is not the first")
	}
	src := s.write("big.jpg", thumbJPEG(picture(900, 900)))
	os.Chtimes(src, time.Now().Add(time.Minute), time.Now().Add(time.Minute))
	if cfg := thumbConfig(t, s.get("/covers/big.jpg?w=240", "reader")); cfg.Height != 240 {
		t.Errorf("a replaced cover is still the old picture: %dx%d", cfg.Width, cfg.Height)
	}
	// the other widths of the list, and only those
	if cfg := thumbConfig(t, s.get("/covers/big.jpg?w=120", "reader")); cfg.Width != 120 {
		t.Errorf("w=120: %d", cfg.Width)
	}
	for _, w := range []string{"0", "-1", "300", "abc", "100000", "240.5"} {
		if rec := s.get("/covers/big.jpg?w="+w, "reader"); rec.Code != http.StatusBadRequest {
			t.Errorf("w=%s: %d, want 400", w, rec.Code)
		}
	}
}

func TestCoverThumbnail_WithNoWidthItIsTheCoverAsItIs(t *testing.T) {
	s := newThumbStack(t)
	original := thumbJPEG(picture(800, 1200))
	s.write("big.jpg", original)
	rec := s.get("/covers/big.jpg", "reader")
	if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), original) || s.whole != 1 {
		t.Errorf("%d, %d bytes, whole=%d", rec.Code, rec.Body.Len(), s.whole)
	}
}

func TestCoverThumbnail_APngIsReducedToo_AndWhatCannotBeReducedComesWhole(t *testing.T) {
	s := newThumbStack(t)
	var b bytes.Buffer
	png.Encode(&b, picture(600, 900))
	s.write("photo.png", b.Bytes())
	s.write("small.jpg", thumbJPEG(picture(100, 150)))
	s.write("broken.jpg", []byte("this is not a picture"))
	s.write("weird.webp", []byte("RIFF....WEBPVP8 "))

	if cfg := thumbConfig(t, s.get("/covers/photo.png?w=240", "reader")); cfg.Width != 240 {
		t.Errorf("png: %d wide", cfg.Width)
	}
	for _, name := range []string{"small.jpg", "broken.jpg", "weird.webp"} {
		before := s.whole
		rec := s.get("/covers/"+name+"?w=240", "reader")
		if rec.Code != 200 || s.whole != before+1 {
			t.Errorf("%s: %d, whole %d times (a picture that cannot be reduced goes as it is)", name, rec.Code, s.whole-before)
		}
	}
	// one already narrower is not made again at every request
	if info, err := os.Stat(filepath.Join(s.cache, "240", "small.jpg.jpg")); err != nil || info.Size() != 0 {
		t.Errorf("the mark of a small picture: %v", err)
	}
	if rec := s.get("/covers/small.jpg?w=240", "reader"); rec.Code != 200 || thumbConfig(t, rec).Width != 100 {
		t.Errorf("the small one, the second time: %d", rec.Code)
	}
}

func TestCoverThumbnail_SameRulesOfAccessAsTheCover(t *testing.T) {
	s := newThumbStack(t)
	s.write("retired.jpg", thumbJPEG(picture(800, 1200)))
	s.write("big.jpg", thumbJPEG(picture(800, 1200)))
	if rec := s.get("/covers/retired.jpg?w=240", "reader"); rec.Code != 404 {
		t.Errorf("a retired work's cover to a reader: %d, want 404", rec.Code)
	}
	if rec := s.get("/covers/retired.jpg?w=240", "admin"); rec.Code != 200 {
		t.Errorf("to the staff: %d", rec.Code)
	}
	for _, target := range []string{"/covers/nobody.jpg?w=240", "/covers/..%2Fsecret.jpg?w=240", "/covers/big.jpg/?w=240"} {
		if rec := s.get(target, "reader"); rec.Code != 404 {
			t.Errorf("%s: %d, want 404", target, rec.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(s.cache, "240", "nobody.jpg.jpg")); err == nil {
		t.Error("something was made for a cover nobody has")
	}
}

func TestCoverThumbnail_TheReductionKeepsTheAverageAndAWhiteBehindWhatIsTransparent(t *testing.T) {
	// a picture of two halves, black and white: reduced, the halves are still there and the line between them is not moved
	img := image.NewRGBA(image.Rect(0, 0, 400, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 400; x++ {
			if x < 200 {
				img.Set(x, y, color.Black)
			} else {
				img.Set(x, y, color.White)
			}
		}
	}
	out := reduce(img, 100)
	if r, _, _, _ := out.At(10, 10).RGBA(); r>>8 > 5 {
		t.Errorf("the black half is %d", r>>8)
	}
	if r, _, _, _ := out.At(90, 10).RGBA(); r>>8 < 250 {
		t.Errorf("the white half is %d", r>>8)
	}
	clear := image.NewNRGBA(image.Rect(0, 0, 300, 300)) // fully transparent
	if r, g, b, a := reduce(clear, 100).At(5, 5).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 || a>>8 != 255 {
		t.Errorf("transparent became %d %d %d %d, want white", r>>8, g>>8, b>>8, a>>8)
	}
	if !strings.Contains(widthsText(), "240") {
		t.Error("the list of widths is not said")
	}
}
