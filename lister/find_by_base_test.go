package lister

import (
	"testing"

	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/stretchr/testify/assert"
)

func TestFindKittySessionByBase(t *testing.T) {
	newListerWith := func(names ...string) *RealLister {
		sessions := make([]*model.KittySession, 0, len(names))
		for _, n := range names {
			sessions = append(sessions, &model.KittySession{Name: n, Path: "/p/" + n})
		}
		mockKitty := new(kitty.MockKitty)
		mockKitty.On("ListSessions").Return(sessions, nil)
		return &RealLister{kitty: mockKitty}
	}

	t.Run("exact match wins", func(t *testing.T) {
		l := newListerWith("sesh", "other")
		got, ok := l.FindKittySessionByBase("sesh")
		assert.True(t, ok)
		assert.Equal(t, "sesh", got.Name)
	})

	t.Run("matches enriched prefix", func(t *testing.T) {
		l := newListerWith("400-status — warm the cache", "other")
		got, ok := l.FindKittySessionByBase("400-status")
		assert.True(t, ok)
		assert.Equal(t, "400-status — warm the cache", got.Name)
	})

	t.Run("prefers exact over enriched prefix", func(t *testing.T) {
		l := newListerWith("400-status — warm the cache", "400-status")
		got, ok := l.FindKittySessionByBase("400-status")
		assert.True(t, ok)
		assert.Equal(t, "400-status", got.Name)
	})

	t.Run("does not match a bare prefix without the separator", func(t *testing.T) {
		l := newListerWith("sesh-ui")
		_, ok := l.FindKittySessionByBase("sesh")
		assert.False(t, ok)
	})

	t.Run("matches base containing a slash", func(t *testing.T) {
		l := newListerWith("w/400 — warm the cache")
		got, ok := l.FindKittySessionByBase("w/400")
		assert.True(t, ok)
		assert.Equal(t, "w/400 — warm the cache", got.Name)
	})

	t.Run("no match returns false", func(t *testing.T) {
		l := newListerWith("alpha", "beta")
		_, ok := l.FindKittySessionByBase("gamma")
		assert.False(t, ok)
	})

	t.Run("matches a renamed session by its kitty name", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		mockKitty.On("ListSessions").Return([]*model.KittySession{
			{Name: "other", NativeName: "other", Path: "/p/other"},
			{Name: "renamed/x", NativeName: "ksesh", Path: "/p/ksesh"},
		}, nil)
		l := &RealLister{kitty: mockKitty}
		got, ok := l.FindKittySessionByBase("ksesh")
		assert.True(t, ok)
		assert.Equal(t, "renamed/x", got.Name)
	})

	t.Run("prefers a sesh name over another session's kitty name", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		mockKitty.On("ListSessions").Return([]*model.KittySession{
			{Name: "api", NativeName: "web", Path: "/p/web"},
			{Name: "web2", NativeName: "api", Path: "/p/api"},
		}, nil)
		l := &RealLister{kitty: mockKitty}
		got, ok := l.FindKittySession("api")
		assert.True(t, ok)
		assert.Equal(t, "/p/web", got.Path)
	})
}
