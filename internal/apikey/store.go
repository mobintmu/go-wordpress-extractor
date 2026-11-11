package apikey

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"go-wordpress/internal/config"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Snapshot is an immutable set of keys loaded from one version of the file.
type Snapshot struct {
	byID     map[string]Entry
	Checksum string
	Count    int
}

// Store holds the active key snapshot and verifies presented keys.
type Store struct {
	path    string
	pepper  string
	log     *zap.Logger
	current atomic.Pointer[Snapshot]
	stamp   fileStamp
	now     func() time.Time
	dummy   string
}

type fileStamp struct {
	mod  time.Time
	size int64
}

// NewStore loads the key file. A bad file stops startup.
func NewStore(cfg *config.Config, log *zap.Logger) (*Store, error) {
	s := &Store{
		path:   cfg.APIKeysFile,
		pepper: cfg.APIKeyPepper,
		log:    log,
		now:    time.Now,
	}
	s.dummy = Hash(s.pepper, "dummy-key-for-constant-time-checks")
	if err := s.Reload(); err != nil {
		return nil, fmt.Errorf("load API keys: %w", err)
	}
	return s, nil
}

// Reload reads the key file and swaps in a new snapshot. On error the
// previous snapshot stays active.
func (s *Store) Reload() error {
	st, err := os.Stat(s.path)
	if err != nil {
		return err
	}
	entries, data, err := ReadFile(s.path, false)
	if err != nil {
		return err
	}
	snap := &Snapshot{byID: make(map[string]Entry, len(entries)), Count: len(entries)}
	for _, e := range entries {
		snap.byID[e.ID] = e
	}
	sum := sha256.Sum256(data)
	snap.Checksum = hex.EncodeToString(sum[:])
	s.current.Store(snap)
	s.stamp = fileStamp{mod: st.ModTime(), size: st.Size()}
	s.log.Info("API keys loaded",
		zap.String("file", s.path),
		zap.Int("keys", snap.Count),
		zap.String("checksum", snap.Checksum),
		zap.String("pepper_fingerprint", PepperFingerprint(s.pepper)))
	return nil
}

// Snapshot returns the active snapshot.
func (s *Store) Snapshot() *Snapshot { return s.current.Load() }

// Verify checks a presented key. It returns the key id when the key is
// valid. Every failure takes the same path and returns false.
func (s *Store) Verify(presented string) (string, bool) {
	snap := s.current.Load()
	id, err := Parse(presented)
	entry, found := Entry{}, false
	if err == nil {
		entry, found = snap.byID[id]
	}
	stored := s.dummy
	if found {
		stored = entry.Hash
	}
	match := HashEqual(s.pepper, presented, stored)
	if err != nil || !found || !match || !entry.Usable(s.now()) {
		return "", false
	}
	return id, true
}

func (s *Store) changed() bool {
	st, err := os.Stat(s.path)
	if err != nil {
		return true
	}
	return !st.ModTime().Equal(s.stamp.mod) || st.Size() != s.stamp.size
}

// RegisterReloader polls the key file and reloads it when it changes.
func RegisterReloader(lc fx.Lifecycle, s *Store, cfg *config.Config) {
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
						if !s.changed() {
							continue
						}
						if err := s.Reload(); err != nil {
							// Remember the bad stamp so the error is logged once per change.
							if st, statErr := os.Stat(s.path); statErr == nil {
								s.stamp = fileStamp{mod: st.ModTime(), size: st.Size()}
							}
							s.log.Error("API key reload failed; keeping previous keys", zap.Error(err))
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
