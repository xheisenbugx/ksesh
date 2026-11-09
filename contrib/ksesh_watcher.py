# kitty watcher for ksesh: works around a kitty bug with
# `tab_bar_filter session:.` (seen in kitty 0.43–0.49).
#
# Install: copy this file to ~/.config/kitty/ and add to kitty.conf
#
#     watcher ksesh_watcher.py
#
# then restart kitty.
#
# The bug: Boss.active_session reads the active tab with `if t := self.active_tab`,
# but a Tab is falsy once it has no windows (it defines __len__). When the last
# window of a tab closes (cmd+w), kitty removes that now-empty tab while it is
# still the active one, so the active session reads as "" and the filter matches
# none of the session's other tabs. kitty then takes the tab for the session's
# last and focuses the most recent tab in its history, which is often in
# another session: closing a tab throws you into the session you were in before
# and, when that one has a single tab, hides the tab bar.


def _active_session(boss) -> str:
    t = boss.active_tab
    if t is None:
        return ''
    w = t.active_window
    return (w.created_in_session_name if w is not None else '') or t.created_in_session_name


def on_load(boss, data) -> None:
    from kitty.boss import Boss

    if isinstance(getattr(Boss, 'active_session', None), property):
        Boss.active_session = property(_active_session)
    _snapshot('on_load', boss, data)


# Agent status (see ksesh_agents.py, next to this file): redraw the tab bar and
# notify when an agent's status changes, and mark a finished agent seen once
# its window is focused. And kitty's state for ksesh's sidebars (see
# ksesh_snapshot.py), written whenever something they show changes. Both are
# re-read whenever they change, so editing them needs no restart; kitty only
# reads this file once (contrib/ksesh_reload.py reloads it).

import os

_DIR = os.path.dirname(os.path.abspath(__file__))
_AGENTS_PATH = os.path.join(_DIR, 'ksesh_agents.py')
_SNAPSHOT_PATH = os.path.join(_DIR, 'ksesh_snapshot.py')
_cache: dict = {}


def _module(path: str) -> dict:
    try:
        mtime = os.stat(path).st_mtime
    except OSError:
        return {}
    cached = _cache.get(path)
    if cached is None or cached[0] != mtime:
        import runpy

        try:
            ns = runpy.run_path(path)
        except Exception as e:
            from kitty.utils import log_error

            log_error(f'ksesh_watcher: failed to load {path}: {e}')
            ns = {}
        cached = _cache[path] = (mtime, ns)
    return cached[1]


def _agents() -> dict:
    return _module(_AGENTS_PATH)


def _call(name: str, boss, window, data) -> None:
    fn = _agents().get(name)
    if fn is not None:
        fn(boss, window, data)


def _snapshot(name: str, *args) -> None:
    # Never let the snapshot get in the way of kitty, or of the agent status.
    try:
        fn = _module(_SNAPSHOT_PATH).get(name)
        if fn is not None:
            fn(*args)
    except Exception as e:
        from kitty.utils import log_error

        log_error(f'ksesh_watcher: ksesh_snapshot.{name} failed: {e}')


def on_set_user_var(boss, window, data) -> None:
    try:
        _call('on_set_user_var', boss, window, data)
    finally:
        _snapshot('on_event', boss, window, data)


def on_focus_change(boss, window, data) -> None:
    # The snapshot first: ksesh_agents.py has the sidebar of a tab you go to
    # look again, and it reads the snapshot.
    _snapshot('on_focus_change', boss, window, data)
    _call('on_focus_change', boss, window, data)


def on_close(boss, window, data) -> None:
    try:
        _call('on_close', boss, window, data)
    finally:
        _snapshot('on_event', boss, window, data)


def on_title_change(boss, window, data) -> None:
    _snapshot('on_event', boss, window, data)


def on_tab_bar_dirty(boss, window, data) -> None:
    _snapshot('on_event', boss, window, data)


def on_cmd_startstop(boss, window, data) -> None:
    _snapshot('on_event', boss, window, data)


def on_quit(boss, window, data) -> None:
    _snapshot('on_quit', boss, window, data)
