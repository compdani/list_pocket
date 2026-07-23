#!/usr/bin/env python3
"""Rewrite wrapEcho registrations in cmd/handlers.go to native asHandler/pmRE."""
from __future__ import annotations

import re
from pathlib import Path


def find_call_end(s: str, start: int) -> int:
    """start points at '(' of wrapEcho(...). Return index after closing ')'."""
    depth = 0
    i = start
    while i < len(s):
        ch = s[i]
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
            if depth == 0:
                return i + 1
        i += 1
    raise ValueError("unbalanced")


def transform_handler_expr(expr: str) -> str:
    expr = re.sub(r"\bpm\(", "pmRE(", expr)
    expr = expr.replace("noIndex(", "noIndexRE(")
    expr = expr.replace("a.hasUUID(", "a.hasUUIDRE(")
    expr = expr.replace("a.hasSub(", "a.hasSubRE(")
    expr = expr.replace("a.hasRecordID(", "a.hasRecordIDRE(")
    expr = re.sub(r"pmRE\(hasID\((a\.\w+)\)", r"pmRE(\1", expr)
    expr = re.sub(r"hasID\((a\.\w+)\)", r"\1", expr)
    return expr.strip()


def main() -> None:
    path = Path(__file__).resolve().parents[1] / "cmd" / "handlers.go"
    src = path.read_text()

    if "pmRE :=" not in src:
        src = src.replace(
            '\tpm := a.auth.Perm\n\tapi := se.Group("/mailapi").Bind(apis.RequireAuth())',
            '\tpm := a.auth.Perm\n\tpmRE := a.auth.PermRE\n\tapi := se.Group("/mailapi").Bind(apis.RequireAuth()).BindFunc(hydrateAuthUser)',
        )

    out = []
    i = 0
    key = "wrapEcho("
    while True:
        j = src.find(key, i)
        if j < 0:
            out.append(src[i:])
            break
        out.append(src[i:j])
        end = find_call_end(src, j + len("wrapEcho") - 1 + 1)  # at '('
        # Actually wrapEcho( — find '(' at j+len("wrapEcho")
        paren = j + len("wrapEcho")
        assert src[paren] == "("
        end = find_call_end(src, paren)
        call = src[j:end]
        # wrapEcho(a, tpl, cfg, urlCfg, PARAMS, HANDLER)
        inner = call[len("wrapEcho(") : -1]
        # split from the right for handler (last arg)
        # first 4 args are fixed identifiers/exprs without nested commas at top... use comma scan
        args = []
        depth = 0
        cur = []
        for ch in inner:
            if ch == "(" or ch == "[":
                depth += 1
                cur.append(ch)
            elif ch == ")" or ch == "]":
                depth -= 1
                cur.append(ch)
            elif ch == "," and depth == 0:
                args.append("".join(cur).strip())
                cur = []
            else:
                cur.append(ch)
        if cur:
            args.append("".join(cur).strip())
        if len(args) < 6:
            raise SystemExit(f"bad wrapEcho args ({len(args)}): {call[:120]}")
        handler = transform_handler_expr(",".join(args[5:]) if len(args) > 6 else args[5])
        # args[5] only — if more, join
        if len(args) > 6:
            handler = transform_handler_expr(", ".join(args[5:]))
        else:
            handler = transform_handler_expr(args[5])
        out.append(f"asHandler({handler})")
        i = end

    path.write_text("".join(out))
    print("updated handlers.go registration")


if __name__ == "__main__":
    main()
