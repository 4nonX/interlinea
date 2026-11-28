#!/usr/bin/env python3
"""Runs the desktop tests that just failed again, one by one, each writing
what it says straight to a file, and prints that file and how the test
ended.

ctest shows what a test printed, but a test that crashes on Windows can
take what it printed with it: its output to a pipe is held back until
the end. Written to a file, every line is kept up to the crash, and the
exit code names what ended it (0xC0000005 is an access violation).

Usage: rerun-failed.py <build dir> [config]
"""

import json
import os
import subprocess
import sys
import tempfile

CODES = {
    0xC0000005: "access violation",
    0xC0000409: "stack buffer overrun or abort",
    0xC00000FD: "stack overflow",
    0xC0000374: "heap corruption",
    0x80000003: "breakpoint (assertion)",
    0xC000001D: "illegal instruction",
    0xC0000135: "a DLL was not found",
    0xC0000139: "an entry point was not found in a DLL",
}


def failed(build):
    path = os.path.join(build, "Testing", "Temporary", "LastTestsFailed.log")
    try:
        with open(path, encoding="utf-8") as f:
            return [line.split(":", 1)[1].strip() for line in f if ":" in line]
    except FileNotFoundError:
        return []


def tests(build, config):
    out = subprocess.run(
        ["ctest", "--test-dir", build, "-C", config, "--show-only=json-v1"],
        check=True,
        capture_output=True,
        text=True,
    ).stdout
    found = {}
    for t in json.loads(out)["tests"]:
        env = {}
        for p in t.get("properties", []):
            if p["name"] == "ENVIRONMENT":
                for kv in p["value"]:
                    k, _, v = kv.partition("=")
                    env[k] = v
            if p["name"] == "WORKING_DIRECTORY":
                env["__cwd"] = p["value"]
        found[t["name"]] = (t["command"], env)
    return found


def how(code):
    if code >= 0:
        return f"exit code {code}"
    unsigned = code & 0xFFFFFFFF
    known = CODES.get(unsigned, "")
    return f"exit code {code} (0x{unsigned:08X}{', ' + known if known else ''})"


def main():
    build = sys.argv[1]
    config = sys.argv[2] if len(sys.argv) > 2 else "Release"
    names = failed(build)
    if not names:
        print("no failed tests to run again")
        return
    all_tests = tests(build, config)
    logs = tempfile.mkdtemp(prefix="interlinea-tests-")
    for name in names:
        # What it said in the run that failed, kept in a file as written.
        first = os.path.join(build, "test-logs", f"{name}.txt")
        if os.path.exists(first):
            with open(first, encoding="utf-8", errors="replace") as f:
                lines = f.read().splitlines()
            print(f"== {name}, as it failed: the end of its log")
            print("\n".join(lines[-60:]) if lines else "(empty)", flush=True)
    for name in names:
        if name not in all_tests:
            print(f"== {name}: not found")
            continue
        command, extra = all_tests[name]
        env = dict(os.environ)
        cwd = extra.pop("__cwd", None)
        env.update(extra)
        log = os.path.join(logs, f"{name}.txt")
        print(f"== {name}, run again, its log written to a file", flush=True)
        # The command as ctest has it, without its own log options.
        bare, skip = [], 0
        for i, a in enumerate(command):
            if skip:
                skip -= 1
                continue
            if a == "-o" and i > 0:
                skip = 1
                continue
            bare.append(a)
        try:
            r = subprocess.run(
                bare + ["-o", f"{log},txt", "-maxwarnings", "0"],
                env=env,
                cwd=cwd,
                timeout=300,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                errors="replace",
            )
            ended = how(r.returncode)
            printed = r.stdout
        except subprocess.TimeoutExpired as e:
            ended = "still running after 300 s"
            printed = e.stdout.decode(errors="replace") if isinstance(e.stdout, bytes) else (e.stdout or "")
        try:
            with open(log, encoding="utf-8", errors="replace") as f:
                lines = f.read().splitlines()
        except FileNotFoundError:
            lines = ["(no log: the test ended before it wrote one)"]
        # The start of the log names the system and Qt; the end, what
        # happened last.
        shown = lines if len(lines) <= 160 else lines[:12] + ["..."] + lines[-148:]
        print("\n".join(shown))
        if printed.strip():
            print("-- printed besides the log:")
            print("\n".join(printed.splitlines()[-40:]))
        print(f"-- {name}: {ended}", flush=True)


if __name__ == "__main__":
    main()
