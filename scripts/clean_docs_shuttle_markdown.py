#!/usr/bin/env python3
"""Clean control markers leaked by docs-shuttle's snapshot converter."""

from __future__ import annotations

import argparse
import re
from pathlib import Path


CONTROL_RE = re.compile(r"[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]")
SECTION_RE = re.compile(
    r"^(?P<title>[一二三四五六七八九十]+、.+|"
    r"\d+(?:\.\d+){0,3}\s+.+)$"
)


def clean_line(line: str) -> str:
    line = CONTROL_RE.sub("", line)
    line = line.replace("\u200c", "").replace("\u200d", "")
    line = line.replace("\ufeff", "").strip()
    return line


def heading_for(line: str) -> str:
    match = SECTION_RE.match(line)
    if not match:
        return line

    title = match.group("title")
    if re.match(r"^[一二三四五六七八九十]+、", title):
        return f"# {title}"

    depth = title.split(maxsplit=1)[0].count(".") + 1
    return f"{'#' * min(depth, 4)} {title}"


def clean_markdown(content: str) -> str:
    lines = content.splitlines()
    result: list[str] = []
    in_frontmatter = False
    frontmatter_markers = 0

    for raw_line in lines:
        line = clean_line(raw_line)

        if line == "---" and frontmatter_markers < 2:
            frontmatter_markers += 1
            in_frontmatter = frontmatter_markers == 1
            result.append(line)
            continue

        if in_frontmatter:
            result.append(line)
            continue

        if line.startswith("title:"):
            line = line.replace("O\"", "\"", 1)

        if line.startswith("# "):
            line = "# " + clean_line(line[2:])
        else:
            line = heading_for(line)

        # Snapshot output can wrap control-prefixed table cells in bold markers.
        line = re.sub(r"^\*\*\s*(.*?)\s*\*\*$", r"**\1**", line)
        line = re.sub(r"\*\*\s*\*\*", "", line)
        result.append(line)

    content = "\n".join(result)
    content = re.sub(r"(?m)^#\s*$\n?", "", content)
    content = re.sub(r"\*\*\s*\n+\s*([^*\n]+)\*\*", r"**\1**", content)
    content = re.sub(r"\n{3,}", "\n\n", content)
    return content.rstrip() + "\n"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("input", type=Path)
    parser.add_argument("-o", "--output", type=Path)
    args = parser.parse_args()

    output = args.output or args.input
    content = args.input.read_text(encoding="utf-8")
    output.write_text(clean_markdown(content), encoding="utf-8")


if __name__ == "__main__":
    main()
