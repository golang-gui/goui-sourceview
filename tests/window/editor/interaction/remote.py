#!/usr/bin/env python3
"""Real VNC/RDP input and presented-frame observation for main.go -remote.

Start the window in the interactive session. Inspect its screenshot before
supplying --bounds X,Y,W,H and --geometry-version. Requires remote-computer-use
and Pillow on the observer. No editor APIs, injected GUI actions or clipboard
are used. Each modification has an independent byte-length/caret oracle; the
window must also exit normally and log verified (checked by its launcher).
Timing includes remote input transport, capture/PNG decoding and polling. It
is not native measurement time, monitor scanout or an exact UI freeze time.
The passive GUI timer log separately records delivery gaps. VNC colors may be
lossy; the marker uses separated color classes and validates its magic byte.
This changes focus, pointer, window geometry and the in-memory document only.
It releases held input on failure. Never run alongside another desktop user.
"""
import argparse
import json
from pathlib import Path
import struct
import subprocess
import time

from PIL import Image


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--conn", required=True)
    parser.add_argument("--platform", choices=["mac", "windows"], required=True)
    parser.add_argument("--bounds", required=True, help="observed client X,Y,W,H, in screenshot pixels")
    parser.add_argument("--geometry-version", type=int, required=True)
    parser.add_argument("--run-id", type=int, default=0)
    parser.add_argument("--file", required=True, type=Path, help="local copy of the identical input")
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--cli", default=str(Path.home()/".remote-computer-use/bin/remote-computer-use"))
    parser.add_argument("--resize", action="store_true")
    parser.add_argument("--line-end", action="store_true",
                        help="move to an unwrapped logical line end before editing (avoids linked tag-name edits)")
    parser.add_argument("--timeout", type=float, default=30)
    args = parser.parse_args()
    assert 0 <= args.run_id <= 127
    output = args.output.resolve()
    assert not output.is_relative_to(Path(__file__).resolve().parents[4])
    output.mkdir(parents=True, exist_ok=True)
    bounds = list(map(int, args.bounds.split(",")))
    assert len(bounds) == 4
    x, y, width, height = bounds
    original = args.file.read_text().replace("\r\n", "\n").replace("\r", "\n").encode()
    expected = original
    resize_failures = []
    primary = "CMD" if args.platform == "mac" else "CTRL"
    log = (output/"operations.jsonl").open("w")

    def call(*parts):
        argv = [str(round(p)) if isinstance(p, float) else str(p) for p in parts]
        result = subprocess.run([args.cli, "--conn", args.conn, *argv, "--json"],
                                capture_output=True, check=True, timeout=15)
        value = json.loads(result.stdout)
        assert value.get("ok"), value
        return value

    def record(value):
        value.update(connection=args.conn, platform=args.platform, file=args.file.name)
        log.write(json.dumps(value)+"\n")
        log.flush()
        print(json.dumps(value), flush=True)

    def status():
        meta = call("screenshot", "--output", output/"latest.png")
        assert meta["geometry_version"] == args.geometry_version, "desktop geometry changed"
        with Image.open(output/"latest.png") as screenshot:
            im = screenshot.convert("RGB")
            mx, my = x+100, y+24
            r, g, b = im.getpixel((mx+3, my+8))
            if not (r > 170 and b > 170 and g < 120):
                return None
            data = bytearray(24)
            for i in range(192):
                r, g, b = im.getpixel((mx+9+i*3, my+8))
                if r < 80 and g < 140 and b > 140:
                    data[i//8] |= 1 << (i % 8)
            if data[0] != 0xa7:
                return None
            _, seq, rev, caret, anchor, length, w, h, flags = struct.unpack("<B H I I I I H H B", data)
            if flags >> 1 != args.run_id:
                return None
            return dict(seq=seq, revision=rev, caret=caret, anchor=anchor, length=length,
                        width=w, height=h, pending=bool(flags & 1), run_id=flags >> 1,
                        frame_sequence=meta["frame_sequence"])

    def ready():
        until = time.monotonic()+args.timeout
        while time.monotonic() < until:
            s = status()
            if s and not s["pending"]:
                return s
            time.sleep(.01)
        raise TimeoutError("no presented, non-pending marker at the observed bounds")

    def measure(name, action, predicate=None):
        before = ready()
        started, wall = time.perf_counter_ns(), time.time_ns()
        action()
        previous, gap, samples = started, 0, 0
        last = None
        while (time.perf_counter_ns()-started)/1e9 < args.timeout:
            last = status()
            now = time.perf_counter_ns()
            gap = max(gap, now-previous)
            previous = now
            samples += 1
            changed = last and (last["seq"] != before["seq"] or last["revision"] != before["revision"])
            if changed and not last["pending"] and (predicate is None or predicate(last)):
                record(dict(stage=name, input_at_ns=wall, visible_ms=(now-started)/1e6,
                            capture_gap_ms=gap/1e6, samples=samples, before=before, after=last))
                return last
            time.sleep(.01)
        record(dict(stage=name, failed=True, before=before, last=last, samples=samples))
        raise TimeoutError(name)

    def key(name, shortcut, predicate=None):
        return measure(name, lambda: call("key", shortcut), predicate)

    def edit(name, text):
        nonlocal expected
        before = ready()
        lo, hi = sorted((before["caret"], before["anchor"]))
        encoded = text.encode()
        want, caret = expected[:lo]+encoded+expected[hi:], lo+len(encoded)
        measure(name, lambda: call("type", "--text", text),
                lambda s: s["revision"] > before["revision"] and s["length"] == len(want)
                and s["caret"] == caret and s["anchor"] == caret)
        expected = want

    def history(name, want, redo=False):
        nonlocal expected
        before = ready()
        key(name, primary+("+SHIFT+Z" if redo else "+Z"),
            lambda s: s["revision"] > before["revision"] and s["length"] == len(want))
        expected = want

    try:
        initial = ready()
        assert initial["length"] == len(original)
        assert initial["revision"] == 1 and initial["seq"] == 0, "expected a fresh fixture window"
        measure("middle-scrollbar", lambda: call("click", "--x", x+width-4, "--y", y+height*.5,
                                                 "--geometry-version", args.geometry_version))
        measure("middle-click", lambda: call("click", "--x", x+width*.45, "--y", y+height*.48,
                                             "--geometry-version", args.geometry_version),
                lambda s: len(original)*.3 < s["caret"] < len(original)*.7)
        if args.line_end:
            at = ready()["caret"]
            end = original.find(b"\n", at)
            if end < 0:
                end = len(original)
            key("middle-line-end", "CMD+RIGHT" if args.platform == "mac" else "END",
                lambda s: s["caret"] == s["anchor"] == end)
        base = expected
        at = ready()["caret"]
        edit("middle-character-insert", "x")
        before = ready()
        key("middle-backspace", "BACKSPACE", lambda s: s["revision"] > before["revision"]
            and s["length"] == len(base) and s["caret"] == s["anchor"] == at)
        expected = base
        edit("middle-insert-word", " latencyword")
        word = expected
        before = ready()
        measure("middle-select-word", lambda: [call("key", "SHIFT+LEFT") for _ in range(11)],
                lambda s: abs(s["anchor"]-s["caret"]) == 11 and s["length"] == len(word))
        edit("middle-replace", "q")
        replacement = expected
        history("middle-undo-replace", word)
        history("middle-redo-replace", replacement, True)
        history("middle-restore-word", word)
        history("middle-restore-original", base)
        edit("middle-insert-delete-probe", "x")
        at = ready()["caret"]-1
        key("middle-left", "LEFT", lambda s: s["caret"] == s["anchor"] == at)
        before = ready()
        key("middle-delete", "DELETE", lambda s: s["revision"] > before["revision"]
            and s["length"] == len(base) and s["caret"] == s["anchor"] == at)
        expected = base
        if args.resize:
            for name, dx, dy in [("width-narrow", -240, 0), ("width-restore", 240, 0),
                                 ("height-shorter", 0, -140), ("height-restore", 0, 140),
                                 ("corner-narrow", -180, -120), ("corner-restore", 180, 120)]:
                before = ready()
                # Windows' invisible native resize border is outside the
                # client; clicking its scrollbar is an ordinary client press.
                edge = -4 if args.platform == "windows" else 2
                px = x+width-edge if dx else x+width//2
                py = y+height-edge if dy else y+height//2
                call("move", "--x", px, "--y", py, "--geometry-version", args.geometry_version)
                time.sleep(.06)
                started, wall = time.perf_counter_ns(), time.time_ns()
                call("pointer-down", "--x", px, "--y", py, "--geometry-version", args.geometry_version)
                presented = []
                try:
                    for i in range(1, 9):
                        call("move", "--x", px+dx*i//8, "--y", py+dy*i//8,
                             "--geometry-version", args.geometry_version)
                        s = status()
                        if s and (not presented or (s["width"], s["height"]) !=
                                  (presented[-1]["width"], presented[-1]["height"])):
                            presented.append(dict(at_ms=(time.perf_counter_ns()-started)/1e6, **s))
                finally:
                    call("pointer-up", "--button", "left")
                released = time.perf_counter_ns()
                width, height = width+dx, height+dy
                after = ready()
                until = time.monotonic()+args.timeout
                while (after["width"], after["height"]) != (before["width"]+dx, before["height"]+dy):
                    if time.monotonic() > until:
                        raise AssertionError("native resize did not present requested dimensions")
                    after = ready()
                assert after["length"] == len(original) and after["caret"] == before["caret"]
                changes = [s for s in presented if (s["width"], s["height"]) != (before["width"], before["height"])]
                record(dict(stage=name, input_at_ns=wall, drag_ms=(released-started)/1e6,
                            release_visible_ms=(time.perf_counter_ns()-released)/1e6,
                            frames_during_drag=len(changes), presented=presented, before=before, after=after))
                if len(changes) < 2:
                    resize_failures.append(name)
        assert expected == original
        record(dict(stage="ready-for-close-verification", bytes=len(original), resize_failures=resize_failures))
        if args.platform == "mac":
            call("click", "--x", x+14, "--y", y+14, "--geometry-version", args.geometry_version)
        else:
            call("key", "ALT+F4")
        if resize_failures:
            raise AssertionError("fewer than two intermediate viewport sizes presented: "+", ".join(resize_failures))
    finally:
        call("release-input")
        log.close()


if __name__ == "__main__":
    main()
