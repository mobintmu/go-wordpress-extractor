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
	"strings"
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

	// Learned holds every file in the learned directory, whatever its
	// status. Only active, valid ones are indexed for Lookup, and only for
	// hosts no site config claims.
	Learned       []*Entry
	learnedByHost map[string]*Entry
	// Default is the built-in AI rule for hosts without a config, served by
	// APP_DEFAULT_ENGINE, or nil when APP_DEFAULT_ENGINE is none.
	Default *Entry
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

// Lookup returns the entry claiming host, or nil. Site configs come first,
// then active learned configs. The default AI rule is not returned here.
func (s *Snapshot) Lookup(host string) *Entry { return s.byHost[NormalizeHost(host)] }

// LearnedFor returns the learned entry for host in any status, or nil.
func (s *Snapshot) LearnedFor(host string) *Entry {
	if s.learnedByHost == nil {
		return nil
	}
	return s.learnedByHost[NormalizeHost(host)]
}

// WithLearned adds the learned entries. An active, valid one claims its
// domains unless a site config already does.
func (s *Snapshot) WithLearned(entries []*Entry) *Snapshot {
	s.Learned = entries
	s.learnedByHost = map[string]*Entry{}
	for _, e := range entries {
		for _, d := range e.Domains {
			s.learnedByHost[d] = e
			if e.LearnedStatus() != LearnedActive {
				continue
			}
			if _, taken := s.byHost[d]; !taken {
				s.byHost[d] = e
			}
		}
	}
	return s
}

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
	dir        string
	learnedDir string
	// defaultEngine serves hosts without a rule; "" for none.
	defaultEngine string
	aiDelayMS     int
	log           *zap.Logger
	current       atomic.Pointer[Snapshot]
	fingerprint   string
}

// ResolveDefaultEngine replaces fetch.engine "default" with engine in every
// valid entry. The registry calls it with APP_DEFAULT_ENGINE, so a site config
// can follow the instance's default AI engine instead of naming one. With
// engine "" (APP_DEFAULT_ENGINE is none, or the AI crawler is off) entries keep
// "default": the site is loaded but a request for it fails with
// ENGINE_UNAVAILABLE.
func ResolveDefaultEngine(entries []*Entry, engine string) {
	if engine == "" {
		return
	}
	for _, e := range entries {
		if e.Site != nil && e.Site.Fetch.Engine == EngineDefault {
			e.Site.Fetch.Engine = engine
		}
	}
}

// NewRegistry loads every site config. Invalid sites are logged and marked
// unavailable; only an unreadable directory stops startup.
func NewRegistry(cfg *config.Config, log *zap.Logger) (*Registry, error) {
	r := &Registry{dir: cfg.SiteConfigDir, log: log,
		learnedDir:    cfg.LearnedDir(),
		defaultEngine: cfg.DefaultAIEngine(),
		aiDelayMS:     cfg.AICrawlerDefaultDelayMS,
	}
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
	fp, err := r.dirsFingerprint()
	if err != nil {
		return err
	}
	entries, err := LoadDir(r.dir)
	if err != nil {
		return err
	}
	ResolveDefaultEngine(entries, r.defaultEngine)
	active, hasActive, err := LoadActive(r.dir)
	if err != nil {
		return err
	}
	learned, err := LoadLearned(r.learnedDir)
	if err != nil {
		return err
	}
	snap := NewSnapshot(entries).WithLearned(learned)
	if r.defaultEngine != "" {
		snap.Default = DefaultAIEntry(r.defaultEngine, r.aiDelayMS)
	}
	for _, e := range learned {
		if e.Err != nil {
			r.log.Error("learned config invalid", zap.String("file", filepath.Base(e.File)), zap.Error(e.Err))
		} else {
			r.log.Info("learned config loaded", zap.String("site_id", e.SiteID), zap.String("status", e.LearnedStatus()), zap.String("file", filepath.Base(e.File)))
		}
	}
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

// dirsFingerprint covers the config directory and, when it lies outside it,
// the learned directory.
func (r *Registry) dirsFingerprint() (string, error) {
	fp, err := fingerprint(r.dir)
	if err != nil || r.learnedDir == "" || isWithin(r.learnedDir, r.dir) {
		return fp, err
	}
	if _, serr := os.Stat(r.learnedDir); serr != nil {
		return fp, nil
	}
	lfp, err := fingerprint(r.learnedDir)
	return fp + lfp, err
}

func isWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
						fp, err := r.dirsFingerprint()
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
