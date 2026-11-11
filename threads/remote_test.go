package threads

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/remote"
)

func TestTheThreadOfAnAgentOnAnotherMachineIsKeptHere(t *testing.T) {
	k, mk, _ := newKitty(t)
	since := now.Add(-time.Minute)
	away := &model.Agent{WindowID: 5, Name: "claude", Status: "waiting", Message: "Allow Bash: make", Since: &since,
		Title: "✳ Deploy the API", Session: "api@devbox", Conversation: "far", Host: "devbox", Path: "/home/me/c/api",
		Thread: model.Thread{Pin: 1}}
	mk.EXPECT().ListAgents().Return([]*model.Agent{away}, nil)

	_, err := k.ListAgents()
	require.NoError(t, err)

	rec, ok := k.store.Get("far")
	require.True(t, ok, "its hooks keep its history on that machine: here, it is kept from its window")
	assert.Equal(t, "devbox", rec.Host)
	assert.Equal(t, "/home/me/c/api", rec.Cwd)
	assert.Equal(t, "claude", rec.Agent)
	assert.Equal(t, "Deploy the API", rec.Title)
	assert.Equal(t, "Allow Bash: make", rec.Message)
	assert.Equal(t, model.Thread{Pin: 1}, rec.Thread)

	// Its window gone, it is a closed thread on that machine.
	mk.EXPECT().ListAgents().Unset()
	mk.EXPECT().ListAgents().Return(nil, nil)
	agents, err := k.ListAgents()
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.True(t, agents[0].Closed)
	assert.Equal(t, "devbox", agents[0].Host)
}

func TestAnAgentHereWithoutAHistoryIsStillNotRecorded(t *testing.T) {
	k, mk, _ := newKitty(t)
	mk.EXPECT().ListAgents().Return([]*model.Agent{liveAgent(1, "here", "Local", model.Thread{})}, nil)
	_, err := k.ListAgents()
	require.NoError(t, err)
	_, ok := k.store.Get("here")
	assert.False(t, ok, "its own hook writes it")
}

func TestResumeAThreadOnAnotherMachine(t *testing.T) {
	k, mk, _ := newKitty(t)
	require.NoError(t, k.store.Update("far", func(r *model.ThreadRecord) {
		*r = model.ThreadRecord{Conversation: "far", Agent: "claude", Host: "devbox", Cwd: "/home/me/c/api",
			CustomTitle: "Deploy", Thread: model.Thread{Pin: 3}, StatusAt: now.Add(-time.Hour)}
	}))
	mk.EXPECT().ListAgents().Return(nil, nil)
	mk.EXPECT().ListSessions().Return(nil, nil)
	window := remote.Join(remote.Window("devbox", "/home/me/c/api")...)
	mk.EXPECT().NewSessionWith("api@devbox", "", window, map[string]string{
		kitty.PinVar: "3", kitty.HostVar: "devbox", kitty.CwdVar: "/home/me/c/api", kitty.TitleVar: "Deploy",
	}).Return("window:9", nil)
	mk.EXPECT().WaitForPrompt("window:9", remotePrompt).Return(nil)
	mk.EXPECT().SendKeys("window:9", "claude --resume far").Return("", nil)
	mk.EXPECT().SwitchOrAttach("api@devbox", model.ConnectOpts{Switch: true}).Return("", nil)

	require.NoError(t, k.ResumeThread("far"), "its directory isn't here: it isn't looked for")
}
