package lister_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/cache"
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
)

func fakeSessions() model.SeshSessions {
	return model.SeshSessions{
		OrderedIndex: []string{"kitty:main"},
		Directory: model.SeshSessionMap{
			"kitty:main": {Src: "kitty", Name: "main", Path: "/home/user"},
		},
	}
}

func updatedSessions() model.SeshSessions {
	return model.SeshSessions{
		OrderedIndex: []string{"kitty:main", "kitty:dev"},
		Directory: model.SeshSessionMap{
			"kitty:main": {Src: "kitty", Name: "main", Path: "/home/user"},
			"kitty:dev":  {Src: "kitty", Name: "dev", Path: "/home/user/dev"},
		},
	}
}

func TestCachingLister_ColdStart(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	sessions := fakeSessions()
	// Inner is always called with empty opts (full list)
	inner.On("List", lister.ListOptions{}).Return(sessions, nil).Once()

	cl := lister.NewCachingLister(inner, fc)
	got, err := cl.List(lister.ListOptions{Kitty: true})
	require.NoError(t, err)
	assert.Equal(t, sessions.OrderedIndex, got.OrderedIndex)

	cached, err := fc.Read()
	require.NoError(t, err)
	assert.Equal(t, sessions.OrderedIndex, cached.Sessions.OrderedIndex)

	cl.Wait()
}

func TestCachingLister_WarmHit(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)
	opts := lister.ListOptions{Kitty: true}

	sessions := fakeSessions()
	require.NoError(t, fc.Write(sessions))

	cl := lister.NewCachingLister(inner, fc)
	got, err := cl.List(opts)
	require.NoError(t, err)
	assert.Equal(t, sessions.OrderedIndex, got.OrderedIndex)

	cl.Wait()
	inner.AssertNotCalled(t, "List")
}

func TestCachingLister_RefreshThenWarmRead(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)
	opts := lister.ListOptions{Kitty: true}

	fresh := updatedSessions()
	inner.On("List", opts).Return(fresh, nil).Once()

	cl := lister.NewCachingLister(inner, fc)
	cl.RefreshCache(opts)
	cl.Wait()

	got, err := cl.List(opts)
	require.NoError(t, err)
	assert.Equal(t, fresh.OrderedIndex, got.OrderedIndex)
	assert.Equal(t, fresh.Directory, got.Directory)
}

func TestCachingLister_ColdStartError(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	// Inner is always called with empty opts
	inner.On("List", lister.ListOptions{}).Return(model.SeshSessions{}, fmt.Errorf("kitty not running")).Once()

	cl := lister.NewCachingLister(inner, fc)
	_, err := cl.List(lister.ListOptions{Kitty: true})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kitty not running")

	cl.Wait()
}

func TestCachingLister_RefreshCache(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)
	opts := lister.ListOptions{Kitty: true}

	freshSessions := updatedSessions()
	inner.On("List", opts).Return(freshSessions, nil).Once()

	cl := lister.NewCachingLister(inner, fc)
	cl.RefreshCache(opts)
	cl.Wait()

	cached, err := fc.Read()
	require.NoError(t, err)
	assert.Equal(t, freshSessions.OrderedIndex, cached.Sessions.OrderedIndex)
}

func TestCachingLister_DelegatesMethods(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	session := model.SeshSession{Src: "kitty", Name: "test"}
	inner.On("FindKittySession", "test").Return(session, true)
	inner.On("GetAttachedKittySession").Return(session, true)
	inner.On("GetLastKittySession").Return(session, true)
	inner.On("FindConfigSession", "test").Return(session, true)
	inner.On("FindConfigWildcard", "/path").Return(model.WildcardConfig{}, false)
	inner.On("FindZoxideSession", "test").Return(session, true)
	inner.On("FindSessionFileConfig", "test").Return(session, true)
	inner.On("ListKittyWindows").Return(model.SeshSessions{}, nil)

	cl := lister.NewCachingLister(inner, fc)

	s, ok := cl.FindKittySession("test")
	assert.True(t, ok)
	assert.Equal(t, "test", s.Name)

	s, ok = cl.GetAttachedKittySession()
	assert.True(t, ok)

	s, ok = cl.GetLastKittySession()
	assert.True(t, ok)

	s, ok = cl.FindConfigSession("test")
	assert.True(t, ok)

	_, ok = cl.FindConfigWildcard("/path")
	assert.False(t, ok)

	s, ok = cl.FindZoxideSession("test")
	assert.True(t, ok)

	s, ok = cl.FindSessionFileConfig("test")
	assert.True(t, ok)

	panes, pErr := cl.ListKittyWindows()
	assert.NoError(t, pErr)
	assert.Empty(t, panes.OrderedIndex)
}

