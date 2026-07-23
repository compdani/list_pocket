#!/usr/bin/env python3
"""Convert echo.NewHTTPError(...) to apperr helpers and fix imports."""
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

STATUS_HELPERS = {
    "http.StatusBadRequest": "BadRequest",
    "http.StatusForbidden": "Forbidden",
    "http.StatusNotFound": "NotFound",
    "http.StatusInternalServerError": "Internal",
    "http.StatusConflict": "Conflict",
}

# Packages / files to convert (relative to repo root).
TARGETS = [
    "cmd",
    "internal/core",
    "internal/auth",
]

SKIP_NAMES = {
    # Keep dual support briefly? No — convert everything including pbhttp.
}

ECHO_CONST_REPLACEMENTS = [
    ("echo.HeaderContentType", '"Content-Type"'),
    ("echo.HeaderContentDisposition", '"Content-Disposition"'),
    ("echo.HeaderContentLength", '"Content-Length"'),
    ("echo.MIMEApplicationJSON", '"application/json"'),
    ("echo.MIMEApplicationJSONCharsetUTF8", '"application/json; charset=UTF-8"'),
    ("echo.MIMEOctetStream", '"application/octet-stream"'),
    ("echo.MIMETextHTML", '"text/html"'),
    ("echo.MIMETextHTMLCharsetUTF8", '"text/html; charset=UTF-8"'),
    ("echo.MIMETextPlain", '"text/plain"'),
    ("echo.MIMETextPlainCharsetUTF8", '"text/plain; charset=UTF-8"'),
    ("echo.MIMEMultipartForm", '"multipart/form-data"'),
]


def find_matching_paren(s: str, open_idx: int) -> int:
    """open_idx points at '('; return index of matching ')'."""
    depth = 0
    i = open_idx
    in_str = False
    str_ch = ""
    escape = False
    while i < len(s):
        ch = s[i]
        if in_str:
            if escape:
                escape = False
            elif ch == "\\":
                escape = True
            elif ch == str_ch:
                in_str = False
            i += 1
            continue
        if ch in ('"', "'", "`"):
            in_str = True
            str_ch = ch
            i += 1
            continue
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
            if depth == 0:
                return i
        i += 1
    raise ValueError("unbalanced paren")


def split_top_level_args(args: str) -> list[str]:
    parts: list[str] = []
    depth = 0
    in_str = False
    str_ch = ""
    escape = False
    start = 0
    i = 0
    while i < len(args):
        ch = args[i]
        if in_str:
            if escape:
                escape = False
            elif ch == "\\":
                escape = True
            elif ch == str_ch:
                in_str = False
            i += 1
            continue
        if ch in ('"', "'", "`"):
            in_str = True
            str_ch = ch
            i += 1
            continue
        if ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth -= 1
        elif ch == "," and depth == 0:
            parts.append(args[start:i].strip())
            start = i + 1
        i += 1
    tail = args[start:].strip()
    if tail:
        parts.append(tail)
    return parts


def convert_new_http_error(call_inner: str) -> str:
    """call_inner is contents inside echo.NewHTTPError(...)."""
    args = split_top_level_args(call_inner)
    if len(args) < 2:
        # echo.NewHTTPError(code) — rare
        if len(args) == 1:
            return f"apperr.New({args[0]}, http.StatusText({args[0]}))"
        raise ValueError(f"bad args: {call_inner!r}")

    status, msg = args[0], args[1]
    # If more than 2 args were somehow present (shouldn't), join remainder into msg
    if len(args) > 2:
        msg = ", ".join(args[1:])

    helper = STATUS_HELPERS.get(status)
    if helper:
        return f"apperr.{helper}({msg})"
    return f"apperr.New({status}, {msg})"


def ensure_import(src: str, import_path: str) -> str:
    line = f'\t"{import_path}"'
    if import_path in src:
        return src
    m = re.search(r"\nimport \(\n", src)
    if not m:
        # single import form
        m2 = re.search(r'\nimport\s+"[^"]+"\n', src)
        if not m2:
            return src
        block = f"\nimport (\n\t{m2.group(0).strip().removeprefix('import ').strip()}\n{line}\n)\n"
        return src[: m2.start()] + block + src[m2.end() :]
    return src[: m.end()] + line + "\n" + src[m.end() :]


def remove_import(src: str, import_path: str) -> str:
    # Remove exact import line variants
    patterns = [
        rf'\n\t"{re.escape(import_path)}"\n',
        rf'\n\techo\s+"{re.escape(import_path)}"\n',
        rf'\n\t"{re.escape(import_path)}/middleware"\n',
    ]
    for pat in patterns:
        src = re.sub(pat, "\n", src)
    # Collapse empty import ()
    src = re.sub(r"\nimport \(\n\)\n", "\n", src)
    return src


def still_needs_echo(src: str) -> bool:
    # After removing NewHTTPError and const replacements, any remaining echo. usage?
    tmp = src
    for old, _ in ECHO_CONST_REPLACEMENTS:
        tmp = tmp.replace(old, "")
    # strip import lines for check
    tmp = re.sub(r'"github.com/labstack/echo[^"]*"', "", tmp)
    return bool(re.search(r"\becho\.", tmp) or "echo.Context" in tmp or "echo.HandlerFunc" in tmp or "echo.Echo" in tmp or "echo.HTTPError" in tmp or "echo.MiddlewareFunc" in tmp)


def convert_file(path: Path) -> bool:
    src = path.read_text()
    orig = src

    # Replace NewHTTPError calls
    needle = "echo.NewHTTPError("
    out = []
    i = 0
    changed_errors = 0
    while True:
        j = src.find(needle, i)
        if j < 0:
            out.append(src[i:])
            break
        out.append(src[i:j])
        open_paren = j + len("echo.NewHTTPError")
        close = find_matching_paren(src, open_paren)
        inner = src[open_paren + 1 : close]
        try:
            replacement = convert_new_http_error(inner)
        except Exception as e:
            print(f"FAIL {path}: {e} near {src[j:j+80]!r}", file=sys.stderr)
            out.append(src[j : close + 1])
            i = close + 1
            continue
        out.append(replacement)
        changed_errors += 1
        i = close + 1
    src = "".join(out)

    # Const replacements
    for old, new in ECHO_CONST_REPLACEMENTS:
        src = src.replace(old, new)

    if src == orig and changed_errors == 0:
        return False

    if "apperr." in src or changed_errors:
        src = ensure_import(src, "github.com/compdani/list_pocket/internal/apperr")

    if not still_needs_echo(src):
        src = remove_import(src, "github.com/labstack/echo/v4")
        src = remove_import(src, "github.com/labstack/echo/v4/middleware")

    if src != orig:
        path.write_text(src)
        print(f"updated {path.relative_to(ROOT)} ({changed_errors} NewHTTPError)")
        return True
    return False


def main() -> None:
    files: list[Path] = []
    for t in TARGETS:
        p = ROOT / t
        files.extend(sorted(p.rglob("*.go")))

    n = 0
    for f in files:
        if convert_file(f):
            n += 1
    print(f"done: {n} files changed")


if __name__ == "__main__":
    main()
