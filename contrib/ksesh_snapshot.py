# kitty's state as `kitten @ ls` would give it, kept in a file for ksesh's
# sidebars, so they don't each run `kitten @ ls` every second.
#
# Loaded by ksesh_watcher.py, which re-reads it whenever it changes. kitty calls
# the watcher on what the sidebars show: a user variable set (an agent's status,
# a thread's triage), a title change, the focus moving, a window closing, the
# tab bar changing (tabs and windows opening, closing, moving), a command
# starting or stopping. Each schedules a write a moment later, so a burst of
# them (`ksesh agent hook` sets several variables at once) is one write. A
# heartbeat rewrites it every couple of seconds too, for what changes without
# an event (the program running in a window), and so ksesh can tell kitty is
# still keeping it up: it trusts the file only while it is recent, and asks
# kitty with `kitten @ ls` otherwise (see kitty/snapshot.go in ksesh).
#
# The file is $XDG_STATE_HOME/ksesh/kitty-<pid>.json (~/.local/state by
# default), kitty's pid being what its windows see as $KITTY_PID:
#
#     {"version": 1, "pid": 4242, "written_at": 1700000000.25, "os_windows": [...]}
#
# os_windows is `kitten @ ls` less what ksesh doesn't read: no environment
# variables (they are slow to read, and secrets have no business on disk), no
# layouts.
from __future__ import annotations

import json
import os
import time
from typing import Any

VERSION = 1
# A burst of events within this many seconds is written once.
DEBOUNCE = 0.1
# And writes are at least this far apart, however busy kitty is (an agent
# animating its title while it works changes it several times a second).
MIN_INTERVAL = 0.25
# How often it is rewritten without any event. ksesh trusts it for 6 seconds.
HEARTBEAT = 2.0
# The most errors logged to kitty's log before going quiet.
MAX_LOGGED = 5


def _state_dir() -> str:
    base = os.environ.get('XDG_STATE_HOME') or os.path.join(os.path.expanduser('~'), '.local', 'state')
    return os.path.join(base, 'ksesh')


def snapshot_path(pid: int | None = None) -> str:
    return os.path.join(_state_dir(), f'kitty-{os.getpid() if pid is None else pid}.json')


# What survives this file being re-read: kitty's timers outlive the module that
# set them, so they call through here to the newest code. Kept on the boss.
def _shared(boss: Any) -> dict[str, Any]:
    st = getattr(boss, '_ksesh_snapshot', None)
    if not isinstance(st, dict):
        st = {}
        try:
            boss._ksesh_snapshot = st
        except Exception:
            st = _fallback_shared
    return st


def _register(boss: Any) -> dict[str, Any]:
    """Make this copy of the module the one timers call: the watcher only
    calls the newest."""
    st = _shared(boss)
    st['write'] = write
    return st


_fallback_shared: dict[str, Any] = {}


def _log(st: dict[str, Any], msg: str) -> None:
    n = st.get('logged', 0)
    if n >= MAX_LOGGED:
        return
    st['logged'] = n + 1
    try:
        from kitty.utils import log_error

        log_error(f'ksesh_snapshot: {msg}' + (' (no more errors will be logged)' if n + 1 == MAX_LOGGED else ''))
    except Exception:
        pass


# Serialization. ---------------------------------------------------------------


def _window(w: Any, is_active: bool, is_focused: bool) -> dict[str, Any]:
    """A window as `kitten @ ls` lists it (kitty's Window.as_dict), the fields
    ksesh reads."""
    child = w.child
    return {
        'id': w.id,
        'is_focused': is_focused,
        'is_active': is_active,
        'title': w.title,
        'pid': child.root_pid,
        'cwd': child.current_cwd or child.cwd,
        'cmdline': child.cmdline,
        'last_focused_at': w.last_focused_at,
        'created_at': w.created_at,
        'session_name': w.created_in_session_name,
        'user_vars': dict(w.user_vars),
        'needs_attention': w.needs_attention,
        'has_activity_since_last_focus': w.has_activity_since_last_focus,
        'foreground_processes': child.foreground_processes,
    }


def _walk(boss: Any, focused_os_window: int) -> list[dict[str, Any]]:
    """kitty's OS windows, tabs and windows, as kitty's Boss.list_os_windows,
    TabManager.list_tabs and Tab.list_windows have them."""
    active_tm = boss.active_tab_manager
    os_windows = []
    for os_window_id, tm in boss.os_window_map.items():
        active_tab = tm.active_tab
        tabs = []
        for tab in tm:
            active_window = tab.active_window
            windows = [
                _window(w, w is active_window, w is active_window and w.os_window_id == focused_os_window)
                for w in tab  # a tab's docks, so the sidebar, included
            ]
            if windows:
                tabs.append(
                    {
                        'id': tab.id,
                        'is_focused': tab is active_tab and tab.os_window_id == focused_os_window,
                        'is_active': tab is active_tab,
                        'title': tab.name or tab.title,
                        'windows': windows,
                    }
                )
        if tabs:
            os_windows.append(
                {
                    'id': os_window_id,
                    'is_active': tm is active_tm,
                    'is_focused': os_window_id == focused_os_window,
                    'tabs': tabs,
                }
            )
    return os_windows


# What ksesh doesn't read, dropped when falling back on kitty's own listing.
_DROP = ('env', 'neighbors', 'layout_state', 'layout_opts', 'enabled_layouts', 'groups')


