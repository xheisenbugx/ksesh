# Tab titles for ksesh, used as {custom} in kitty.conf's tab_title_template:
#
#     tab_title_template "{custom}"
#
# - A tab is titled with its number ("2"); one renamed with set_tab_title shows
#   its number and name ("2 database"), cut to 20 characters.
# - A tab with an agent shows its status icon (see ksesh_agents.py, which must
#   be next to this file): "2 [bell] api".
# - The active tab also names its session, using the name ksesh shows (kept in
#   the ksesh_name user variable for sessions kitty can't name directly), and
#   then every other session with an agent that needs you.
#
# kitty reads this file once: run `kitten @ kitten ksesh_reload.py` (or restart
# kitty) after editing it. ksesh_agents.py is re-read on its own when it changes.
import os

from kitty.fast_data_types import get_boss

MAX_NAME = 20

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
        except Exception:
            _agents_cache['ns'] = {}
        _agents_cache['mtime'] = mtime
    return _agents_cache['ns']


def _short(tab, index) -> str:
    name = tab.name.strip()
    w = tab.active_window
    # A name that is just the shell's own title (a rename prompt submitted
    # unchanged) isn't a real name.
    if not name or (w is not None and name == w.title):
        return str(index)
    if len(name) > MAX_NAME:
        name = name[:MAX_NAME - 1] + '…'
    return f'{index} {name}'


def _session(tab) -> str:
    w = tab.active_window
    native = w.created_in_session_name if w is not None else ''
    if not native:
        return ''
    # Only the windows ksesh created carry the name it shows; tabs opened with
    # cmd+t don't, so look across the whole session.
    for win in get_boss().all_windows:
        if win.created_in_session_name == native and (name := win.user_vars.get('ksesh_name')):
            return name
    return native


def draw_title(data) -> str:
    boss = get_boss()
    tab = boss.tab_for_id(data['tab_id'])
    if tab is None:
        return data['title']
    agents = _agents()
    # The SGR that puts the tab's own foreground back after a colored icon.
    restore = data['fmt'].fg.tab
    title = _short(tab, data['index'])
    if (badge := agents.get('tab_badge')) is not None:
        b = badge(tab, restore)
        if b:
            index, _, rest = title.partition(' ')
            title = f'{index} {b}{rest}'.rstrip()
    tm = tab.tab_manager_ref()
    if tm is not None and tm.active_tab is tab:
        if session := _session(tab):
            title += f' · {session}'
        if (strip := agents.get('attention_strip')) is not None:
            title += strip(boss, agents['session_of_tab'](tab), restore)
    return title
