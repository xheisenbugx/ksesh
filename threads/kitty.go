package threads

import (
	"cmp"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/namer"
)

// Record is how a hook event changes its conversation's record: the status
// it reports, and, when the hook read the window (seen), what the window
// shows of the thread.
func Record(r *model.ThreadRecord, u agent.Update, seen *model.Agent, now time.Time) {
	r.Agent = cmp.Or(u.Agent, r.Agent)
	r.Cwd = cmp.Or(u.Cwd, r.Cwd)
	r.Transcript = cmp.Or(u.Transcript, r.Transcript)
	if u.Clear {
		r.EndedAt = now
	} else {
		r.EndedAt = time.Time{}
		r.Status, r.StatusAt = string(u.Status), now
		if u.SetMessage {
			r.Message = u.Message
		}
		if u.SetPrompt {
			r.Prompt = u.Prompt
		}
		if u.StartTurn {
			r.TurnAt = now
		}
	}
	if seen != nil {
		see(r, seen)
	}
	r.UpdatedAt = now
}

// see keeps what a live window shows of its thread: its title, when it is
// one worth keeping, its session and its triage state.
func see(r *model.ThreadRecord, a *model.Agent) {
	if t := agent.ThreadTitle(a.Title, cmp.Or(r.Agent, a.Name)); t != "" {
		r.Title = t
	}
	r.Session, r.WindowID, r.Thread = a.Session, a.WindowID, a.Thread
}

// Kitty is kitty with the thread history: what the sidebar drives. Its
// ListAgents lists the closed threads too, after the live ones, and keeps
// the history up to date with what the live ones show; SetThread writes
// through to the history; and closed threads can be resumed or forgotten.
type Kitty struct {
	kitty.Kitty
	store *Store
	namer namer.Namer
	keep  Retention
	// Now is the clock, for tests.
	Now func() time.Time

	mu       sync.Mutex
	convs    map[int]string // each live window's conversation, as last listed
	prunedAt time.Time
	// resumed is when each conversation was last resumed, so asking again
	// before its agent is up doesn't open a second window.
	resumed map[string]time.Time
}

// resumeWait is how long a resumed thread is given to show up in its new
// window before resuming it again opens another.
const resumeWait = 20 * time.Second

// pruneEvery is how often ListAgents deletes the records past retention.
// They are left out of its list at once.
const pruneEvery = 10 * time.Minute

func NewKitty(k kitty.Kitty, store *Store, n namer.Namer, keep Retention) *Kitty {
	return &Kitty{Kitty: k, store: store, namer: n, keep: keep, Now: time.Now, convs: map[int]string{}, resumed: map[string]time.Time{}}
}

func (k *Kitty) ListAgents() ([]*model.Agent, error) {
	live, err := k.Kitty.ListAgents()
	if err != nil {
		return nil, err
	}
	records, err := k.store.List()
	if err != nil {
		slog.Debug("threads: couldn't read the thread history", "error", err)
		return live, nil
	}
	now := k.Now()
	open := map[string]bool{}
	convs := map[int]string{}
	for _, a := range live {
		if a.Conversation != "" {
			open[a.Conversation] = true
			convs[a.WindowID] = a.Conversation
		}
	}
	k.mu.Lock()
	k.convs = convs
	prune := now.Sub(k.prunedAt) >= pruneEvery
	if prune {
		k.prunedAt = now
	}
	k.mu.Unlock()

	k.sync(live, records)
	if expired := Expired(records, open, now, k.keep); len(expired) > 0 {
		if prune {
			for _, conv := range expired {
				if err := k.store.Forget(conv); err != nil {
					slog.Debug("threads: couldn't forget", "conversation", conv, "error", err)
				}
			}
		}
		records = slices.DeleteFunc(records, func(r model.ThreadRecord) bool { return slices.Contains(expired, r.Conversation) })
	}
	return agent.WithClosed(live, records), nil
}

// sync writes what the live windows show of their threads into their
// records, when it changed: the title the agent gave its window since its
// last hook event, a triage change made without the sidebar. Only records
// the hooks created are kept up to date.
func (k *Kitty) sync(live []*model.Agent, records []model.ThreadRecord) {
	byConv := map[string]model.ThreadRecord{}
	for _, r := range records {
		byConv[r.Conversation] = r
	}
	for _, a := range live {
		r, ok := byConv[a.Conversation]
		if a.Conversation == "" || !ok {
			continue
		}
		want := r
		see(&want, a)
		if Same(want, r) {
			continue
		}
		now := k.Now()
		err := k.store.Update(a.Conversation, func(rec *model.ThreadRecord) {
			see(rec, a)
			rec.UpdatedAt = now
		})
		if err != nil {
			slog.Debug("threads: couldn't update", "conversation", a.Conversation, "error", err)
		}
	}
}