def _prune(d: Any) -> Any:
    if isinstance(d, dict):
        return {k: _prune(v) for k, v in d.items() if k not in _DROP}
    if isinstance(d, list):
        return [_prune(v) for v in d]
    return d


def os_windows(boss: Any) -> list[dict[str, Any]]:
    from kitty.child import cached_process_data
    from kitty.fast_data_types import current_focused_os_window_id

    # Reads every process once for all windows, as `ls` does.
    with cached_process_data():
        try:
            return _walk(boss, current_focused_os_window_id())
        except Exception:
            # kitty moved something this relies on: its own listing is slower
            # (it reads every window's environment) but always there.
            return _prune(list(boss.list_os_windows()))


def _dumps(data: Any) -> str:
    return json.dumps(data, separators=(',', ':'), ensure_ascii=False, default=str)


def serialize(boss: Any, now: float | None = None) -> tuple[str, str]:
    """The snapshot, and the part of it that is kitty's state."""
    # The time first: the state is read after it, so it is at least as new.
    written_at = time.time() if now is None else now
    body = _dumps(os_windows(boss))
    head = _dumps({'version': VERSION, 'pid': os.getpid(), 'written_at': written_at})
    return head[:-1] + ',"os_windows":' + body + '}', body


def _write_file(path: str, text: str) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    # Atomically: sidebars read it at any time.
    tmp = f'{path}.tmp'
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    try:
        os.write(fd, text.encode('utf-8'))
    finally:
        os.close(fd)
    os.replace(tmp, path)


def write(boss: Any, force: bool = False) -> None:
    """Write the snapshot if kitty changed since the last one, or anyway when
    forced (the heartbeat: it says kitty is keeping it up)."""
    st = _shared(boss)
    path = snapshot_path()
    try:
        text, body = serialize(boss)
        st['last_write'] = time.monotonic()
        if not force and body == st.get('last_body'):
            return  # nothing the sidebars would see: don't wake them
        _write_file(path, text)
        st['last_body'] = body
    except Exception as e:
        st['last_body'] = None
        _log(st, f'could not write {path}: {e!r}')
        # Better none than a wrong one: ksesh asks kitty then.
        try:
            os.remove(path)
        except OSError:
            pass


# Scheduling. ------------------------------------------------------------------


def _add_timer(callback: Any, interval: float, repeats: bool) -> Any:
    from kitty.fast_data_types import add_timer

    return add_timer(callback, interval, repeats)


def _get_boss() -> Any:
    from kitty.fast_data_types import get_boss

    return get_boss()


def _fire(timer_id: Any = None) -> None:
    try:
        boss = _get_boss()
        st = _shared(boss)
        st['pending'] = None
        st['write'](boss)
    except Exception as e:
        _log(_fallback_shared, f'write failed: {e!r}')


def _beat(timer_id: Any = None) -> None:
    try:
        boss = _get_boss()
        _shared(boss)['write'](boss, True)
    except Exception as e:
        _log(_fallback_shared, f'heartbeat failed: {e!r}')


def schedule(boss: Any) -> None:
    """Write the snapshot in a moment, once for every event until then."""
    try:
        st = _register(boss)
        if st.get('pending') is None:
            since = time.monotonic() - st.get('last_write', 0.0)
            st['pending'] = _add_timer(_fire, max(DEBOUNCE, MIN_INTERVAL - since), False)
        if st.get('heartbeat') is None:
            st['heartbeat'] = _add_timer(_beat, HEARTBEAT, True)
    except Exception as e:
        _log(_fallback_shared, f'could not schedule a write: {e!r}')


def _clean_stale() -> None:
    """Remove the snapshots of kittys that are gone."""
    import glob

    for path in glob.glob(os.path.join(_state_dir(), 'kitty-*.json')):
        try:
            pid = int(os.path.basename(path)[len('kitty-') : -len('.json')])
        except ValueError:
            continue
        if pid == os.getpid():
            continue
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            try:
                os.remove(path)
            except OSError:
                pass
        except OSError:
            pass  # alive, someone else's


# Watcher callbacks, called by ksesh_watcher.py. ---------------------------------


def on_load(boss: Any, data: dict[str, Any]) -> None:
    try:
        _clean_stale()
    except Exception:
        pass
    schedule(boss)


def on_event(boss: Any, window: Any, data: dict[str, Any]) -> None:
    schedule(boss)


def on_focus_change(boss: Any, window: Any, data: dict[str, Any]) -> None:
    if not data.get('focused'):
        schedule(boss)
        return
    # At once: the watcher calls this before ksesh_agents.py tells the
    # sidebar of the tab you went to to look again, and what it reads has to
    # say its tab is on screen now.
    try:
        _register(boss)
        write(boss)
    except Exception as e:
        _log(_fallback_shared, f'write failed: {e!r}')
    schedule(boss)


def on_quit(boss: Any, window: Any, data: dict[str, Any]) -> None:
    if not data.get('confirmed'):
        return
    try:
        st = _shared(boss)
        from kitty.fast_data_types import remove_timer

        for name in ('pending', 'heartbeat'):
            if st.get(name) is not None:
                remove_timer(st[name])
                st[name] = None
        os.remove(snapshot_path())
    except Exception:
        pass
