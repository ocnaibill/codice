// Package version says which build of the Códice this is and where its source is.
package version

import (
	"errors"
	"net/url"
	"strings"
)

// Version is set when the image is built (-ldflags "-X .../version.Version=<commit>"). A binary
// built any other way says "dev".
var Version = "dev"

// DefaultSourceURL is where the source of the upstream Códice is.
const DefaultSourceURL = "https://github.com/ocnaibill/codice"

// SourceURL reads CODICE_SOURCE_URL: where the source of THIS running copy can be had. The AGPL
// gives everyone who uses the service over a network the right to the source of the version
// they are using, so whoever runs a modified copy points this at their own. Empty means the
// upstream repository. Only an http(s) address with a host and no credentials is accepted: it
// is shown to every user as a link, so it is checked at startup instead of at the click.
func SourceURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultSourceURL, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("CODICE_SOURCE_URL must be an http(s) address without credentials, like https://git.example.com/me/codice")
	}
	return u.String(), nil
}