// SetThread writes a thread's triage state onto its window, and into its
// record, so it is there when the window goes.
func (k *Kitty) SetThread(windowID int, t model.Thread) error {
	if err := k.Kitty.SetThread(windowID, t); err != nil {
		return err
	}
	k.mu.Lock()
	conv := k.convs[windowID]
	k.mu.Unlock()
	return k.KeepThread(conv, t)
}

// KeepThread writes a thread's triage state into its conversation's record,
// if the history has one.
func (k *Kitty) KeepThread(conversation string, t model.Thread) error {
	if _, ok := k.store.Get(conversation); !ok {
		return nil
	}
	now := k.Now()
	return k.store.Update(conversation, func(r *model.ThreadRecord) {
		r.Thread, r.UpdatedAt = t, now
	})
}

// ForgetThread drops a conversation from the thread history.
func (k *Kitty) ForgetThread(conversation string) error {
	return k.store.Forget(conversation)
}

// ResumeThread opens a closed thread again: a new window, running its agent
// on the same conversation in the directory it ran in. The window goes in
// the thread's session, as a new tab, when the session is still there, or
// starts the session again. Once the agent's hooks report the conversation,
// the window is the thread, and takes on its triage state (see
// agent.Resume). A conversation still open in a window is gone to instead.
func (k *Kitty) ResumeThread(conversation string) error {
	live, err := k.Kitty.ListAgents()
	if err != nil {
		return err
	}
	for _, a := range live {
		if a.Conversation == conversation {
			return k.Kitty.FocusWindow(a.WindowID)
		}
	}
	k.mu.Lock()
	at, again := k.resumed[conversation]
	k.mu.Unlock()
	if again && k.Now().Sub(at) < resumeWait {
		return fmt.Errorf("already resuming it, give it a moment")
	}
	rec, ok := k.store.Get(conversation)
	if !ok {
		return fmt.Errorf("no thread %s in the thread history", conversation)
	}
	command, ok := agent.ResumeCommand(rec.Agent, rec.Conversation)
	if !ok {
		return fmt.Errorf("can't resume a %s thread", cmp.Or(rec.Agent, "agent's"))
	}
	if rec.Cwd == "" {
		return fmt.Errorf("don't know where the thread ran")
	}
	if info, err := os.Stat(rec.Cwd); err != nil || !info.IsDir() {
		return fmt.Errorf("the thread's directory is gone: %s", rec.Cwd)
	}
	name := rec.Session
	if name == "" {
		if name, err = k.namer.Name(rec.Cwd); err != nil {
			return err
		}
	}
	sessions, err := k.Kitty.ListSessions()
	if err != nil {
		return err
	}
	var target string
	if slices.ContainsFunc(sessions, func(s *model.KittySession) bool { return s.Name == name }) {
		target, err = k.Kitty.NewWindowInSession(model.KittyTabOpts{TargetSession: name, StartDir: rec.Cwd})
	} else {
		target, err = k.Kitty.NewSession(name, rec.Cwd)
	}
	if err != nil {
		return fmt.Errorf("couldn't open a window to resume in: %w", err)
	}
	// Typed into the window's shell, as a session's startup command is, so
	// the agent runs with the shell's PATH and the window stays when it
	// exits.
	if _, err := k.Kitty.SendKeys(target, command); err != nil {
		return err
	}
	k.mu.Lock()
	k.resumed[conversation] = k.Now()
	k.mu.Unlock()
	_, err = k.Kitty.SwitchOrAttach(name, model.ConnectOpts{Switch: true})
	return err
}

// RetentionOf reads [sidebar]'s keep_closed and keep_closed_days.
func RetentionOf(cfg model.SidebarConfig) Retention {
	keep := Retention{Keep: DefaultKeep, MaxAge: DefaultMaxAge}
	if n := cfg.KeepClosed; n != nil {
		keep.Keep = max(*n, 0)
	}
	if days := cfg.KeepClosedDays; days != nil {
		keep.MaxAge = time.Duration(max(*days, 0)) * 24 * time.Hour
	}
	return keep
}
