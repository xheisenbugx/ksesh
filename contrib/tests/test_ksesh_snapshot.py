# Tests for contrib/ksesh_snapshot.py with kitty's modules stubbed out:
#
#     python3 -m unittest discover -s contrib/tests
#
# (or `just test-contrib`).
from __future__ import annotations

import json
import os
import runpy
import sys
import tempfile
import types
import unittest
from contextlib import contextmanager
from typing import Any

CONTRIB = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SNAPSHOT = os.path.join(CONTRIB, 'ksesh_snapshot.py')
WATCHER = os.path.join(CONTRIB, 'ksesh_watcher.py')


class Timers:
    """kitty's add_timer, run by hand."""

    def __init__(self) -> None:
        self.timers: dict[int, tuple[Any, float, bool]] = {}
        self.next_id = 1

    def add_timer(self, callback: Any, interval: float, repeats: bool) -> int:
        tid = self.next_id
        self.next_id += 1
        self.timers[tid] = (callback, interval, repeats)
        return tid

    def remove_timer(self, tid: int) -> None:
        self.timers.pop(tid, None)

    def one_shots(self) -> list[int]:
        return [tid for tid, (_, _, repeats) in self.timers.items() if not repeats]

    def repeating(self) -> list[int]:
        return [tid for tid, (_, _, repeats) in self.timers.items() if repeats]

    def fire(self, tid: int) -> None:
        callback, _, repeats = self.timers[tid]
        if not repeats:
            del self.timers[tid]
        callback(tid)


class Child:
    def __init__(self, cwd: str) -> None:
        self.root_pid = 100
        self.current_cwd = cwd
        self.cwd = '/'
        self.cmdline = ['/bin/zsh']
        self.foreground_processes = [{'pid': 101, 'cmdline': ['claude'], 'cwd': cwd}]


class Window:
    def __init__(self, wid: int, os_window_id: int = 1, **user_vars: str) -> None:
        self.id = wid
        self.os_window_id = os_window_id
        self.title = f'window {wid}'
        self.child = Child(f'/c/{wid}')
        self.last_focused_at = float(wid)
        self.created_at = 1700000000000000000 + wid
        self.created_in_session_name = 'api'
        self.user_vars = dict(user_vars)
        self.needs_attention = False
        self.has_activity_since_last_focus = False


class Tab:
    def __init__(self, tid: int, windows: list[Window], os_window_id: int = 1) -> None:
        self.id = tid
        self.os_window_id = os_window_id
        self.windows = windows
        self.active_window = windows[0] if windows else None
        self.name = ''
        self.title = f'tab {tid}'

    def __iter__(self) -> Any:
        return iter(self.windows)


class TabManager:
    def __init__(self, tabs: list[Tab]) -> None:
        self.tabs = tabs
        self.active_tab = tabs[0]

    def __iter__(self) -> Any:
        return iter(self.tabs)

    def mark_tab_bar_dirty(self) -> None:
        pass


class Boss:
    def __init__(self) -> None:
        sidebar = Window(9, ksesh_role='sidebar')
        self.tm = TabManager([Tab(1, [Window(1, ksesh_status='working'), sidebar]), Tab(2, [Window(2)]), Tab(3, [])])
        self.os_window_map = {1: self.tm}
        self.active_tab_manager = self.tm
        self.all_tab_managers = [self.tm]


@contextmanager
def stubbed_kitty(timers: Timers, boss: Boss, errors: list[str]) -> Any:
    @contextmanager
    def cached_process_data() -> Any:
        yield

    kitty = types.ModuleType('kitty')
    fdt = types.ModuleType('kitty.fast_data_types')
    fdt.add_timer = timers.add_timer  # type: ignore[attr-defined]
    fdt.remove_timer = timers.remove_timer  # type: ignore[attr-defined]
    fdt.get_boss = lambda: boss  # type: ignore[attr-defined]
    fdt.current_focused_os_window_id = lambda: 1  # type: ignore[attr-defined]
    child = types.ModuleType('kitty.child')
    child.cached_process_data = cached_process_data  # type: ignore[attr-defined]
    utils = types.ModuleType('kitty.utils')
    utils.log_error = errors.append  # type: ignore[attr-defined]
    stubs = {'kitty': kitty, 'kitty.fast_data_types': fdt, 'kitty.child': child, 'kitty.utils': utils}
    saved = {name: sys.modules.get(name) for name in stubs}
    sys.modules.update(stubs)
    try:
        yield
    finally:
        for name, mod in saved.items():
            if mod is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = mod


