#!/usr/bin/env python3
"""X11 real-input/presented-pixel verification, not an editor control endpoint.

Requires libX11, libXtst, xdotool, xclip and Pillow (screenshots only). Start
main.go separately, then run this with --file INPUT --output OUTSIDE_REPO.
Uses XTest key/button/motion events; no DispatchEvent, CommitText, SetSelection
or model mutation. Clipboard setup, assertions and screenshots are outside the
measurement window. Timing starts just before XFlush of real desktop input,
ends at XGetImage observation of the new, non-pending painted status. This is
an X11 desktop observation, not monitor scanout. Sampling interval is 5 ms;
actual capture intervals are recorded. Selection/length and final whole-text
equality on normal Alt+F4 close are independently checked. The launching process
must also require exit code zero and the application's `verified` log record.
Failed operations are never counted as passes. --bulk-bytes explicitly tests a
larger paste; some X11 selection owners require unsupported INCR transfers.
--middle-edit restricts the run to middle-of-document edits/history and skips
resizing; it still uses the real scrollbar, text click, keyboard and clipboard.
"""
import argparse
import ctypes as C
import ctypes.util
import json
import os
from pathlib import Path
import re
import struct
import subprocess as sp
import time


class XImage(C.Structure):
    _fields_ = [("width", C.c_int), ("height", C.c_int),
                ("xoffset", C.c_int), ("format", C.c_int),
                ("data", C.c_void_p), ("byte_order", C.c_int),
                ("bitmap_unit", C.c_int), ("bitmap_bit_order", C.c_int),
                ("bitmap_pad", C.c_int), ("depth", C.c_int),
                ("bytes_per_line", C.c_int), ("bits_per_pixel", C.c_int)]


