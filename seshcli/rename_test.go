package seshcli

import (
	"testing"

	"github.com/xheisenbugx/ksesh/github"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/namer"
	"github.com/stretchr/testify/assert"
)

// kittyRenamer embeds the generated kitty mock (so it satisfies the full
// kitty.Kitty interface) and overrides RenameSession to capture its arguments.
type kittyRenamer struct {
	*kitty.MockKitty
	called     bool
	gotTarget  string
	gotNewName string
}

func newKittyRenamer() *kittyRenamer {
	return &kittyRenamer{MockKitty: new(kitty.MockKitty)}
}

func (t *kittyRenamer) RenameSession(target string, newName string) (string, error) {
	t.called = true
	t.gotTarget = target
	t.gotNewName = newName
	return "", nil
}

func TestEnrichedName(t *testing.T) {
	t.Run("appends sanitized issue title to base", func(t *testing.T) {
		mockNamer := new(namer.MockNamer)
		mockGithub := new(github.MockGithub)
		mockNamer.On("Name", "/p").Return("400-status", nil)
		mockGithub.On("Issue", "/p").
			Return(github.Issue{Number: 400, Title: "warm: the cache", State: "OPEN"}, true, nil)

		deps := &Deps{Namer: mockNamer}
		deps.Github = mockGithub

		assert.Equal(t, "400-status — warm the cache", enrichedName(deps, "/p"))
	})

	t.Run("returns bare base when no issue", func(t *testing.T) {
		mockNamer := new(namer.MockNamer)
		mockGithub := new(github.MockGithub)
		mockNamer.On("Name", "/p").Return("400-status", nil)
		mockGithub.On("Issue", "/p").Return(github.Issue{}, false, nil)

		deps := &Deps{Namer: mockNamer}
		deps.Github = mockGithub

		assert.Equal(t, "400-status", enrichedName(deps, "/p"))
	})

	t.Run("returns empty when namer fails", func(t *testing.T) {
		mockNamer := new(namer.MockNamer)
		mockNamer.On("Name", "/p").Return("", assert.AnError)

		deps := &Deps{Namer: mockNamer}

		assert.Equal(t, "", enrichedName(deps, "/p"))
	})
}

func TestRenameTarget(t *testing.T) {
	t.Run("uses the named session when an arg is given", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockLister.On("FindKittySession", "foo").
			Return(model.SeshSession{Name: "foo", Path: "/p/foo"}, true)

		deps := &Deps{Lister: mockLister}
		got, ok := renameTarget(deps, []string{"foo"})

		assert.True(t, ok)
		assert.Equal(t, "foo", got.Name)
	})

	t.Run("falls back to the attached session", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockLister.On("GetAttachedKittySession").
			Return(model.SeshSession{Name: "bar", Path: "/p/bar"}, true)

		deps := &Deps{Lister: mockLister}
		got, ok := renameTarget(deps, nil)

		assert.True(t, ok)
		assert.Equal(t, "bar", got.Name)
	})
}

func TestRunEnrich(t *testing.T) {
	t.Run("renames when the enriched name differs", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockNamer := new(namer.MockNamer)
		mockGithub := new(github.MockGithub)
		mockKitty := newKittyRenamer()
		mockLister.On("GetAttachedKittySession").
			Return(model.SeshSession{Name: "400-status", Path: "/p"}, true)
		mockNamer.On("Name", "/p").Return("400-status", nil)
		mockGithub.On("Issue", "/p").
			Return(github.Issue{Number: 400, Title: "warm the cache", State: "OPEN"}, true, nil)

		deps := &Deps{Lister: mockLister, Namer: mockNamer}
		deps.Github = mockGithub
		deps.Kitty = mockKitty

		err := runEnrich(deps, nil)

		assert.NoError(t, err)
		assert.Equal(t, "400-status", mockKitty.gotTarget)
		assert.Equal(t, "400-status — warm the cache", mockKitty.gotNewName)
	})

	t.Run("does not rename when the name is unchanged (idempotent)", func(t *testing.T) {
		mockLister := new(lister.MockLister)
		mockNamer := new(namer.MockNamer)
		mockGithub := new(github.MockGithub)
		mockKitty := newKittyRenamer()
		mockLister.On("GetAttachedKittySession").
			Return(model.SeshSession{Name: "400-status — warm the cache", Path: "/p"}, true)
		mockNamer.On("Name", "/p").Return("400-status", nil)
		mockGithub.On("Issue", "/p").
			Return(github.Issue{Number: 400, Title: "warm the cache", State: "OPEN"}, true, nil)

		deps := &Deps{Lister: mockLister, Namer: mockNamer}
		deps.Github = mockGithub
		deps.Kitty = mockKitty

		err := runEnrich(deps, nil)

		assert.NoError(t, err)
		assert.False(t, mockKitty.called)
	})
}
