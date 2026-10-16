package handlers

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // the formats a cover is kept in, read by image.Decode
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Reading a book on a small screen over a home network does not need the cover at the size it was kept: a list of two hundred books
// would load two hundred pictures of several hundred kilobytes. A cover asked with ?w= comes at that width, as a JPEG.

// CoverThumbnailWidths are the widths a cover can be asked at. A fixed list, so that nobody can fill the disk with one picture at every
// size; 240 is what the catalog (OPDS) points e-readers at.
var CoverThumbnailWidths = []int{120, 240, 480}

const (
	thumbQuality   = 82
	maxThumbSource = 40_000_000 // pixels: more than this is not decoded in the server's memory
	maxThumbBytes  = 25 << 20   // of the file
)

// CoverThumbnailHandler serves the covers at a reduced width. Without ?w= it is the handler of the covers, untouched.
type CoverThumbnailHandler struct {
	Next   http.Handler // the covers as they are (FilesHandler)
	Root   string       // where the covers are
	Cache  string       // where the reduced ones are kept, so that they are made once; outside the covers folder, which is part of the backup
	Lookup FileLookup
	// CacheHeader is the Cache-Control of the answer.
	CacheHeader string
}

func (h *CoverThumbnailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("w")
	if raw == "" {
		h.Next.ServeHTTP(w, r)
		return
	}
	width, err := strconv.Atoi(raw)
	if err != nil || !allowedThumbnailWidth(width) {
		http.Error(w, "w is one of "+widthsText(), http.StatusBadRequest)
		return
	}
	rel, ok := cleanRelPath(wildcardPath(r))
	if !ok {
		http.NotFound(w, r)
		return
	}
	access, err := h.Lookup(r.Context(), rel)
	switch {
	case errors.Is(err, ErrFileNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		http.Error(w, "Error looking up file", http.StatusInternalServerError)
		return
	}
	if access.WorkRetired && !isStaffRequest(r) {
		http.NotFound(w, r)
		return
	}
	source, inside := insideStorage(h.Root, filepath.FromSlash(rel))
	info, err := os.Stat(source)
	if !inside || err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	// A picture that cannot be reduced (a format the server does not read, a broken file, one already smaller than asked) goes as it is:
	// a cover that comes whole is better than none.
	whole := func() { r.URL.RawQuery = ""; h.Next.ServeHTTP(w, r) }
	if info.Size() > maxThumbBytes {
		whole()
		return
	}
	thumb, inside := insideStorage(h.Cache, filepath.Join(strconv.Itoa(width), filepath.FromSlash(rel)+".jpg"))
	if !inside {
		http.NotFound(w, r)
		return
	}
	if made, err := os.Stat(thumb); err != nil || made.ModTime().Before(info.ModTime()) {
		if err := makeThumbnail(source, thumb, width); err != nil {
			whole()
			return
		}
		if made, err = os.Stat(thumb); err != nil {
			whole()
			return
		}
	} else if made.Size() == 0 { // the mark of a picture that was already small enough
		whole()
		return
	}
	f, err := os.Open(thumb)
	if err != nil {
		whole()
		return
	}
	defer f.Close()
	made, _ := f.Stat()
	if h.CacheHeader != "" {
		w.Header().Set("Cache-Control", h.CacheHeader)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeContent(w, r, filepath.Base(thumb), made.ModTime(), f)
}

func allowedThumbnailWidth(w int) bool {
	for _, a := range CoverThumbnailWidths {
		if a == w {
			return true
		}
	}
	return false
}

func widthsText() string {
	var parts []string
	for _, w := range CoverThumbnailWidths {
		parts = append(parts, strconv.Itoa(w))
	}
	return strings.Join(parts, ", ")
}

var errTooSmall = errors.New("the picture is not wider than asked")

// makeThumbnail writes the picture at `source` to `dest`, reduced to `width` and as a JPEG. A picture that is not wider than that leaves an
// empty file at `dest`, the mark that there is nothing to reduce (and an error), so it is not read again.
func makeThumbnail(source, dest string, width int) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width*cfg.Height > maxThumbSource {
		return errors.New("the picture cannot be read")
	}
	if cfg.Width <= width {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err == nil {
			os.WriteFile(dest, nil, 0o644)
		}
		return errTooSmall
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	out := reduce(src, width)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: thumbQuality}); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".thumb-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(buf.Bytes())
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return errors.New("the thumbnail could not be written")
	}
	return os.Rename(tmp.Name(), dest)
}

// reduce scales a picture down to `width`, keeping its proportions, by the average of the pixels each new one covers (a picture that is
// only sampled loses its thin lines, and a cover is mostly lettering). A transparent background is put on white, since a JPEG has none.
func reduce(src image.Image, width int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	height := int(float64(sh)*float64(width)/float64(sw) + 0.5)
	if height < 1 {
		height = 1
	}
	rgba := image.NewRGBA(image.Rect(0, 0, sw, sh))
	draw.Draw(rgba, rgba.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Over)
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		y0, y1 := y*sh/height, (y+1)*sh/height
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < width; x++ {
			x0, x1 := x*sw/width, (x+1)*sw/width
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, n uint32
			for sy := y0; sy < y1; sy++ {
				row := rgba.Pix[sy*rgba.Stride : sy*rgba.Stride+sw*4]
				for sx := x0; sx < x1; sx++ {
					p := row[sx*4 : sx*4+4]
					r += uint32(p[0])
					g += uint32(p[1])
					bl += uint32(p[2])
					n++
				}
			}
			i := y*dst.Stride + x*4
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n), uint8(g/n), uint8(bl/n), 255
		}
	}
	return dst
}
