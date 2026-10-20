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


# Agent status (see ksesh_agents.py, next to this file): redraw the tab bar and
# notify when an agent's status changes, and mark a finished agent seen once
# its window is focused. ksesh_agents.py is re-read whenever it changes, so
# editing it needs no restart; kitty only reads this file once.

import os

_AGENTS_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'ksesh_agents.py')
_agents_cache: dict = {}


def _agents() -> dict:
    try:
        mtime = os.stat(_AGENTS_PATH).st_mtime
    except OSError:
        return {}
    if _agents_cache.get('mtime') != mtime:
        import runpy

        try:
            _agents_cache['ns'] = runpy.run_path(_AGENTS_PATH)
        except Exception as e:
            from kitty.utils import log_error

            log_error(f'ksesh_watcher: failed to load {_AGENTS_PATH}: {e}')
            _agents_cache['ns'] = {}
        _agents_cache['mtime'] = mtime
    return _agents_cache['ns']


def _call(name: str, boss, window, data) -> None:
    fn = _agents().get(name)
    if fn is not None:
        fn(boss, window, data)


def on_set_user_var(boss, window, data) -> None:
    _call('on_set_user_var', boss, window, data)


def on_focus_change(boss, window, data) -> None:
    _call('on_focus_change', boss, window, data)


def on_close(boss, window, data) -> None:
    _call('on_close', boss, window, data)
