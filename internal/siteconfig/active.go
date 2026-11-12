package siteconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ActiveFile lists the websites that GET /api/v1/sites returns. It sits in
// the config directory next to sites/ and templates/.
const ActiveFile = "active_sites.json"

// activeList is the content of ActiveFile.
type activeList struct {
	Active []string `json:"active"`
}

// LoadActive reads the active website domains from dir. present is false
// when the file does not exist; an unreadable or malformed file is an error.
func LoadActive(dir string) (domains []string, present bool, err error) {
	b, err := os.ReadFile(filepath.Join(dir, ActiveFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var l activeList
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, true, fmt.Errorf("%s: invalid JSON: %w", ActiveFile, err)
	}
	if l.Active == nil {
		return nil, true, fmt.Errorf(`%s: missing "active" list`, ActiveFile)
	}
	out := make([]string, 0, len(l.Active))
	for _, d := range l.Active {
		if h := NormalizeHost(d); h != "" {
			out = append(out, h)
		}
	}
	return out, true, nil
}
