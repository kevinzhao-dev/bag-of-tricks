import os
import re
import unicodedata
from pathlib import Path
from typing import Dict, Iterable, Optional, Set
from urllib.parse import unquote, urlsplit


VIDEO_EXTENSIONS = {".avi", ".m4v", ".mkv", ".mov", ".mp4", ".mpeg", ".mpg", ".ts", ".webm", ".wmv"}
ARCHIVE_EXTENSIONS = {".7z", ".rar", ".zip"}
VIDEO_CODE = re.compile(
    r"(?<![a-z0-9])(?:fc2ppv-(?P<fc2>[0-9]{5,})"
    r"|(?P<dated>[0-9]{6}-[0-9]{2,3})"
    r"|(?P<catalog>(?:s-cute|[0-9]{0,4}[a-z]{2,12})-(?:no-?)?[0-9]{2,6}[a-z]?))"
    r"(?![a-z0-9])"
)
DATE_SERIES = {
    "10mu": ("10mu", "10musume"),
    "1pon": ("1pon", "1pondo"),
    "carib": ("carib", "caribbeancom"),
    "paco": ("paco", "pacopacomama"),
}


def normalize_name(name: str) -> str:
    name = unicodedata.normalize("NFKC", name).casefold()
    name = re.sub(r"\s*\([0-9]+\)$", "", name)  # Browser's duplicate filename suffix
    name = re.sub(r"(?<![a-z0-9])fc2[\s_-]*ppv[\s_-]*([0-9]+)", r"fc2ppv-\1", name)
    return re.sub(r"[-_\s]+", "-", name).strip("-")


def video_key(name: str, *, title: bool = False) -> Optional[str]:
    name = normalize_name(name)
    match = VIDEO_CODE.search(name)
    if not match:
        return None if title else name

    code = match[0]
    suffix = name[match.end():].lstrip("-")
    if match["dated"]:
        adjacent = [name[:match.start()].rstrip("-").rsplit("-", 1)[-1], suffix.split("-")[0]]
        series = next(
            (series for series, aliases in DATE_SERIES.items() if any(a in aliases for a in adjacent)),
            None,
        )
        if series:
            code = f"{code}-{series}"
            if suffix.split("-")[0] in DATE_SERIES[series]:
                suffix = suffix.partition("-")[2]
        elif title:
            return None

    if title:
        return code

    part = re.match(r"(?:part-?|pt-?)?([0-9]{1,3})(?=$|[-.])", suffix)
    if part:
        return f"{code}-part-{int(part[1])}"
    variant = re.match(r"(sp|special|sample|preview|trailer|gift|bonus|t)(?![a-z0-9])", suffix)
    if variant:
        return f"{code}-{variant[1]}"
    return code


def scan_downloads(directory: Path) -> Set[str]:
    keys: Set[str] = set()
    for root, directories, files in os.walk(directory):
        directories[:] = [
            name for name in directories
            if not name.startswith(".") and not name.casefold().endswith((".app", ".download"))
        ]
        for filename in files:
            if filename.startswith(".") or Path(filename).suffix.casefold() not in VIDEO_EXTENSIONS:
                continue
            try:
                path = Path(root, filename)
                if path.is_symlink() or not path.is_file() or path.stat().st_size == 0:
                    continue
            except OSError:
                continue
            key = video_key(path.stem)
            if key:
                keys.add(key)
    return keys


def match_downloads(title: str, urls: Iterable[str], local_keys: Set[str]) -> Dict[str, int]:
    expected: Set[str] = set()
    unnamed: Set[str] = set()
    for url in urls:
        file_id, _, filename = urlsplit(url).path.removeprefix("/file/").partition("/")
        name = Path(unquote(filename).rstrip("/"))
        if name.suffix.casefold() == ".html":
            name = name.with_suffix("")
        if name.suffix.casefold() in VIDEO_EXTENSIONS | ARCHIVE_EXTENSIONS:
            key = video_key(name.stem)
            if key:
                expected.add(key)
                continue
        unnamed.add(file_id)

    total = len(expected) + len(unnamed)
    found = len(expected & local_keys)
    if total <= 1 and not found and not any(VIDEO_CODE.search(key) for key in expected):
        key = video_key(title, title=True)
        if key:
            return {"found": int(key in local_keys), "total": 1}
    return {"found": found, "total": total}
