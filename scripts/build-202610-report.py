"""Snapshot ACG Secrets' October 2026 list for the local viewer report.

Run from the repository root: python scripts/build-202610-report.py
The generated report never loads images or scripts from the source website.
"""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import urllib.request
from datetime import date
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urlparse


SOURCE = "https://acgsecrets.hk/bangumi/202610/"
ROOT = Path(__file__).resolve().parents[1]
DATA_PATH = ROOT / "frontend/apps/viewer/src/seasonal/202610/data.json"
COVER_DIR = ROOT / "frontend/apps/viewer/public/seasonal/202610/covers"
HEADERS = {"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124 Safari/537.36"}


class Node:
    def __init__(self, tag="", attrs=(), parent=None):
        self.tag = tag
        self.attrs = dict(attrs)
        self.parent = parent
        self.children = []

    def has(self, name):
        return name in self.attrs.get("class", "").split()

    def text(self):
        return "".join(child.text() if isinstance(child, Node) else child for child in self.children)

    def find(self, predicate):
        found = [self] if predicate(self) else []
        for child in self.children:
            if isinstance(child, Node):
                found.extend(child.find(predicate))
        return found

    def first(self, predicate):
        found = self.find(predicate)
        return found[0] if found else None


class Parser(HTMLParser):
    VOID = {"area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"}

    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.root = Node()
        self.current = self.root

    def handle_starttag(self, tag, attrs):
        node = Node(tag, attrs, self.current)
        self.current.children.append(node)
        if tag not in self.VOID:
            self.current = node

    def handle_startendtag(self, tag, attrs):
        self.handle_starttag(tag, attrs)

    def handle_endtag(self, tag):
        node = self.current
        while node.parent and node.tag != tag:
            node = node.parent
        if node.parent:
            self.current = node.parent

    def handle_data(self, data):
        self.current.children.append(data)


def clean(value):
    return re.sub(r"\s+", " ", value or "").strip(" 、，\n\t")


def prose(node):
    parts = []
    def visit(item):
        if isinstance(item, str):
            parts.append(item)
        elif item.tag == "br":
            parts.append("\n")
        else:
            for child in item.children:
                visit(child)
            if item.tag in {"p", "div"}:
                parts.append("\n")
    visit(node)
    return re.sub(r"\n{3,}", "\n\n", "\n".join(clean(line) for line in "".join(parts).splitlines()).strip())


def get(url):
    request = urllib.request.Request(url, headers=HEADERS)
    with urllib.request.urlopen(request, timeout=35) as response:
        return response.read()


def people(card, section_class):
    section = card.first(lambda node: node.has(section_class))
    if not section:
        return []
    entries = []
    for row in section.find(lambda node: node.has("anime_person") and not node.has("hide")):
        role = row.first(lambda node: node.has("type"))
        names = [clean(node.text()) for node in row.find(lambda node: node.has("entities-block"))]
        if role and names:
            entries.append({"role": clean(role.text()), "names": [name for name in names if name]})
    return entries


def parse_sort_date(text):
    match = re.search(r"(\d{1,2})月(\d{1,2})日[^／]*／[^／]*／(\d{1,2})時(\d{1,2})分", text)
    if not match:
        return ""
    month, day, hour, minute = map(int, match.groups())
    return f"2026-{month:02d}-{day:02d}T{hour:02d}:{minute:02d}:00+09:00"


def parse_card(card):
    title_node = card.first(lambda node: node.tag == "h3" and node.has("entity_localized_name"))
    if not title_node:
        return None
    original_node = card.first(lambda node: node.has("entity_original_name"))
    anchor_node = card.first(lambda node: node.has("anime_anchor"))
    cover_node = card.first(lambda node: node.has("anime_cover_image"))
    cover_image = cover_node.first(lambda node: node.tag == "img") if cover_node else None
    tags_node = card.first(lambda node: node.has("anime_tag"))
    time_node = card.first(lambda node: node.has("time_today") and node.has("main_time"))
    tomorrow_node = card.first(lambda node: node.has("time_tomorrow") and node.has("main_time"))
    story_node = card.first(lambda node: node.has("anime_story"))
    tags = [clean(node.text()) for node in tags_node.find(lambda node: node.tag == "tags")] if tags_node else []
    schedule = clean(time_node.text()) if time_node else "首播时间待定"
    sort_text = clean(tomorrow_node.text()) if tomorrow_node else schedule
    return {
        "id": anchor_node.attrs.get("id", "") if anchor_node else "",
        "title": clean(title_node.text()),
        "originalTitle": clean(original_node.text()) if original_node else "",
        "tags": [tag for tag in tags if tag],
        "schedule": schedule,
        "premiereAt": parse_sort_date(sort_text),
        "story": prose(story_node) if story_node else "",
        "cast": people(card, "anime_cast"),
        "staff": people(card, "anime_staff"),
        "sourceUrl": SOURCE + ("#" + anchor_node.attrs["id"] if anchor_node and anchor_node.attrs.get("id") else ""),
        "coverSource": cover_image.attrs.get("src", "") if cover_image else "",
    }


def main():
    parser = Parser()
    parser.feed(get(SOURCE).decode("utf-8"))
    cards = parser.root.find(lambda node: node.has("acgs-anime"))
    entries = [entry for card in cards if (entry := parse_card(card))]
    if len(entries) < 30:
        raise RuntimeError(f"Expected the complete seasonal list; got only {len(entries)} works")
    COVER_DIR.mkdir(parents=True, exist_ok=True)
    for index, entry in enumerate(entries, 1):
        source = entry.pop("coverSource")
        entry["cover"] = ""
        if not source or urlparse(source).hostname not in {"static.acgsecrets.hk", "static2.acgsecrets.hk"}:
            continue
        extension = Path(urlparse(source).path).suffix.lower()
        if extension not in {".jpg", ".jpeg", ".png", ".webp"}:
            continue
        original = COVER_DIR / f"{index:02d}{extension}"
        optimized = COVER_DIR / f"{index:02d}.webp"
        if not optimized.exists() and not original.exists():
            try:
                original.write_bytes(get(source))
            except Exception as error:
                print(f"Cover unavailable for {entry['title']}: {error}")
                continue
        if not optimized.exists() and shutil.which("ffmpeg"):
            result = subprocess.run([
                "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", str(original),
                "-vf", "scale='min(640,iw)':-2", "-c:v", "libwebp", "-quality", "80", str(optimized),
            ], capture_output=True)
            if result.returncode != 0 or not optimized.exists() or optimized.stat().st_size == 0:
                optimized.unlink(missing_ok=True)
                print(f"WebP conversion failed for {entry['title']}: {result.stderr.decode(errors='replace')[:160]}")
        if optimized.exists():
            original.unlink(missing_ok=True)
            entry["cover"] = f"/seasonal/202610/covers/{optimized.name}"
        elif original.exists():
            entry["cover"] = f"/seasonal/202610/covers/{original.name}"
    DATA_PATH.parent.mkdir(parents=True, exist_ok=True)
    DATA_PATH.write_text(json.dumps({"source": SOURCE, "snapshotDate": date.today().isoformat(), "works": entries}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"Saved {len(entries)} works, {sum(bool(e['cover']) for e in entries)} local covers to {DATA_PATH}")


if __name__ == "__main__":
    main()