func sessionsWithAttached() model.SeshSessions {
	return model.SeshSessions{
		OrderedIndex: []string{"kitty:main", "kitty:dev", "kitty:work"},
		Directory: model.SeshSessionMap{
			"kitty:main": {Src: "kitty", Name: "main", Path: "/home/user", Attached: 1},
			"kitty:dev":  {Src: "kitty", Name: "dev", Path: "/home/user/dev"},
			"kitty:work": {Src: "kitty", Name: "work", Path: "/home/user/work"},
		},
	}
}

func TestCachingLister_HideAttached_ColdStart(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	sessions := sessionsWithAttached()
	// Inner is always called with empty opts (cache stores full list)
	inner.On("List", lister.ListOptions{}).Return(sessions, nil).Once()
	inner.On("GetAttachedKittySession").Return(sessions.Directory["kitty:main"], true)

	cl := lister.NewCachingLister(inner, fc)
	got, err := cl.List(lister.ListOptions{Kitty: true, HideAttached: true})
	require.NoError(t, err)
	// "main" should be filtered out
	assert.Equal(t, []string{"kitty:dev", "kitty:work"}, got.OrderedIndex)

	// Cache should still contain the full unfiltered list
	cached, err := fc.Read()
	require.NoError(t, err)
	assert.Equal(t, []string{"kitty:main", "kitty:dev", "kitty:work"}, cached.Sessions.OrderedIndex)

	cl.Wait()
}

func TestCachingLister_HideAttached_WarmHit(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	sessions := sessionsWithAttached()
	require.NoError(t, fc.Write(sessions))

	inner.On("GetAttachedKittySession").Return(sessions.Directory["kitty:main"], true)

	cl := lister.NewCachingLister(inner, fc)
	got, err := cl.List(lister.ListOptions{Kitty: true, HideAttached: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"kitty:dev", "kitty:work"}, got.OrderedIndex)

	cl.Wait()
	inner.AssertNotCalled(t, "List")
}

func TestCachingLister_HideAttached_NoAttachedSession(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	sessions := fakeSessions()
	require.NoError(t, fc.Write(sessions))

	inner.On("GetAttachedKittySession").Return(model.SeshSession{}, false)

	cl := lister.NewCachingLister(inner, fc)
	got, err := cl.List(lister.ListOptions{Kitty: true, HideAttached: true})
	require.NoError(t, err)
	// Nothing filtered since no session is attached
	assert.Equal(t, sessions.OrderedIndex, got.OrderedIndex)

	cl.Wait()
}

func TestCachingLister_WaitBlocksUntilDone(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)
	opts := lister.ListOptions{}

	sessions := fakeSessions()
	inner.On("List", opts).Return(sessions, nil).Run(func(_ mock.Arguments) {
		time.Sleep(50 * time.Millisecond)
	})

	cl := lister.NewCachingLister(inner, fc)
	cl.RefreshCache(opts)

	cl.Wait()

	cached, err := fc.Read()
	require.NoError(t, err)
	assert.Equal(t, sessions.OrderedIndex, cached.Sessions.OrderedIndex)
}

// --- Source filtering and dedup tests ---

func mixedSessions() model.SeshSessions {
	return model.SeshSessions{
		OrderedIndex: []string{"kitty:main", "config:project", "zoxide:code"},
		Directory: model.SeshSessionMap{
			"kitty:main":     {Src: "kitty", Name: "main", Path: "/home/user"},
			"config:project": {Src: "config", Name: "project", Path: "/home/user/project"},
			"zoxide:code":    {Src: "zoxide", Name: "code", Path: "/home/user/code"},
		},
	}
}

func TestCachingLister_SourceFilter_FromCache(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	// Pre-populate cache with mixed-source sessions
	sessions := mixedSessions()
	require.NoError(t, fc.Write(sessions))

	cl := lister.NewCachingLister(inner, fc)

	// Request only kitty — should filter from full cache
	got, err := cl.List(lister.ListOptions{Kitty: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"kitty:main"}, got.OrderedIndex)

	cl.Wait()
	inner.AssertNotCalled(t, "List")
}