class SnapshotTest(unittest.TestCase):
    def setUp(self) -> None:
        self.dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.dir.cleanup)
        self.env = {'XDG_STATE_HOME': os.environ.get('XDG_STATE_HOME')}
        os.environ['XDG_STATE_HOME'] = self.dir.name
        self.addCleanup(self.restore_env)
        self.timers = Timers()
        self.boss = Boss()
        self.errors: list[str] = []
        stubs = stubbed_kitty(self.timers, self.boss, self.errors)
        stubs.__enter__()
        self.addCleanup(stubs.__exit__, None, None, None)
        self.ns = runpy.run_path(SNAPSHOT)
        self.path = os.path.join(self.dir.name, 'ksesh', f'kitty-{os.getpid()}.json')

    def restore_env(self) -> None:
        if self.env['XDG_STATE_HOME'] is None:
            os.environ.pop('XDG_STATE_HOME', None)
        else:
            os.environ['XDG_STATE_HOME'] = self.env['XDG_STATE_HOME']

    def read(self) -> dict[str, Any]:
        with open(self.path) as f:
            return json.load(f)

    def test_serializes_like_ls(self) -> None:
        text, body = self.ns['serialize'](self.boss, 1700000000.5)
        data = json.loads(text)
        self.assertEqual(data['version'], 1)
        self.assertEqual(data['pid'], os.getpid())
        self.assertEqual(data['written_at'], 1700000000.5)
        self.assertEqual(json.loads(body), data['os_windows'])

        [osw] = data['os_windows']
        self.assertEqual((osw['id'], osw['is_active'], osw['is_focused']), (1, True, True))
        self.assertEqual([t['id'] for t in osw['tabs']], [1, 2], 'a tab without windows is left out, as ls does')
        tab = osw['tabs'][0]
        self.assertEqual((tab['is_active'], tab['is_focused'], tab['title']), (True, True, 'tab 1'))
        self.assertEqual([w['id'] for w in tab['windows']], [1, 9], 'docks are among the windows')
        w = tab['windows'][0]
        self.assertTrue(w['is_active'] and w['is_focused'])
        self.assertFalse(tab['windows'][1]['is_active'])
        self.assertEqual(w['cwd'], '/c/1')
        self.assertEqual(w['session_name'], 'api')
        self.assertEqual(w['user_vars'], {'ksesh_status': 'working'})
        self.assertEqual(w['foreground_processes'][0]['cmdline'], ['claude'])
        self.assertEqual(w['created_at'], 1700000000000000001)
        self.assertNotIn('env', w)
        # As in ls, a window's is_focused only says it is its tab's active
        # window in the focused OS window, whether the tab is active or not.
        self.assertTrue(osw['tabs'][1]['windows'][0]['is_focused'])
        self.assertFalse(osw['tabs'][1]['is_focused'])

    def test_falls_back_on_kittys_listing(self) -> None:
        def list_os_windows() -> Any:
            yield {'id': 1, 'tabs': [{'id': 1, 'layout_state': {}, 'windows': [{'id': 1, 'env': {'TOKEN': 'x'}, 'neighbors': {}}]}]}

        self.boss.list_os_windows = list_os_windows  # type: ignore[attr-defined]
        del self.boss.tm.tabs[0].windows[0].child  # something kitty moved
        _, body = self.ns['serialize'](self.boss)
        self.assertEqual(json.loads(body), [{'id': 1, 'tabs': [{'id': 1, 'windows': [{'id': 1}]}]}], 'without the environment')

    def test_debounces_events(self) -> None:
        for _ in range(5):
            self.ns['on_event'](self.boss, None, {})
        self.assertEqual(len(self.timers.one_shots()), 1, 'a burst is one write')
        self.assertEqual(len(self.timers.repeating()), 1, 'and the heartbeat starts')
        [tid] = self.timers.one_shots()
        self.assertEqual(self.timers.timers[tid][1], self.ns['DEBOUNCE'])
        self.assertFalse(os.path.exists(self.path))

        self.timers.fire(tid)
        data = self.read()
        self.assertEqual(data['os_windows'][0]['tabs'][0]['windows'][0]['id'], 1)
        self.assertEqual(oct(os.stat(self.path).st_mode & 0o777), oct(0o600))
        self.assertFalse(os.path.exists(self.path + '.tmp'))

        # Right after a write, the next one waits out MIN_INTERVAL.
        self.ns['on_event'](self.boss, None, {})
        [tid] = self.timers.one_shots()
        self.assertGreater(self.timers.timers[tid][1], self.ns['DEBOUNCE'])
        self.assertLessEqual(self.timers.timers[tid][1], self.ns['MIN_INTERVAL'])

    def test_unchanged_state_is_not_rewritten_but_the_heartbeat_is(self) -> None:
        self.ns['on_event'](self.boss, None, {})
        self.timers.fire(self.timers.one_shots()[0])
        os.utime(self.path, (0, 0))

        self.ns['on_event'](self.boss, None, {})
        self.timers.fire(self.timers.one_shots()[0])
        self.assertEqual(os.stat(self.path).st_mtime, 0, 'nothing changed: the sidebars stay asleep')

        self.boss.tm.tabs[1].windows[0].user_vars['ksesh_status'] = 'waiting'
        self.ns['on_event'](self.boss, None, {})
        self.timers.fire(self.timers.one_shots()[0])
        self.assertNotEqual(os.stat(self.path).st_mtime, 0)
        self.assertEqual(self.read()['os_windows'][0]['tabs'][1]['windows'][0]['user_vars'], {'ksesh_status': 'waiting'})

        os.utime(self.path, (0, 0))
        self.timers.fire(self.timers.repeating()[0])
        self.assertNotEqual(os.stat(self.path).st_mtime, 0, 'the heartbeat says kitty keeps it up')

    def test_survives_being_reread(self) -> None:
        self.ns['on_load'](self.boss, {})
        [heartbeat] = self.timers.repeating()
        fresh = runpy.run_path(SNAPSHOT)
        fresh['on_load'](self.boss, {})
        self.assertEqual(self.timers.repeating(), [heartbeat], 'still one heartbeat')

        calls: list[bool] = []
        st = self.boss._ksesh_snapshot  # type: ignore[attr-defined]
        self.assertIs(st['write'], fresh['write'])
        st['write'] = lambda boss, force=False: calls.append(force)
        self.timers.fire(heartbeat)  # set by the old copy
        self.assertEqual(calls, [True], 'old timers call the new code')

    def test_a_failed_write_removes_the_snapshot(self) -> None:
        self.ns['write'](self.boss)
        self.assertTrue(os.path.exists(self.path))
        self.boss.os_window_map = None  # type: ignore[assignment]
        self.boss.list_os_windows = lambda: 1 / 0  # type: ignore[attr-defined]
        for _ in range(10):
            self.ns['write'](self.boss, True)
        self.assertFalse(os.path.exists(self.path), 'ksesh asks kitty instead')
        self.assertEqual(len(self.errors), self.ns['MAX_LOGGED'], 'logged sparingly')

    def test_quit_removes_the_snapshot(self) -> None:
        self.ns['on_load'](self.boss, {})
        self.ns['write'](self.boss)
        self.ns['on_quit'](self.boss, None, {'confirmed': False})
        self.assertTrue(os.path.exists(self.path), 'not quitting yet')
        self.ns['on_quit'](self.boss, None, {'confirmed': True})
        self.assertFalse(os.path.exists(self.path))
        self.assertEqual(self.timers.timers, {})

    def test_load_cleans_up_after_dead_kittys(self) -> None:
        os.makedirs(os.path.dirname(self.path), exist_ok=True)
        dead = os.path.join(os.path.dirname(self.path), 'kitty-999999.json')
        alive = os.path.join(os.path.dirname(self.path), f'kitty-{os.getppid()}.json')
        for p in (dead, alive):
            with open(p, 'w') as f:
                f.write('{}')
        self.ns['on_load'](self.boss, {})
        self.assertFalse(os.path.exists(dead))
        self.assertTrue(os.path.exists(alive))

    def test_the_watcher_forwards_events(self) -> None:
        watcher = runpy.run_path(WATCHER)
        window = self.boss.tm.tabs[0].windows[0]
        for name in ('on_title_change', 'on_tab_bar_dirty', 'on_cmd_startstop', 'on_close', 'on_set_user_var'):
            self.timers.timers.clear()
            self.boss.__dict__.pop('_ksesh_snapshot', None)
            watcher[name](self.boss, window, {'key': 'ksesh_pin', 'value': '1'})
            self.assertEqual(len(self.timers.one_shots()), 1, name)

    def test_focus_writes_at_once(self) -> None:
        self.ns['on_focus_change'](self.boss, None, {'focused': False})
        self.assertFalse(os.path.exists(self.path), 'leaving a window waits for the debounce')
        self.ns['on_focus_change'](self.boss, None, {'focused': True})
        self.assertTrue(os.path.exists(self.path), 'before the sidebar is told to look')

    def test_the_watcher_writes_before_the_sidebar_is_told(self) -> None:
        watcher = runpy.run_path(WATCHER)
        seen: list[bool] = []
        agents = watcher['_agents']()
        orig = agents['on_focus_change']
        agents['on_focus_change'] = lambda boss, window, data: seen.append(os.path.exists(self.path))
        try:
            watcher['on_focus_change'](self.boss, self.boss.tm.tabs[0].windows[0], {'focused': True})
        finally:
            agents['on_focus_change'] = orig
        self.assertEqual(seen, [True])

    def test_a_broken_snapshot_never_reaches_kitty(self) -> None:
        watcher = runpy.run_path(WATCHER)
        self.boss.__dict__['_ksesh_snapshot'] = {}
        sys.modules['kitty.fast_data_types'].add_timer = None  # type: ignore[attr-defined]
        watcher['on_title_change'](self.boss, None, {})
        self.assertTrue(self.errors, 'logged instead')


if __name__ == '__main__':
    unittest.main()
