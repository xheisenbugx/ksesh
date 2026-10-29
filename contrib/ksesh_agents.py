# Agent status in kitty for ksesh: tab badges, an attention strip and desktop
# notifications, driven by the user variables `ksesh agent hook` sets on each
# agent's window (ksesh_status, ksesh_msg, ksesh_prompt, …).
#
# It is loaded by ksesh_watcher.py (notifications, clearing "done" once seen)
# and by tab_bar.py (badges and the strip), both of which re-read it whenever
# it changes, so edits here apply without restarting kitty. See the README's
# "Agents" section for the kitty.conf and tab_bar.py lines it needs.
#
# With tab_bar_filter session:. kitty only shows the active session's tabs, so
# an agent in another session would be invisible. The strip fixes that: the
# right end of the tab bar lists every other session with an agent blocked on
# you, apart from the tabs, e.g.
#
#     1 code  2 [bell] web · api                needs you: [bell] auth  [warning] docs
from __future__ import annotations

import base64
import time
from typing import Any

STATUS_VAR = 'ksesh_status'
MSG_VAR = 'ksesh_msg'
PROMPT_VAR = 'ksesh_prompt'
TURN_AT_VAR = 'ksesh_turn_at'
NAME_VAR = 'ksesh_name'

# Most urgent first. The icons are single-cell Font Awesome glyphs from a nerd
# font, the same ones `ksesh list -i` uses; the colors are Catppuccin Mocha.
STATUSES: dict[str, tuple[str, str]] = {
    'waiting': ('', '#f9e2af'),  # bell, yellow
    'error': ('', '#f38ba8'),  # warning, red
    'done': ('', '#a6e3a1'),  # check, green
    'working': ('', '#89b4fa'),  # spinner, blue
}
PRIORITY = {s: len(STATUSES) - i for i, s in enumerate(STATUSES)}
# What the strip shows: agents blocked on you. Finished ones only notify, and
# wait in the picker.
STRIP = ('waiting', 'error')

# Desktop notifications for these, only when the agent's window isn't the one
# you're looking at (kitty's o=unfocused). A notification goes away once the
# agent moves on, e.g. when you approve it from the picker.
NOTIFY = {'waiting': 'needs you', 'error': 'failed', 'done': 'finished'}
# A turn shorter than this finishes without a notification: you were likely
# still around for a quick answer.
QUIET_TURN = 30
# How many sessions the strip names before summing up the rest as "+N".
STRIP_MAX = 4
STRIP_NAME_MAX = 14


def _status(window: Any) -> str:
    return window.user_vars.get(STATUS_VAR, '')


def _most_urgent(windows: Any) -> str:
    best = ''
    for w in windows:
        s = _status(w)
        if PRIORITY.get(s, 0) > PRIORITY.get(best, 0):
            best = s
    return best


def _sgr(hex_color: str) -> str:
    h = hex_color.lstrip('#')
    r, g, b = int(h[0:2], 16), int(h[2:4], 16), int(h[4:6], 16)
    return f'\x1b[38:2:{r}:{g}:{b}m'


def _icon(status: str, restore: str) -> str:
    icon, color = STATUSES[status]
    return f'{_sgr(color)}{icon}{restore}'


def session_of_tab(tab: Any) -> str:
    w = tab.active_window
    native = w.created_in_session_name if w is not None else ''
    return native or getattr(tab, 'created_in_session_name', '') or ''


def session_names(boss: Any) -> dict[str, str]:
    """The name ksesh shows for every kitty session, by kitty's own name."""
    names: dict[str, str] = {}
    for w in boss.all_windows:
        native = w.created_in_session_name
        if native and (name := w.user_vars.get(NAME_VAR)):
            names[native] = name
    return names


def tab_badge(tab: Any, restore: str) -> str:
    """The status icon of the tab's most urgent agent, and a space, or ''."""
    status = _most_urgent(tab)
    if status not in STATUSES:
        return ''
    return _icon(status, restore) + ' '


def attention(boss: Any, current_session: str) -> list[tuple[str, str]]:
    """(status, name) of every other session with an agent blocked on you,
    most urgent first, then longest waiting first."""
    by_session: dict[str, str] = {}
    since: dict[str, float] = {}
    for w in boss.all_windows:
        status = _status(w)
        native = w.created_in_session_name
        if status not in STRIP or not native or native == current_session:
            continue
        if PRIORITY[status] > PRIORITY.get(by_session.get(native, ''), 0):
            by_session[native] = status
        try:
            at = float(w.user_vars.get('ksesh_status_at', '0'))
        except ValueError:
            at = 0
        since[native] = min(since.get(native, at), at)
    names = session_names(boss)
    order = sorted(by_session, key=lambda s: (-PRIORITY[by_session[s]], since.get(s, 0)))
    return [(by_session[native], names.get(native, native)) for native in order]