func TestCachingLister_SourceFilter_DifferentFilters(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	// Pre-populate cache with mixed-source sessions
	sessions := mixedSessions()
	require.NoError(t, fc.Write(sessions))

	cl := lister.NewCachingLister(inner, fc)

	// First call: kitty only
	got, err := cl.List(lister.ListOptions{Kitty: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"kitty:main"}, got.OrderedIndex)

	// Second call: zoxide only — must NOT return kitty sessions
	got, err = cl.List(lister.ListOptions{Zoxide: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"zoxide:code"}, got.OrderedIndex)

	// Third call: no source flags — returns all
	got, err = cl.List(lister.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, sessions.OrderedIndex, got.OrderedIndex)

	cl.Wait()
	inner.AssertNotCalled(t, "List")
}

func TestCachingLister_SourceFilter_ColdStart(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	sessions := mixedSessions()
	// Inner is called with empty opts (fetches all sources)
	inner.On("List", lister.ListOptions{}).Return(sessions, nil).Once()

	cl := lister.NewCachingLister(inner, fc)

	// Request only zoxide — inner fetches all, cache stores all, filter returns zoxide
	got, err := cl.List(lister.ListOptions{Zoxide: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"zoxide:code"}, got.OrderedIndex)

	// Cache should contain all sources
	cached, err := fc.Read()
	require.NoError(t, err)
	assert.Equal(t, sessions.OrderedIndex, cached.Sessions.OrderedIndex)

	cl.Wait()
}

func TestCachingLister_HideAttached_BeforeHideDuplicates(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	sessions := model.SeshSessions{
		OrderedIndex: []string{"kitty:project", "config:project", "kitty:other"},
		Directory: model.SeshSessionMap{
			"kitty:project":  {Src: "kitty", Name: "project", Path: "/p"},
			"config:project": {Src: "config", Name: "project", Path: "/p"},
			"kitty:other":    {Src: "kitty", Name: "other", Path: "/o"},
		},
	}
	require.NoError(t, fc.Write(sessions))

	inner.On("GetAttachedKittySession").Return(sessions.Directory["kitty:project"], true)

	cl := lister.NewCachingLister(inner, fc)
	got, err := cl.List(lister.ListOptions{HideAttached: true, HideDuplicates: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"config:project", "kitty:other"}, got.OrderedIndex)

	cl.Wait()
	inner.AssertNotCalled(t, "List")
}

func TestCachingLister_HideDuplicates_PostCache(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	// Sessions with duplicate name and path across sources
	sessions := model.SeshSessions{
		OrderedIndex: []string{"kitty:project", "config:project", "zoxide:other"},
		Directory: model.SeshSessionMap{
			"kitty:project":  {Src: "kitty", Name: "project", Path: "/home/user/project"},
			"config:project": {Src: "config", Name: "project", Path: "/home/user/project"},
			"zoxide:other":   {Src: "zoxide", Name: "other", Path: "/home/user/other"},
		},
	}
	require.NoError(t, fc.Write(sessions))

	cl := lister.NewCachingLister(inner, fc)

	// With HideDuplicates, "config:project" is a duplicate of "kitty:project" (same name + path)
	got, err := cl.List(lister.ListOptions{HideDuplicates: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"kitty:project", "zoxide:other"}, got.OrderedIndex)

	// Without HideDuplicates, all returned
	got, err = cl.List(lister.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, sessions.OrderedIndex, got.OrderedIndex)

	cl.Wait()
	inner.AssertNotCalled(t, "List")
}

func TestCachingLister_FormatsAfterCacheRead(t *testing.T) {
	dir := t.TempDir()
	fc := cache.NewFileCacheWithPath(filepath.Join(dir, "sessions.gob"))
	inner := lister.NewMockLister(t)

	sessions := fakeSessions()
	require.NoError(t, fc.Write(sessions))

	opts := lister.ListOptions{Format: "{name}", FormatSet: true}
	formatted := fakeSessions()
	formattedSession := formatted.Directory["kitty:main"]
	formattedSession.Name = "formatted-main"
	formatted.Directory["kitty:main"] = formattedSession
	inner.On("Format", sessions, opts).Return(formatted, nil).Once()

	cl := lister.NewCachingLister(inner, fc)
	got, err := cl.List(opts)
	require.NoError(t, err)
	assert.Equal(t, "formatted-main", got.Directory["kitty:main"].Name)

	cached, err := fc.Read()
	require.NoError(t, err)
	assert.Equal(t, "main", cached.Sessions.Directory["kitty:main"].Name)
	inner.AssertNotCalled(t, "List")
}
