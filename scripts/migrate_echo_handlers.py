#!/usr/bin/env python3
"""Mechanically convert cmd Echo handlers to PocketBase RequestEvent handlers."""
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "cmd"

# Files with App handlers to convert (exclude tests, pbhttp, handlers registration partially).
HANDLER_FILES = [
    "handlers.go",  # HealthCheck, appearance, middleware kept dual
    "admin.go",
    "auth.go",
    "i18n.go",
    "settings.go",
    "lists.go",
    "templates.go",
    "roles.go",
    "users.go",
    "bounce.go",
    "maintenance.go",
    "tx.go",
    "tx_messages.go",
    "subscribers.go",
    "subscribers_bulk.go",
    "campaigns.go",
    "media.go",
    "import.go",
    "inbound_email_inbox.go",
    "inbound_email_replies.go",
    "text_messaging.go",
    "ai_campaign_builder.go",
    "public.go",
    "archive.go",
    "events.go",
]


def ensure_import(src: str, import_line: str) -> str:
    if import_line in src:
        return src
    # Insert into import block
    m = re.search(r"import \(\n", src)
    if not m:
        return src
    return src[: m.end()] + f'\t{import_line}\n' + src[m.end() :]


def convert_file(path: Path) -> bool:
    src = path.read_text()
    orig = src

    # Signature: func (a *App) Name(c echo.Context) error
    src = re.sub(
        r"func \(a \*App\) (\w+)\(c echo\.Context\) error",
        r"func (a *App) \1(re *pbcore.RequestEvent) error",
        src,
    )

    # Middleware-style: func xxx(next echo.HandlerFunc) echo.HandlerFunc — leave for now
    # Handler funcs that are not methods: serveCustomAppearance returns echo.HandlerFunc — leave

    # Common replacements (order matters)
    replacements = [
        (r"\bc\.Param\(", "pathParam(re, "),
        (r"\bc\.QueryParam\(", "queryParam(re, "),
        (r"\bc\.QueryParams\(", "queryParams(re, "),
        (r"\bc\.Bind\(", "bindJSON(re, "),
        (r"\bc\.Request\(\)", "re.Request"),
        (r"\bc\.Response\(\)", "re.Response"),
        (r"\bauth\.GetUser\(c\)", "auth.GetUserRE(re)"),
        (r"\bGetUser\(c\)", "auth.GetUserRE(re)"),
        (r"\bExtractRoleID\(c\)", "auth.ExtractRoleIDRE(re)"),
        (r"\bc\.Get\(\"app\"\)\.\(\*App\)", "getApp(re)"),
        (r"\bc\.FormValue\(", "re.Request.FormValue("),
        (r"\bc\.FormFile\(", "re.Request.FormFile("),
        (r"\bc\.MultipartForm\(\)", "re.Request.MultipartForm"),
        (r"\bc\.Redirect\(", "re.Redirect("),
        (r"\bc\.NoContent\(", "re.NoContent("),
        (r"\bgetID\(c\)", 're.Get("id").(int)'),
        # JSON helpers
        (r"\bc\.JSON\(http\.StatusOK, okResp\{", "okJSON(re, "),
        # After okJSON(re, X})  — fix trailing }) that was okResp{X}
        # Handled below with a second pass
        (r"\bc\.JSON\(", "re.JSON("),
        (r"\bc\.Blob\(", "writeBlob(re, "),
        (r"\bc\.Stream\(", "writeStream(re, "),
        (r"\bc\.Render\(", "renderTpl(re, "),
        (r"\bc\.RealIP\(\)", "clientIP(re)"),
        (r"\bc\.Cookie\(", "re.Request.Cookie("),
        (r"\bc\.SetCookie\(", "http.SetCookie(re.Response, "),
        (r"\bc\.Attachment\(", "re.Attachment("),  # may not exist
    ]

    for pat, repl in replacements:
        src = re.sub(pat, repl, src)

    # Fix okJSON(re, data}) -> okJSON(re, data)  when converted from okResp{data}
    # Pattern: okJSON(re, SOMETHING})
    def fix_okjson(m: re.Match) -> str:
        inner = m.group(1).rstrip()
        if inner.endswith("}"):
            # count braces - simple case okJSON(re, x})
            return f"okJSON(re, {inner[:-1]})"
        return m.group(0)

    src = re.sub(r"okJSON\(re, (.*)\}\)", fix_okjson, src)

    # trackingOpenEvent(c -> trackingOpenEvent(re
    src = src.replace("trackingOpenEvent(c)", "trackingOpenEvent(re)")
    src = src.replace("func trackingOpenEvent(c echo.Context)", "func trackingOpenEvent(re *pbcore.RequestEvent)")

    # roleRouteRecordID(c -> (re
    src = src.replace("roleRouteRecordID(c)", "roleRouteRecordID(re)")
    src = re.sub(
        r"func roleRouteRecordID\(c echo\.Context\)",
        "func roleRouteRecordID(re *pbcore.RequestEvent)",
        src,
    )

    if "pbcore " not in src and "*pbcore.RequestEvent" in src:
        src = ensure_import(src, 'pbcore "github.com/pocketbase/pocketbase/core"')

    # Remove unused echo import only if echo. is gone — leave for NewHTTPError
    if src != orig:
        path.write_text(src)
        return True
    return False


def main() -> int:
    changed = 0
    for name in HANDLER_FILES:
        path = ROOT / name
        if not path.exists():
            print(f"skip missing {name}", file=sys.stderr)
            continue
        if convert_file(path):
            print(f"converted {name}")
            changed += 1
        else:
            print(f"unchanged {name}")
    print(f"done, {changed} files changed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
