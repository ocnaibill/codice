package handlers

import (
	"encoding/json"
	"net/http"
)

// AboutHandler answers what the "Sobre" screen shows about this installation (DEC-120): the
// version, the license and where the source is. It carries nothing about the instance
// itself, no contact and no configuration.
type AboutHandler struct {
	Version   string
	SourceURL string
}

const (
	aboutLicense    = "AGPL-3.0"
	aboutLicenseURL = "https://www.gnu.org/licenses/agpl-3.0.html"
)

func (h *AboutHandler) Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"version":    h.Version,
		"license":    aboutLicense,
		"licenseUrl": aboutLicenseURL,
		"sourceUrl":  h.SourceURL,
	})
}