def draw_strip(boss: Any, draw_data: Any, screen: Any) -> None:
    """Draw the attention strip at the right end of the tab bar, after the
    tabs are drawn. It is left out when the tabs leave no room for it."""
    from kitty.fast_data_types import wcswidth
    from kitty.tab_bar import as_rgb, color_as_int

    tm = boss.os_window_map.get(draw_data.os_window_id)
    if tm is None or tm.active_tab is None:
        return
    items = attention(boss, session_of_tab(tm.active_tab))
    if not items:
        return
    # (text, color) runs; None is the bar's own inactive color.
    runs: list[tuple[str, int | None]] = [('needs you: ', None)]
    for i, (status, name) in enumerate(items[:STRIP_MAX]):
        if len(name) > STRIP_NAME_MAX:
            name = name[: STRIP_NAME_MAX - 1] + '…'
        icon, color = STATUSES[status]
        runs += [('  ' if i else '', None), (icon, int(color.lstrip('#'), 16)), (f' {name}', None)]
    if len(items) > STRIP_MAX:
        runs.append((f'  +{len(items) - STRIP_MAX}', None))
    runs.append((' ', None))
    width = sum(wcswidth(text) for text, _ in runs)
    start = screen.columns - width
    if start < screen.cursor.x + 2:
        return
    screen.cursor.x = start
    screen.cursor.bold = screen.cursor.italic = False
    screen.cursor.bg = as_rgb(color_as_int(draw_data.default_bg))
    for text, color in runs:
        screen.cursor.fg = as_rgb(color if color is not None else color_as_int(draw_data.inactive_fg))
        screen.draw(text)


# Watcher side. ---------------------------------------------------------------

# The last notification each window got, while it may still be on screen.
_notified: dict[int, tuple[str, float]] = {}


def _redraw(boss: Any) -> None:
    for tm in boss.all_tab_managers:
        tm.mark_tab_bar_dirty()


def _notify(boss: Any, window: Any, status: str) -> None:
    # A status repeated within a few seconds (a permission prompt reported by
    # two hooks) is one notification.
    prev = _notified.get(window.id)
    now = time.monotonic()
    if prev is not None and prev[0] == status and now - prev[1] < 5:
        return
    _notified[window.id] = (status, now)

    native = window.created_in_session_name
    where = session_names(boss).get(native, native) or window.title
    title = f'{where}: agent {NOTIFY[status]}'
    body = window.user_vars.get(MSG_VAR) or window.user_vars.get(PROMPT_VAR) or ''

    def b64(s: str) -> str:
        return base64.standard_b64encode(s.encode('utf-8')).decode('ascii')

    # OSC 99, as if the agent's window had sent it: kitty then shows it only
    # while you aren't looking at that window, focuses the window when the
    # notification is clicked, and replaces the previous one from the same
    # window (same i=) instead of stacking them.
    ident = f'ksesh-{window.id}'
    meta = f'i={ident}:e=1:o=unfocused:a=focus'
    if status == 'waiting':
        meta += ':u=2'
    if body:
        window.desktop_notify(99, memoryview(f'{meta}:d=0:p=title;{b64(title)}'.encode()))
        window.desktop_notify(99, memoryview(f'i={ident}:e=1:d=1:p=body;{b64(body)}'.encode()))
    else:
        window.desktop_notify(99, memoryview(f'{meta}:d=1:p=title;{b64(title)}'.encode()))


def _close_notification(window: Any) -> None:
    if _notified.pop(window.id, None) is not None:
        window.desktop_notify(99, memoryview(f'i=ksesh-{window.id}:p=close;'.encode()))


def _is_watched(boss: Any, window: Any) -> bool:
    from kitty.fast_data_types import current_focused_os_window_id

    return boss.active_window is window and window.os_window_id == current_focused_os_window_id()


def _quick_turn(window: Any) -> bool:
    try:
        return time.time() - float(window.user_vars.get(TURN_AT_VAR, '')) < QUIET_TURN
    except ValueError:
        return False


def on_set_user_var(boss: Any, window: Any, data: dict[str, Any]) -> None:
    if data.get('key') != STATUS_VAR:
        return
    _redraw(boss)
    status = data.get('value') or ''
    if status == 'done' and _is_watched(boss, window):
        # It finished in front of you: already seen.
        window.set_user_var(STATUS_VAR, 'idle')
        return
    if status in NOTIFY and not (status == 'done' and _quick_turn(window)):
        _notify(boss, window, status)
    else:
        _close_notification(window)


def on_focus_change(boss: Any, window: Any, data: dict[str, Any]) -> None:
    # A finished agent you've looked at no longer needs you. Waiting and
    # failed ones still do, until they move on.
    if data.get('focused') and _status(window) == 'done':
        window.set_user_var(STATUS_VAR, 'idle')
        _close_notification(window)


def on_close(boss: Any, window: Any, data: dict[str, Any]) -> None:
    _notified.pop(window.id, None)
    if _status(window):
        _redraw(boss)
