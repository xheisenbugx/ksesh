package lister

import (
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/xheisenbugx/ksesh/cache"
	"github.com/xheisenbugx/ksesh/model"
)

const softTTL = 5 * time.Second

// CachingLister wraps a Lister with stale-while-revalidate caching for List().
// All other Lister methods delegate directly to the inner lister.
type CachingLister struct {
	inner Lister
	cache cache.Cache
	wg    sync.WaitGroup
}

// NewCachingLister creates a CachingLister that decorates inner with file-based caching.
func NewCachingLister(inner Lister, c cache.Cache) *CachingLister {
	return &CachingLister{inner: inner, cache: c}
}

// List implements Lister. It returns cached data when available, triggering a
// background refresh when the cache is older than the soft TTL.
// The cache always stores the full unfiltered list; source filters,
// HideDuplicates, and HideAttached are applied after reading from cache.
func (cl *CachingLister) List(opts ListOptions) (model.SeshSessions, error) {
	// Always fetch/store the full list (all sources, no dedup, no hide);
	// we apply all view-level filters ourselves after cache read.
	innerOpts := ListOptions{}

	cached, err := cl.cache.Read()
	if err == nil {
		age := time.Since(cached.Timestamp)
		if age < softTTL {
			slog.Debug("cache: hit (fresh)", "age", age)
			return cl.finish(cached.Sessions, opts)
		}
		// Stale -- return immediately but revalidate in background
		slog.Debug("cache: hit (stale, revalidating)", "age", age)
		cl.wg.Add(1)
		go func() {
			defer cl.wg.Done()
			cl.revalidate(innerOpts)
		}()
		return cl.finish(cached.Sessions, opts)
	}

	// Cold start -- fetch synchronously
	slog.Debug("cache: miss (cold start)")
	sessions, err := cl.inner.List(innerOpts)
	if err != nil {
		return sessions, err
	}
	if writeErr := cl.cache.Write(sessions); writeErr != nil {
		slog.Warn("cache: write failed on cold start", "error", writeErr)
	}
	return cl.finish(sessions, opts)
}

// finish applies view-level filtering and then display formatting without
// changing the raw session data stored in the cache.
func (cl *CachingLister) finish(sessions model.SeshSessions, opts ListOptions) (model.SeshSessions, error) {
	filtered := cl.applyFilters(sessions, opts)
	if !needsFormatting(opts) {
		return filtered, nil
	}
	return cl.inner.Format(filtered, opts)
}

// applyFilters applies view-level filters that should not affect what gets
// stored in the cache: source filtering, HideDuplicates, and HideAttached.
func (cl *CachingLister) applyFilters(sessions model.SeshSessions, opts ListOptions) model.SeshSessions {
	filtered := sessions.OrderedIndex

	// 1. Source filtering: if any source flag is set, keep only matching sessions.
	if allowed := sourceSet(opts); allowed != nil {
		result := make([]string, 0, len(filtered))
		for _, index := range filtered {
			if allowed[sessions.Directory[index].Src] {
				result = append(result, index)
			}
		}
		filtered = result
	}

	// 2. HideAttached runs before HideDuplicates so the attached kitty
	// session is removed from the dedup input. Otherwise kitty would win
	// the dedup against a same-named config/sessionfile entry and then be
	// hidden, leaving no entry for the user to pick.
	if opts.HideAttached {
		attached, ok := cl.inner.GetAttachedKittySession()
		if ok {
			for i, index := range filtered {
				s := sessions.Directory[index]
				if s.Src == "kitty" && s.Name == attached.Name {
					filtered = slices.Delete(slices.Clone(filtered), i, i+1)
					break
				}
			}
		}
	}

	// 3. HideDuplicates: per-source dedup rules (see applyDedup).
	if opts.HideDuplicates {
		filtered = applyDedup(model.SeshSessions{
			OrderedIndex: filtered,
			Directory:    sessions.Directory,
		})
	}

	if len(filtered) == len(sessions.OrderedIndex) {
		return sessions
	}
	return model.SeshSessions{
		OrderedIndex: filtered,
		Directory:    sessions.Directory,
	}
}

// sourceSet returns a set of allowed source names based on opts, or nil if
// no source flags are set (meaning all sources are allowed).
func sourceSet(opts ListOptions) map[string]bool {
	if !opts.Kitty && !opts.Config && !opts.Zoxide && !opts.SessionFile && !opts.Panes {
		return nil
	}
	m := make(map[string]bool)
	if opts.Kitty {
		m["kitty"] = true
	}
	if opts.Config {
		m["config"] = true
	}
	if opts.Zoxide {
		m["zoxide"] = true
	}
	if opts.SessionFile {
		m["sessionfile"] = true
	}
	if opts.Panes {
		m["kitty-window"] = true
	}
	return m
}

func (cl *CachingLister) revalidate(opts ListOptions) {
	sessions, err := cl.inner.List(opts)
	if err != nil {
		slog.Warn("cache: background revalidation fetch failed", "error", err)
		return
	}
	if err := cl.cache.Write(sessions); err != nil {
		slog.Warn("cache: background revalidation write failed", "error", err)
	}
}

// RefreshCache fetches live data from the inner lister and writes it to the cache.
// This bypasses the cache read entirely, intended for use after sesh connect.
func (cl *CachingLister) RefreshCache(opts ListOptions) {
	cl.wg.Add(1)
	go func() {
		defer cl.wg.Done()
		cl.revalidate(opts)
	}()
}

// Wait blocks until all background refresh goroutines have completed.
// Call this before process exit to avoid truncated cache writes.
func (cl *CachingLister) Wait() {
	cl.wg.Wait()
}

// --- Delegate all other Lister methods to inner ---

func (cl *CachingLister) Format(sessions model.SeshSessions, opts ListOptions) (model.SeshSessions, error) {
	return cl.inner.Format(sessions, opts)
}

func (cl *CachingLister) ListKittyWindows() (model.SeshSessions, error) {
	return cl.inner.ListKittyWindows()
}

func (cl *CachingLister) FindKittySession(name string) (model.SeshSession, bool) {
	return cl.inner.FindKittySession(name)
}

func (cl *CachingLister) FindKittySessionByBase(base string) (model.SeshSession, bool) {
	return cl.inner.FindKittySessionByBase(base)
}

func (cl *CachingLister) GetAttachedKittySession() (model.SeshSession, bool) {
	return cl.inner.GetAttachedKittySession()
}

func (cl *CachingLister) GetLastKittySession() (model.SeshSession, bool) {
	return cl.inner.GetLastKittySession()
}

func (cl *CachingLister) FindConfigSession(name string) (model.SeshSession, bool) {
	return cl.inner.FindConfigSession(name)
}

func (cl *CachingLister) FindConfigWildcard(path string) (model.WildcardConfig, bool) {
	return cl.inner.FindConfigWildcard(path)
}

func (cl *CachingLister) FindZoxideSession(name string) (model.SeshSession, bool) {
	return cl.inner.FindZoxideSession(name)
}

func (cl *CachingLister) FindSessionFileConfig(name string) (model.SeshSession, bool) {
	return cl.inner.FindSessionFileConfig(name)
}
