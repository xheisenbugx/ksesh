# Reloads tab_bar.py and the kitty.conf watchers into a running kitty, for
# ksesh's kitty-side files. kitty reads both once and caches them for good
# (load-config doesn't re-read either), so without this a new tab_bar.py or
# watcher only takes effect after quitting kitty, and with it every session.
#
# Install: copy next to kitty.conf, then after changing tab_bar.py or a watcher:
#
#     kitten @ kitten ksesh_reload.py
#
# It pokes at kitty internals (the caches in kitty.tab_bar and kitty.launch),
# checked against kitty 0.43-0.49; on a kitty where they moved it logs what it
# couldn't do and changes nothing else. Restarting kitty always works.
from typing import Any

from kittens.tui.handler import result_handler


def main(args: list[str]) -> str:
    return ''


def _reload_tab_bar() -> None:
    from kitty import tab_bar

    for name in ('load_custom_draw_tab_module', 'load_custom_draw_tab'):
        fn = getattr(tab_bar, name, None)
        if fn is not None and hasattr(fn, 'clear_cached'):
            fn.clear_cached()


def _reload_watchers(boss: Any) -> int:
    from kitty.fast_data_types import get_options
    from kitty.launch import watcher_modules
    from kitty.utils import resolve_custom_file
    from kitty.window import global_watchers

    paths = {resolve_custom_file(p) for p in get_options().watcher}
    old = {p: watcher_modules.pop(p) for p in list(watcher_modules) if p in paths}
    old_funcs = {id(f) for m in old.values() if m for f in m.values() if callable(f)}
    # Forces GlobalWatchers to load the modules again on its next call, which
    # also runs their on_load.
    global_watchers.options_spec = None
    fresh = global_watchers()
    attrs = [a for a in vars(fresh) if a.startswith('on_') and isinstance(getattr(fresh, a), list)]
    count = 0
    for w in boss.all_windows:
        for attr in attrs:
            callbacks = getattr(w.watchers, attr)
            callbacks[:] = [f for f in callbacks if id(f) not in old_funcs]
        w.watchers.add(fresh)
        count += 1
    return count


@result_handler(no_ui=True)
def handle_result(args: list[str], answer: str, target_window_id: int, boss: Any) -> None:
    from kitty.utils import log_error

    try:
        _reload_tab_bar()
    except Exception as e:
        log_error(f'ksesh_reload: could not reload tab_bar.py: {e}')
    try:
        n = _reload_watchers(boss)
    except Exception as e:
        log_error(f'ksesh_reload: could not reload watchers: {e}')
        n = 0
    for tm in boss.all_tab_managers:
        tm.mark_tab_bar_dirty()
    log_error(f'ksesh_reload: reloaded tab_bar.py and the watchers of {n} windows')
