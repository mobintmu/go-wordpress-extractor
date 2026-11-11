package apikey

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// File formats. A path ending in .json is read as JSON, anything else as .env.
const (
	FormatEnv  = "env"
	FormatJSON = "json"

	envKeyPrefix = "APIKEY_"
	envHeader    = "# Managed by the apikey command. Contains hashes only.\n"
	neverExpires = "never"
)

// FormatOf returns the file format used for path.
func FormatOf(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return FormatJSON
	}
	return FormatEnv
}

type jsonFile struct {
	Version int     `json:"version"`
	Keys    []Entry `json:"keys"`
}

// Parse reads key entries from data in the given format. Any problem
// rejects the whole file.
func ParseFile(data []byte, format string) ([]Entry, error) {
	var entries []Entry
	var err error
	switch format {
	case FormatJSON:
		entries, err = parseJSON(data)
	default:
		entries, err = parseEnv(data)
	}
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if err := e.Validate(); err != nil {
			return nil, err
		}
		if seen[e.ID] {
			return nil, fmt.Errorf("duplicate key id %q", e.ID)
		}
		seen[e.ID] = true
	}
	return entries, nil
}

func parseJSON(data []byte) ([]Entry, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f jsonFile
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("invalid JSON key file: %w", err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("unsupported key file version %d", f.Version)
	}
	return f.Keys, nil
}

func parseEnv(data []byte) ([]Entry, error) {
	var out []Entry
	sc := bufio.NewScanner(bytes.NewReader(data))
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		name, value, ok := strings.Cut(text, "=")
		if !ok || !strings.HasPrefix(name, envKeyPrefix) {
			return nil, fmt.Errorf("line %d: expected APIKEY_<ID>=algorithm,status,created_at,expires_at,name,hash", line)
		}
		id := strings.ToLower(strings.TrimPrefix(name, envKeyPrefix))
		if strings.ToUpper(id) != strings.TrimPrefix(name, envKeyPrefix) {
			return nil, fmt.Errorf("line %d: variable name must be upper case", line)
		}
		parts := strings.Split(value, ",")
		if len(parts) != 6 {
			return nil, fmt.Errorf("line %d: expected 6 comma-separated values, got %d", line, len(parts))
		}
		created, err := time.Parse(time.RFC3339, parts[2])
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid created_at: %w", line, err)
		}
		e := Entry{
			ID:        id,
			Algorithm: parts[0],
			Status:    parts[1],
			CreatedAt: created,
			Name:      parts[4],
			Hash:      parts[5],
		}
		if parts[3] != neverExpires {
			exp, err := time.Parse(time.RFC3339, parts[3])
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid expires_at: %w", line, err)
			}
			e.ExpiresAt = &exp
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Encode renders entries in the given format.
func Encode(entries []Entry, format string) ([]byte, error) {
	sorted := append([]Entry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })
	if format == FormatJSON {
		if sorted == nil {
			sorted = []Entry{}
		}
		b, err := json.MarshalIndent(jsonFile{Version: 1, Keys: sorted}, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(b, '\n'), nil
	}
	var buf bytes.Buffer
	buf.WriteString(envHeader)
	for _, e := range sorted {
		exp := neverExpires
		if e.ExpiresAt != nil {
			exp = e.ExpiresAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(&buf, "%s%s=%s,%s,%s,%s,%s,%s\n", envKeyPrefix, strings.ToUpper(e.ID),
			e.Algorithm, e.Status, e.CreatedAt.UTC().Format(time.RFC3339), exp, e.Name, e.Hash)
	}
	return buf.Bytes(), nil
}

// ReadFile reads and parses a key file. A missing file yields no entries
// when allowMissing is set.
func ReadFile(path string, allowMissing bool) ([]Entry, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if allowMissing && errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	entries, err := ParseFile(data, FormatOf(path))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return entries, data, nil
}

// Update reads the key file under an exclusive lock, applies fn, and
// writes the result atomically with temp-file, fsync, and rename.
func Update(path string, fn func([]Entry) ([]Entry, error)) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock %s: %w", path, err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck

	entries, _, err := ReadFile(path, true)
	if err != nil {
		return err
	}
	next, err := fn(entries)
	if err != nil {
		return err
	}
	for _, e := range next {
		if err := e.Validate(); err != nil {
			return err
		}
	}
	data, err := Encode(next, FormatOf(path))
	if err != nil {
		return err
	}
	return WriteAtomic(path, data, 0o600)
}

// WriteAtomic writes data to path through a temp file, fsync, and rename.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) //nolint:errcheck
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		d.Sync() //nolint:errcheck
		d.Close()
	}
	return nil
}