class Desktop:
    def __init__(self, window, scale):
        self.x = C.CDLL(ctypes.util.find_library("X11"))
        self.xt = C.CDLL(ctypes.util.find_library("Xtst"))
        self.x.XOpenDisplay.restype = C.c_void_p
        self.d = self.x.XOpenDisplay(None)
        if not self.d:
            raise RuntimeError("no X11 display")
        self.x.XDefaultRootWindow.argtypes = [C.c_void_p]
        self.x.XDefaultRootWindow.restype = C.c_ulong
        self.root = self.x.XDefaultRootWindow(self.d)
        self.error_handler = C.CFUNCTYPE(C.c_int, C.c_void_p, C.c_void_p)(lambda _d, _e: 0)
        self.x.XSetErrorHandler.argtypes = [C.c_void_p]
        self.x.XSetErrorHandler(self.error_handler)
        self.window, self.scale = window, scale
        self.x.XStringToKeysym.argtypes = [C.c_char_p]
        self.x.XStringToKeysym.restype = C.c_ulong
        self.x.XKeysymToKeycode.argtypes = [C.c_void_p, C.c_ulong]
        self.x.XKeysymToKeycode.restype = C.c_uint
        self.x.XFlush.argtypes = [C.c_void_p]
        self.x.XSync.argtypes = [C.c_void_p, C.c_int]
        self.x.XGetGeometry.argtypes = [C.c_void_p, C.c_ulong] + [C.c_void_p] * 7
        self.x.XTranslateCoordinates.argtypes = [C.c_void_p, C.c_ulong, C.c_ulong, C.c_int, C.c_int] + [C.c_void_p] * 3
        self.x.XGetImage.argtypes = [C.c_void_p, C.c_ulong, C.c_int, C.c_int, C.c_uint, C.c_uint, C.c_ulong, C.c_int]
        self.x.XGetImage.restype = C.POINTER(XImage)
        self.x.XDestroyImage.argtypes = [C.POINTER(XImage)]
        self.xt.XTestFakeKeyEvent.argtypes = [C.c_void_p, C.c_uint, C.c_int, C.c_ulong]
        self.xt.XTestFakeButtonEvent.argtypes = [C.c_void_p, C.c_uint, C.c_int, C.c_ulong]
        self.xt.XTestFakeMotionEvent.argtypes = [C.c_void_p, C.c_int, C.c_int, C.c_int, C.c_ulong]
        self.held = []

    def geometry(self):
        root, child = C.c_ulong(), C.c_ulong()
        x, y, gx, gy = C.c_int(), C.c_int(), C.c_int(), C.c_int()
        w, h, border, depth = C.c_uint(), C.c_uint(), C.c_uint(), C.c_uint()
        assert self.x.XGetGeometry(self.d, self.window, C.byref(root), C.byref(x), C.byref(y), C.byref(w), C.byref(h), C.byref(border), C.byref(depth))
        assert self.x.XTranslateCoordinates(self.d, self.window, self.root, 0, 0, C.byref(gx), C.byref(gy), C.byref(child))
        return gx.value, gy.value, w.value, h.value

    def pixels(self, x, y, w, h):
        im = self.x.XGetImage(self.d, self.root, x, y, w, h, C.c_ulong(-1).value, 2)
        if not im:
            raise RuntimeError("XGetImage failed")
        try:
            assert im.contents.bits_per_pixel == 32 and im.contents.byte_order == 0
            return C.string_at(im.contents.data, im.contents.bytes_per_line * h), im.contents.bytes_per_line
        finally:
            self.x.XDestroyImage(im)

    def status(self):
        x, y, w, h = self.geometry()
        # Top-anchored, last-painted overlay; unchanged during native resize.
        data, _ = self.pixels(x + 5*self.scale, y+13*self.scale, 590*self.scale, 1)
        def pixel(pos):
            at = pos * 4
            return tuple(data[at:at+3])  # BGRX
        if pixel(2*self.scale) != (255, 0, 255):
            return None  # occluded, resizing, or not yet presented
        b = bytearray(24)
        for i in range(192):
            blue, green, red = pixel((9+i*3)*self.scale)
            if red < 32 and green < 100 and blue > 160:
                b[i//8] |= 1 << (i % 8)
        if b[0] != 0xa7:
            return None
        _, seq, rev, caret, anchor, length, width, height, pending = struct.unpack("<B H I I I I H H B", b)
        return dict(seq=seq, revision=rev, caret=caret, anchor=anchor, length=length,
                    width=width, height=height, pending=bool(pending))

    def code(self, key):
        result = self.x.XKeysymToKeycode(self.d, self.x.XStringToKeysym(key.encode()))
        assert result, key
        return result

    def key(self, *keys):
        for key in keys:
            self.xt.XTestFakeKeyEvent(self.d, self.code(key), 1, 0)
            self.held.append(key)
        for key in reversed(keys):
            self.xt.XTestFakeKeyEvent(self.d, self.code(key), 0, 0)
            self.held.remove(key)
        self.x.XFlush(self.d)

    def move(self, x, y):
        self.xt.XTestFakeMotionEvent(self.d, -1, round(x), round(y), 0)
        self.x.XFlush(self.d)

    def button(self, down):
        self.xt.XTestFakeButtonEvent(self.d, 1, int(down), 0)
        self.x.XFlush(self.d)

    def click(self, x, y):
        self.move(x, y)
        self.button(True)
        self.button(False)

    def cleanup(self):
        for key in reversed(self.held):
            self.xt.XTestFakeKeyEvent(self.d, self.code(key), 0, 0)
        self.button(False)
        self.x.XFlush(self.d)


def clipboard_get():
    try:
        return sp.run(["xclip", "-selection", "clipboard", "-out", "-target", "UTF8_STRING"], capture_output=True, timeout=3)
    except sp.TimeoutExpired:
        return sp.CompletedProcess("xclip", 1, b"", b"clipboard read timed out")


def clipboard_set(data):
    # xclip daemon retains ownership until the next owner replaces it.
    sp.run(["xclip", "-selection", "clipboard", "-in"], input=data,
           stdout=sp.DEVNULL, stderr=sp.DEVNULL, check=True, timeout=10)


def normalized(data):
    return data.decode("utf-8").replace("\r\n", "\n").replace("\r", "\n").encode("utf-8")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--file", required=True)
    parser.add_argument("--kind", choices=["source", "text"], default="source")
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--scale", type=int, default=1)
    parser.add_argument("--timeout", type=float, default=30)
    parser.add_argument("--bulk-bytes", type=int, default=0)
    parser.add_argument("--middle-edit", action="store_true")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[4]
    output = args.output.resolve()
    assert not output.is_relative_to(repo), "output must be outside the repository"
    output.mkdir(parents=True, exist_ok=True)
    original = normalized(Path(args.file).read_bytes())
    title = f"SourceView interaction {args.kind} {Path(args.file).name}"
    matches = sp.check_output(["xdotool", "search", "--name", "^" + re.escape(title) + "$"], text=True).split()
    assert len(matches) == 1, f"expected one window, found {matches}"
    window = int(matches[0])
    sp.run(["xdotool", "windowmove", str(window), "80", "70"], check=True)
    sp.run(["xdotool", "windowactivate", "--sync", str(window)], check=True)
    d = Desktop(window, args.scale)
    saved_clipboard = clipboard_get()
    log = (output / "operations.jsonl").open("w")
    expected = original
    resize_failures = []

    def record(value):
        value.update(file=Path(args.file).name, kind=args.kind, scale=args.scale,
                     scope="middle-edit" if args.middle_edit else "full")
        log.write(json.dumps(value) + "\n")
        log.flush()
        print(json.dumps(value), flush=True)

    def wait_status():
        until = time.perf_counter() + args.timeout
        while time.perf_counter() < until:
            s = d.status()
            if s is not None and not s["pending"]:
                return s
            time.sleep(.005)
        raise TimeoutError("no non-pending visible status")

    def measure(name, action, predicate=None, sequence=1):
        before = wait_status()
        focus = int(sp.check_output(["xdotool", "getwindowfocus"], text=True))
        assert focus == window, f"desktop focus changed to {focus}; stopping"
        started = time.perf_counter_ns()
        wall_started = time.time_ns()
        action()
        previous, capture_gap, echo = started, 0, None
        samples = 0
        while (time.perf_counter_ns()-started)/1e9 < args.timeout:
            s = d.status()
            now = time.perf_counter_ns()
            capture_gap = max(capture_gap, now-previous)
            previous = now
            samples += 1
            if s and s["seq"] == (before["seq"]+sequence) % 65536:
                if echo is None and (sequence or predicate is None or predicate(s)):
                    echo = now
                if not s["pending"] and (predicate is None or predicate(s)):
                    record(dict(stage=name, input_at_ns=wall_started, echo_ms=(echo-started)/1e6,
                                visible_ms=(now-started)/1e6, capture_gap_ms=capture_gap/1e6,
                                samples=samples, before=before, after=s))
                    return s
            time.sleep(.005)
        record(dict(stage=name, failed=True, last=s, before=before, samples=samples))
        raise TimeoutError(name)

    def key(name, *keys, predicate=None):
        return measure(name, lambda: d.key(*keys), predicate)

    def edit(name, operation, insert=None, delete=False, sequence=1):
        nonlocal expected
        before = wait_status()
        lo, hi = sorted((before["anchor"], before["caret"]))
        if insert is not None:
            want = expected[:lo]+insert+expected[hi:]
            caret = lo+len(insert)
        elif delete:
            assert lo < hi, "delete requires a known selection"
            want, caret = expected[:lo]+expected[hi:], lo
        else:
            raise AssertionError("independent edit oracle required")
        result = measure(name, operation, lambda s: s["length"] == len(want) and s["caret"] == caret and s["anchor"] == caret and s["revision"] > before["revision"], sequence=sequence)
        expected = want
        return result

    def undo(name, want, redo=False):
        nonlocal expected
        before = wait_status()
        keys = ("Control_L", "Shift_L", "z") if redo else ("Control_L", "z")
        result = key(name, *keys, predicate=lambda s: s["length"] == len(want) and s["revision"] > before["revision"])
        expected = want
        return result

    def capture(name):
        from PIL import ImageGrab
        x, y, w, h = d.geometry()
        ImageGrab.grab(bbox=(x, y, x+w, y+h), xdisplay=os.environ.get("DISPLAY")).save(output / (name+".png"))

    try:
        ready = wait_status()
        assert d.geometry()[2:] == (1100*args.scale, 760*args.scale), "unexpected initial window geometry"
        assert ready["length"] == len(original)
        capture("initial")
        for location in (("middle",) if args.middle_edit else ("head", "middle", "tail")):
            if location == "head":
                key("head-navigation", "Control_L", "Home", predicate=lambda s: s["caret"] == 0)
            elif location == "tail":
                key("tail-navigation", "Control_L", "End", predicate=lambda s: s["caret"] == len(expected))
            else:
                # Real scrollbar track click, then click the visible text.
                x, y, w, h = d.geometry()
                measure("middle-scrollbar", lambda: d.click(x+w-12*args.scale, y+h*.50))
                measure("middle-click", lambda: d.click(x+w*.45, y+h*.48), lambda s: 0 < s["caret"] < len(expected))
            base = expected
            # Paste provides an unambiguous history boundary and ASCII probe.
            clipboard_set(b" latencyword")
            edit(location+"-paste-word", lambda: d.key("Control_L", "v"), insert=b" latencyword")
            word = expected
            key(location+"-select-word", "Control_L", "Shift_L", "Left", predicate=lambda s: abs(s["anchor"]-s["caret"]) == 11)
            edit(location+"-replace", lambda: d.key("q"), insert=b"q", sequence=0)
            replaced = expected
            undo(location+"-undo-replace", word)
            undo(location+"-redo-replace", replaced, redo=True)
            undo(location+"-restore-word", word)
            undo(location+"-restore-original", base)
            at = wait_status()["caret"]
            edit(location+"-character-insert", lambda: d.key("x"), insert=b"x", sequence=0)
            before = wait_status()
            key(location+"-backspace", "BackSpace", predicate=lambda s: s["length"] == len(base) and s["caret"] == at and s["revision"] > before["revision"])
            expected = base
            clipboard_set(b"x")
            edit(location+"-paste-character", lambda: d.key("Control_L", "v"), insert=b"x")
            key(location+"-left", "Left", predicate=lambda s: s["caret"] == at)
            before = wait_status()
            key(location+"-delete-character", "Delete", predicate=lambda s: s["length"] == len(base) and s["caret"] == at and s["revision"] > before["revision"])
            expected = base
            # A Unicode/multiline real clipboard insertion and selected delete.
            # At head/tail selection uses normal document-edge shortcuts.
            if location != "middle":
                payload = ("editing ✓ 中文 é\n".encode()*4096)[:65520]
                payload = payload.decode("utf-8", errors="ignore").encode()
                clipboard_set(payload)
                edit(location+"-paste-64k", lambda: d.key("Control_L", "v"), insert=payload)
                pasted = expected
                if location == "head":
                    key(location+"-select-64k", "Control_L", "Shift_L", "Home", predicate=lambda s: abs(s["anchor"]-s["caret"]) == len(payload))
                else:
                    undo(location+"-undo-paste-select", base)
                    undo(location+"-redo-paste-select", pasted, redo=True)
                    # Undo/redo restores the post-paste caret, not a selection.
                    # Select back to paste start with 4096 normal Shift+Up
                    # would measure selection traversal, not deletion. Instead
                    # check large selection deletion at the head only.
                    undo(location+"-restore-64k", base)
                    continue
                edit(location+"-delete-64k", lambda: d.key("Delete"), delete=True)
                undo(location+"-undo-delete-64k", pasted)
                undo(location+"-restore-64k", base)
        key("bulk-home", "Control_L", "Home", predicate=lambda s: s["caret"] == 0)
        if args.bulk_bytes:
            line = b"bulk pasted text for actual clipboard input and deletion\n"
            payload = (line*(args.bulk_bytes//len(line)+1))[:args.bulk_bytes]
            clipboard_set(payload)
            edit("paste-bulk", lambda: d.key("Control_L", "v"), insert=payload)
            pasted = expected
            key("select-bulk", "Control_L", "Shift_L", "Home", predicate=lambda s: abs(s["anchor"]-s["caret"]) == len(payload))
            edit("delete-bulk", lambda: d.key("Delete"), delete=True)
            undo("undo-delete-bulk", pasted)
            undo("restore-bulk", original)
        capture("edited-restored")
        # Actual integrated-chrome drags. Native WM resize grabs may consume
        # keyboard events, so keyboard echo is not a valid probe during a drag.
        drags = (("width-narrow", -240, 0), ("width-restore", 240, 0),
                 ("height-shorter", 0, -160), ("height-restore", 0, 160),
                 ("corner-narrow", -180, -120), ("corner-restore", 180, 120))
        for name, dx, dy in (() if args.middle_edit else drags):
            if not resize(d, wait_status, record, name, dx*args.scale, dy*args.scale):
                resize_failures.append(name)
        capture("resized-restored")
        # The window's close handler compares the complete model to the input,
        # outside latency observations. Whole-document clipboard copying is a
        # separate platform capability; it is not needed to read model bytes.
        assert expected == original
        record(dict(stage="ready-for-close-verification", bytes=len(original), resize_failures=resize_failures))
        d.key("Alt_L", "F4")
        if resize_failures:
            raise AssertionError("no intermediate viewport frames during drag: "+", ".join(resize_failures))
    finally:
        d.cleanup()
        if saved_clipboard.returncode == 0:
            clipboard_set(saved_clipboard.stdout)
        else:
            clipboard_set(b"")
        log.close()


def resize(d, wait_status, record, name, dx, dy):
    before = wait_status()
    x, y, w, h = d.geometry()
    px = x+w-2*d.scale if dx else x+w*.5
    py = y+h-2*d.scale if dy else y+h*.5
    d.move(px, py)
    time.sleep(.06)
    started = time.perf_counter_ns()
    d.button(True)
    # 16 intermediate constraints over 480 ms; XTest keeps the button held.
    capture_gap, previous, last_presented = 0, started, started
    presented, presentation_gaps = [], []
    last_size = before["width"], before["height"]

    def observe():
        nonlocal previous, capture_gap, last_presented, last_size
        s = d.status()
        now = time.perf_counter_ns()
        capture_gap = max(capture_gap, now-previous)
        previous = now
        if s and (s["width"], s["height"]) != last_size:
            last_size = s["width"], s["height"]
            presentation_gaps.append((now-last_presented)/1e6)
            presented.append(dict(at_ms=(now-started)/1e6, width=s["width"], height=s["height"], pending=s["pending"]))
            last_presented = now
        return s, now

    for i in range(1, 17):
        d.move(px+dx*i/16, py+dy*i/16)
        sent = time.perf_counter_ns()
        end = sent + 30_000_000
        while time.perf_counter_ns() < end:
            observe()
            time.sleep(.005)
    d.button(False)
    released = time.perf_counter_ns()
    until = released+30_000_000_000
    while time.perf_counter_ns() < until:
        s, now = observe()
        if s and s["width"] == before["width"]+dx//d.scale and s["height"] == before["height"]+dy//d.scale and not s["pending"]:
            ax, ay, aw, ah = d.geometry()
            assert aw == w+dx and ah == h+dy, "resize missed requested geometry"
            assert s["caret"] == before["caret"] and s["length"] == before["length"], "resize changed text or caret"
            during = sum(item["at_ms"] <= (released-started)/1e6 for item in presented)
            responsive = during >= 2
            record(dict(stage=name, drag_ms=(released-started)/1e6, release_visible_ms=(now-released)/1e6,
                        input_at_ns=time.time_ns()-(now-started), presentation_gap_max_ms=max(presentation_gaps),
                        frames_during_drag=during, responsive=responsive,
                        presented=presented, capture_gap_ms=capture_gap/1e6,
                        window_before=[w, h], window_after=[aw, ah], before=before, after=s))
            return responsive
        time.sleep(.005)
    record(dict(stage=name, failed=True, last=s, before=before))
    raise TimeoutError(name)


if __name__ == "__main__":
    main()
