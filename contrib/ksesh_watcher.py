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
