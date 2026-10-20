package sessionfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/oswrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	for _, f := range []string{"work.kitty-session", "api.session", "notes.txt", "dupe.kitty_session"} {
		require.NoError(t, os.WriteFile(filepath.Join(first, f), nil, 0o644))
	}
	require.NoError(t, os.Mkdir(filepath.Join(first, "dir.kitty-session"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(second, "dupe.kitty-session"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(second, "zeta.kitty-session"), nil, 0o644))

	mHome := home.NewMockHome(t)
	mHome.EXPECT().ExpandPath(first).Return(first, nil)
	mHome.EXPECT().ExpandPath(second).Return(second, nil)
	mHome.EXPECT().ExpandPath("/missing").Return("/missing", nil)
	s := NewSessionFile(oswrap.NewOs(), mHome, []string{first, "/missing", second})

	configs, err := s.List()

	require.NoError(t, err)
	assert.Equal(t, []*model.SessionFileConfig{
		{Name: "api", Path: filepath.Join(first, "api.session")},
		{Name: "dupe", Path: filepath.Join(first, "dupe.kitty_session")},
		{Name: "work", Path: filepath.Join(first, "work.kitty-session")},
		{Name: "zeta", Path: filepath.Join(second, "zeta.kitty-session")},
	}, configs)
}

func TestDefaultDir(t *testing.T) {
	mHome := home.NewMockHome(t)
	mHome.EXPECT().ExpandPath(DefaultDir).Return("/nonexistent/kitty/sessions", nil)

	configs, err := NewSessionFile(oswrap.NewOs(), mHome, nil).List()

	require.NoError(t, err)
	assert.Empty(t, configs)
}
