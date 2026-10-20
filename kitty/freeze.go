package kitty

import (
	"errors"
	"strconv"
)

// Without a listen_on socket, `kitten @` talks to kitty over the terminal it
// runs in: it writes a request to the tty and reads kitty's reply back from
// it. A TUI owns that same tty and reads everything that arrives on it, so a
// remote control call made while the picker is up races the picker for
// kitty's reply, and one of them gets garbage.
//
// Freeze sidesteps that. It reads everything the picker could need before the
// TUI starts, and until Thaw serves reads from that snapshot instead of
// calling kitty. Killing a session while frozen drops it from the snapshot at
// once and closes its windows on Thaw, once the TUI has let go of the tty.
//
// Over a socket there is no race, so Freeze does nothing.

var errFrozen = errors.New("kitty remote control is paused while the picker owns the terminal")

// ttyMode reports whether remote control goes over the tty.
func (k *RealKitty) ttyMode() bool {
	return k.IsAttached() && k.os.Getenv("KITTY_LISTEN_ON") == ""
}

// Freeze snapshots kitty for a TUI, capturing every session's screen as well
// when captures is set (for the picker's preview pane).
func (k *RealKitty) Freeze(captures bool) error {
	if !k.ttyMode() {
		return nil
	}
	k.invalidate()
	st, err := k.load()
	if err != nil {
		return err
	}
	snapshot := map[int]string{}
	if captures {
		for _, sess := range st.sessions {
			out, err := k.rc("get-text", "--ansi", "--match", "id:"+strconv.Itoa(sess.focusID))
			if err == nil {
				snapshot[sess.focusID] = out
			}
		}
	}
	k.mu.Lock()
	k.frozen = true
	k.captures = snapshot
	k.mu.Unlock()
	return nil
}

// Thaw resumes remote control and carries out what was deferred while frozen.
func (k *RealKitty) Thaw() error {
	k.mu.Lock()
	if !k.frozen {
		k.mu.Unlock()
		return nil
	}
	k.frozen = false
	k.captures = nil
	pending := k.pendingKills
	k.pendingKills = nil
	k.state = nil
	k.mu.Unlock()

	var errs []error
	for _, ids := range pending {
		if _, err := k.mutate("close-window", "--ignore-no-match", "--match", matchIDs(ids)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (k *RealKitty) isFrozen() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.frozen
}

// frozenCapture serves CapturePane from the snapshot.
func (k *RealKitty) frozenCapture(sess *sessionState) string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.captures[sess.focusID]
}

// deferKill drops a session from the snapshot and queues its windows to be
// closed on Thaw.
func (k *RealKitty) deferKill(sess *sessionState) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.pendingKills = append(k.pendingKills, sess.windowIDs())
	if k.state == nil {
		return
	}
	kept := k.state.sessions[:0:0]
	for _, s := range k.state.sessions {
		if s != sess {
			kept = append(kept, s)
		}
	}
	k.state.sessions = kept
	if k.state.current == sess {
		k.state.current = nil
	}
}
