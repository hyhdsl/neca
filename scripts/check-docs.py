#!/usr/bin/env python3
"""check-docs.py - M7 documentation gate.

Checks:
  1. Markdown internal links resolve (external URLs are not fetched).
  2. Code fences are balanced (triple-backtick and triple-tilde).
  3. The help-site structure, CNAME and language entrypoints exist.

Run from the repository root: python3 scripts/check-docs.py
Exit code 0 = pass, 1 = findings.
"""

from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parent.parent
LINK_RE = re.compile(r"\]\(\s*(<[^>]+>|[^)\s]+)(?:\s+[^)]*)?\)")
FENCE_TICK = chr(96) * 3
FENCE_TILDE = "~~~"
HELP = ROOT / "docs" / "help"
REQUIRED_ROOT = [
    "README.md",
    "index.html",
    "home.css",
    "404.html",
    "sitemap.xml",
    "robots.txt",
    "custom.css",
    "CNAME",
    ".nojekyll",
]
REQUIRED_ZH = [
    "README.md",
    "SUMMARY.md",
    "_sidebar.md",
    ".gitbook.yaml",
    "index.html",
    "getting-started.md",
    "install.md",
    "certificates.md",
    "packages.md",
    "web.md",
    "operations.md",
    "security.md",
    "troubleshooting.md",
    "faq.md",
    "development.md",
    "api.md",
]
REQUIRED_EN = [
    "README.md",
    "SUMMARY.md",
    "_sidebar.md",
    ".gitbook.yaml",
    "index.html",
    "getting-started.md",
    "install.md",
    "certificates.md",
    "web.md",
    "security.md",
    "faq.md",
    "api.md",
]


def tracked_markdown() -> list[Path]:
    try:
        out = subprocess.check_output(
            ["git", "ls-files", "*.md"], cwd=ROOT, text=True
        )
        paths = [ROOT / line for line in out.splitlines() if line.strip()]
        if paths:
            return sorted(paths)
    except (OSError, subprocess.CalledProcessError):
        pass
    return sorted(
        p
        for p in ROOT.rglob("*.md")
        if ".git" not in p.parts and "dist" not in p.parts
    )


def check_fences(path: Path, text: str) -> list[str]:
    errors: list[str] = []
    opener: str | None = None
    for lineno, line in enumerate(text.splitlines(), 1):
        stripped = line.strip()
        if opener is None:
            if stripped.startswith(FENCE_TICK) or stripped.startswith(FENCE_TILDE):
                opener = stripped[:3]
        elif stripped.startswith(opener):
            opener = None
    if opener is not None:
        errors.append(f"{path.relative_to(ROOT)}: unclosed code fence {opener}")
    return errors


def check_links(path: Path, text: str) -> list[str]:
    errors: list[str] = []
    for match in LINK_RE.finditer(text):
        target = match.group(1).strip()
        if target.startswith("<") and target.endswith(">"):
            target = target[1:-1]
        if target.startswith(("http://", "https://", "mailto:", "#")):
            continue
        target = unquote(target.split("#", 1)[0])
        if not target or target.startswith("/"):
            continue
        resolved = (path.parent / target).resolve()
        try:
            resolved.relative_to(ROOT)
        except ValueError:
            errors.append(f"{path.relative_to(ROOT)}: link escapes repo: {target}")
            continue
        if not resolved.exists():
            errors.append(f"{path.relative_to(ROOT)}: missing link: {target}")
    return errors


def require(path: Path, label: str, errors: list[str]) -> None:
    if not path.exists():
        errors.append(f"missing {label}: {path.relative_to(ROOT)}")


def check_help_structure() -> list[str]:
    errors: list[str] = []
    for name in REQUIRED_ROOT:
        require(HELP / name, "root help file", errors)
    for name in REQUIRED_ZH:
        require(HELP / "zh" / name, "Chinese help file", errors)
    for name in REQUIRED_EN:
        require(HELP / "en" / name, "English help file", errors)
    cname = HELP / "CNAME"
    if cname.exists() and cname.read_text(encoding="utf-8").strip() != "neca.nekobiglazyer.work":
        errors.append("docs/help/CNAME must be neca.nekobiglazyer.work")
    zh_sidebar = HELP / "zh" / "_sidebar.md"
    en_sidebar = HELP / "en" / "_sidebar.md"
    if zh_sidebar.exists() and "https://neca.nekobiglazyer.work/en/" not in zh_sidebar.read_text(encoding="utf-8"):
        errors.append("docs/help/zh/_sidebar.md: missing English entry")
    if en_sidebar.exists() and "https://neca.nekobiglazyer.work/zh/" not in en_sidebar.read_text(encoding="utf-8"):
        errors.append("docs/help/en/_sidebar.md: missing Chinese entry")
    sitemap = HELP / "sitemap.xml"
    if sitemap.exists():
        sm = sitemap.read_text(encoding="utf-8")
        for target in ("https://neca.nekobiglazyer.work/", "/zh/", "/en/"):
            if target not in sm:
                errors.append(f"docs/help/sitemap.xml: missing {target}")
    root_index = HELP / "index.html"
    if root_index.exists():
        root_html = root_index.read_text(encoding="utf-8")
        for target in ("/zh/", "/en/"):
            if target not in root_html:
                errors.append(f"docs/help/index.html: missing {target}")
    return errors


def main() -> int:
    errors: list[str] = []
    docs = tracked_markdown()
    for path in docs:
        try:
            text = path.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            errors.append(f"{path.relative_to(ROOT)}: not UTF-8")
            continue
        errors.extend(check_fences(path, text))
        errors.extend(check_links(path, text))
    errors.extend(check_help_structure())
    if errors:
        print("[ERR]  documentation check failed:", file=sys.stderr)
        for error in errors:
            print("  - " + error, file=sys.stderr)
        return 1
    print(f"[OK]   docs: {len(docs)} Markdown files; links, fences and help structure passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
