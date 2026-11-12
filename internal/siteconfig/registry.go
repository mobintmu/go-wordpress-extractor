package siteconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"go-wordpress/internal/config"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Snapshot is an immutable view of every site config. Requests keep the
// snapshot they started with, even if a reload happens meanwhile.
type Snapshot struct {
	Entries  []*Entry
	byHost   map[string]*Entry
	LoadedAt time.Time

	// active holds the entries named in ActiveFile. When the file is
	// absent it is nil and every entry counts as active.
	active map[*Entry]bool
	// UnknownActive lists domains in ActiveFile that no site config claims.
	UnknownActive []string
}

// IsActive reports whether e is listed in ActiveFile. Without that file,
// every entry is active.
func (s *Snapshot) IsActive(e *Entry) bool { return s.active == nil || s.active[e] }

// WithActive marks the entries claiming domains as active.
func (s *Snapshot) WithActive(domains []string) *Snapshot {
	s.active = map[*Entry]bool{}
	s.UnknownActive = nil
	for _, d := range domains {
		if e := s.Lookup(d); e != nil {
			s.active[e] = true
		} else {
			s.UnknownActive = append(s.UnknownActive, d)
		}
	}
	return s
}

// Lookup returns the entry claiming host, or nil.
func (s *Snapshot) Lookup(host string) *Entry { return s.byHost[NormalizeHost(host)] }

// NewSnapshot indexes entries by domain.
func NewSnapshot(entries []*Entry) *Snapshot {
	s := &Snapshot{Entries: entries, byHost: map[string]*Entry{}, LoadedAt: time.Now()}
	for _, e := range entries {
		for _, d := range e.Domains {
			if _, taken := s.byHost[d]; !taken {
				s.byHost[d] = e
			}
		}
	}
	return s
}

// Registry serves the current snapshot and reloads it when files change.
type Registry struct {
	dir         string
	log         *zap.Logger
	current     atomic.Pointer[Snapshot]
	fingerprint string
}

// NewRegistry loads every site config. Invalid sites are logged and marked
// unavailable; only an unreadable directory stops startup.
func NewRegistry(cfg *config.Config, log *zap.Logger) (*Registry, error) {
	r := &Registry{dir: cfg.SiteConfigDir, log: log}
	if err := r.Reload(); err != nil {
		return nil, fmt.Errorf("load site configs: %w", err)
	}
	return r, nil
}

// Snapshot returns the active snapshot.
func (r *Registry) Snapshot() *Snapshot { return r.current.Load() }

// Reload loads a complete new snapshot. If it cannot be loaded, the
// previous snapshot stays active.
func (r *Registry) Reload() error {
	fp, err := fingerprint(r.dir)
	if err != nil {
		return err
	}
	entries, err := LoadDir(r.dir)
	if err != nil {
		return err
	}
	active, hasActive, err := LoadActive(r.dir)
	if err != nil {
		return err
	}
	snap := NewSnapshot(entries)
	if hasActive {
		snap.WithActive(active)
		for _, d := range snap.UnknownActive {
			r.log.Warn("active site has no site config", zap.String("domain", d), zap.String("file", ActiveFile))
		}
	} else {
		r.log.Info("no active sites file; listing every enabled site", zap.String("file", ActiveFile))
	}
	r.current.Store(snap)
	r.fingerprint = fp

	valid := 0
	for _, e := range entries {
		fields := []zap.Field{
			zap.String("site_id", e.SiteID),
			zap.String("version", e.Version),
			zap.String("status", e.Status()),
			zap.String("file", filepath.Base(e.File)),
		}
		if e.Err != nil {
			r.log.Error("site config invalid", append(fields, zap.Error(e.Err))...)
			continue
		}
		valid++
		r.log.Info("site config loaded", fields...)
	}
	r.log.Info("site configs loaded", zap.Int("sites", len(entries)), zap.Int("valid", valid), zap.String("fingerprint", fp))
	return nil
}

// fingerprint hashes the names, sizes, and modification times of every
// JSON file under dir, so a change to any of them is noticed.
func fingerprint(dir string) (string, error) {
	var lines []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano()))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RegisterReloader polls the config directory and reloads on change.
func RegisterReloader(lc fx.Lifecycle, r *Registry, cfg *config.Config) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				t := time.NewTicker(cfg.ReloadInterval)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						fp, err := fingerprint(r.dir)
						if err == nil && fp == r.fingerprint {
							continue
						}
						if err := r.Reload(); err != nil {
							r.log.Error("site config reload failed; keeping previous snapshot", zap.Error(err))
						}
					}
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			<-done
			return nil
		},
	})
}

// Exists reports whether path exists. Used by tools.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
