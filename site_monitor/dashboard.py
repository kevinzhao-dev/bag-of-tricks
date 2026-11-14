#!/usr/bin/env python3
import json
import os
import re
import sqlite3
import threading
import time
from contextlib import contextmanager
from datetime import date, datetime, timedelta
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Dict, Iterator, List, Optional
from urllib.parse import urljoin, urlsplit

import requests
from bs4 import BeautifulSoup, Tag

BASE_URL = os.getenv("SITE_URL", "https://javip.net/")
DB_PATH = os.getenv("DB_PATH", "videos.db")
START_DATE_STR = os.getenv("START_DATE", "2025-12-25")
TOP_N = int(os.getenv("TOP_N", "20"))
TIMEOUT_SEC = int(os.getenv("TIMEOUT_SEC", "20"))
REQUEST_DELAY_SEC = float(os.getenv("REQUEST_DELAY_SEC", "0.5"))
DEFAULT_INTERVAL_MIN = int(os.getenv("INTERVAL_MINUTES", "30"))
BACKFILL_MAX_PAGES = int(os.getenv("BACKFILL_MAX_PAGES", "10"))
PORT = int(os.getenv("PORT", "8765"))

START_DATE = date.fromisoformat(START_DATE_STR)
FETCH_LOCK = threading.Lock()
DB_LOCK = threading.Lock()


def now_str() -> str:
    return time.strftime("%Y-%m-%d %H:%M:%S")


@contextmanager
def db_connect() -> Iterator[sqlite3.Connection]:
    conn = sqlite3.connect(DB_PATH, timeout=30, isolation_level=None)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA busy_timeout=30000")
    try:
        yield conn
    finally:
        conn.close()


def init_db() -> None:
    with DB_LOCK:
        with db_connect() as conn:
            conn.execute("PRAGMA journal_mode=WAL")
            conn.execute("PRAGMA synchronous=NORMAL")
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS videos (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    url TEXT UNIQUE NOT NULL,
                    title TEXT,
                    cover_url TEXT,
                    images TEXT,
                    rapidgator_url TEXT,
                    rapidgator_urls TEXT,
                    post_date TEXT,
                    discovered_at TEXT NOT NULL,
                    viewed_at TEXT,
                    downloaded_at TEXT
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS settings (
                    key TEXT PRIMARY KEY,
                    value TEXT NOT NULL
                )
                """
            )
            if get_setting(conn, "interval_minutes", None) is None:
                set_setting(conn, "interval_minutes", str(DEFAULT_INTERVAL_MIN))
            if get_setting(conn, "last_checked", None) is None:
                set_setting(conn, "last_checked", "")
            ensure_video_columns(conn)


def ensure_video_columns(conn: sqlite3.Connection) -> None:
    columns = {row["name"] for row in conn.execute("PRAGMA table_info(videos)").fetchall()}
    if "rapidgator_urls" not in columns:
        conn.execute("ALTER TABLE videos ADD COLUMN rapidgator_urls TEXT")
    if "images" not in columns:
        conn.execute("ALTER TABLE videos ADD COLUMN images TEXT")


def get_setting(conn: sqlite3.Connection, key: str, default: Optional[str]) -> Optional[str]:
    row = conn.execute("SELECT value FROM settings WHERE key = ?", (key,)).fetchone()
    if row is None:
        return default
    return row["value"]


def set_setting(conn: sqlite3.Connection, key: str, value: str) -> None:
    conn.execute("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", (key, value))


def fetch_html(url: str) -> str:
    if url.startswith("file://"):
        path = url[len("file://") :]
        with open(path, "r", encoding="utf-8") as f:
            return f.read()
    if os.path.exists(url):
        with open(url, "r", encoding="utf-8") as f:
            return f.read()
    headers = {
        "User-Agent": "site-monitor/1.0 (+local dashboard; respectful crawling)",
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
    }
    response = requests.get(url, headers=headers, timeout=TIMEOUT_SEC)
    response.raise_for_status()
    return response.text


def is_fetchable_url(url: str) -> bool:
    if url.startswith(("http://", "https://", "file://")):
        return True
    return os.path.exists(url)


def extract_post_links(html: str, base_url: str, top_n: int) -> List[str]:
    soup = BeautifulSoup(html, "lxml")
    selectors = [
        "h2.entry-title a",
        "h3.entry-title a",
        "article h2 a",
        "article h3 a",
        "h2 a",
        "h3 a",
    ]

    links: List[str] = []
    for sel in selectors:
        for anchor in soup.select(sel):
            href = anchor.get("href")
            if not href:
                continue
            href = urljoin(base_url, href)
            if is_fetchable_url(href):
                links.append(href.split("#")[0])
        if len(links) >= top_n:
            break

    seen = set()
    uniq = []
    for link in links:
        if link not in seen:
            uniq.append(link)
            seen.add(link)
    return uniq[:top_n]


def extract_listing_posts(html: str, base_url: str) -> List[Dict[str, Optional[str]]]:
    soup = BeautifulSoup(html, "lxml")
    posts: List[Dict[str, Optional[str]]] = []

    for container in soup.select("div.post, article"):
        link_node = container.select_one(
            "h2.entry-title a, h3.entry-title a, article h2 a, article h3 a, h2 a, h3 a"
        )
        if not link_node:
            continue
        href = link_node.get("href")
        if not href:
            continue
        href = urljoin(base_url, href)
        if not is_fetchable_url(href):
            continue

        date_node = container.select_one(".p_date, .entry-date, time")
        date_text = ""
        if date_node:
            date_text = date_node.get("datetime") or date_node.get_text(strip=True) or ""
        post_date = parse_date(date_text)
        posts.append({"url": href.split("#")[0], "post_date": post_date})

    if not posts:
        for link in extract_post_links(html, base_url, TOP_N):
            posts.append({"url": link, "post_date": None})

    seen = set()
    uniq: List[Dict[str, Optional[str]]] = []
    for post in posts:
        if post["url"] not in seen:
            uniq.append(post)
            seen.add(post["url"])
    return uniq


def extract_next_page_url(html: str, base_url: str) -> Optional[str]:
    soup = BeautifulSoup(html, "lxml")
    link_tag = soup.find("link", rel="next")
    if link_tag and link_tag.get("href"):
        return urljoin(base_url, link_tag.get("href"))

    selectors = [
        "a.next",
        "a.nextpostslink",
        ".pagenavi a.nextpostslink",
        "a[rel='next']",
        ".nav-previous a",
    ]
    for sel in selectors:
        node = soup.select_one(sel)
        if node and node.get("href"):
            return urljoin(base_url, node.get("href"))
    return None


def parse_date(text: str) -> Optional[str]:
    if not text:
        return None
    match = re.search(r"\d{4}[./-]\d{2}[./-]\d{2}", text)
    if not match:
        return None
    raw = match.group(0)
    for fmt in ("%Y.%m.%d", "%Y/%m/%d", "%Y-%m-%d"):
        try:
            parsed = datetime.strptime(raw, fmt).date()
            return parsed.isoformat()
        except ValueError:
            continue
    return None


def http_url(value: str, base_url: str = "") -> Optional[str]:
    if not value.strip():
        return None
    try:
        resolved = urljoin(base_url, value.strip())
        parts = urlsplit(resolved)
    except ValueError:
        return None
    if parts.scheme in ("http", "https") and parts.hostname:
        return resolved
    return None


def is_rapidgator_file(url: str) -> bool:
    resolved = http_url(url)
    if not resolved:
        return False
    parts = urlsplit(resolved)
    return (
        parts.hostname in ("rapidgator.net", "www.rapidgator.net")
        and parts.path.startswith("/file/")
        and bool(parts.path.split("/")[2])
    )


def extract_images(entry: Optional[Tag], url: str, cover_url: Optional[str]) -> List[Dict[str, str]]:
    if entry is None:
        return []

    images: Dict[str, Dict[str, str]] = {}
    for node in entry.select("a[href], img"):
        if node.name == "a":
            target = http_url(node["href"], url)
            if not target:
                continue
            parts = urlsplit(target)
            if parts.hostname not in ("imgpv.com", "www.imgpv.com") and not re.search(
                r"\.(?:avif|gif|jpe?g|png|webp)(?:\.html)?$", parts.path, re.IGNORECASE
            ):
                continue
            img = node.find("img")
        else:
            target = None
            img = node
            # A linked thumbnail is represented by its image-page URL above.
            anchor = img.find_parent("a", href=True)
            if anchor and http_url(anchor["href"], url) in images:
                continue

        thumbnail = http_url(img.get("data-src") or img.get("src") or "", url) if img else None
        target = target or thumbnail
        if not target or (node.name == "img" and target == cover_url):
            continue
        image = images.setdefault(target, {"url": target})
        if thumbnail:
            image.setdefault("thumbnail_url", thumbnail)

    return list(images.values())


def extract_post_details(html: str, url: str) -> Dict[str, Any]:
    soup = BeautifulSoup(html, "lxml")
    title = None
    for sel in ("h2.title", "h1.entry-title", "h2.entry-title", "h1", "title"):
        node = soup.select_one(sel)
        if node:
            text = node.get_text(" ", strip=True)
            if text:
                title = text
                break

    date_text = None
    date_node = soup.select_one(".p_date")
    if date_node:
        date_text = date_node.get_text(strip=True)
    post_date = parse_date(date_text or "")

    entry = soup.select_one("div.entry, .entry-content") or soup.select_one("article")
    cover_url = None
    og_image = soup.find("meta", property="og:image")
    if og_image and og_image.get("content"):
        cover_url = og_image.get("content", "").strip()
    if not cover_url:
        img = (entry if entry is not None else soup).find("img")
        if img and img.get("src"):
            cover_url = img.get("src", "").strip()
    if cover_url:
        cover_url = http_url(cover_url, url)

    rapidgator_urls = []
    for anchor in soup.find_all("a", href=True):
        href = http_url(anchor["href"], url)
        if href and is_rapidgator_file(href):
            rapidgator_urls.append(href)

    uniq_urls = list(dict.fromkeys(rapidgator_urls))
    rapidgator_url = uniq_urls[0] if uniq_urls else None

    return {
        "title": title,
        "cover_url": cover_url,
        "images": extract_images(entry, url, cover_url),
        "rapidgator_url": rapidgator_url,
        "rapidgator_urls": uniq_urls,
        "post_date": post_date,
    }


def should_skip(post_date: Optional[str], discovered_date: str) -> bool:
    if post_date and post_date < START_DATE_STR:
        return True
    if not post_date and discovered_date < START_DATE_STR:
        return True
    return False


def save_video(conn: sqlite3.Connection, url: str, details: Dict[str, Any]) -> None:
    images_json = json.dumps(details["images"]) if "images" in details else None
    rapidgator_urls_json = (
        json.dumps(details["rapidgator_urls"]) if "rapidgator_urls" in details else None
    )
    with DB_LOCK:
        conn.execute(
            """
            INSERT INTO videos
                (url, title, cover_url, images, rapidgator_url, rapidgator_urls, post_date, discovered_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(url) DO UPDATE SET
                title = COALESCE(excluded.title, title),
                cover_url = COALESCE(excluded.cover_url, cover_url),
                images = COALESCE(excluded.images, images),
                rapidgator_url = COALESCE(excluded.rapidgator_url, rapidgator_url),
                rapidgator_urls = COALESCE(excluded.rapidgator_urls, rapidgator_urls),
                post_date = COALESCE(excluded.post_date, post_date)
            """,
            (
                url,
                details.get("title"),
                details.get("cover_url"),
                images_json,
                details.get("rapidgator_url"),
                rapidgator_urls_json,
                details.get("post_date"),
                now_str(),
            ),
        )


def run_fetch_once() -> Dict[str, Any]:
    if not FETCH_LOCK.acquire(blocking=False):
        return {"status": "busy"}

    result = {"status": "ok", "new": 0, "found": 0, "errors": 0, "checked_at": ""}
    try:
        homepage = fetch_html(BASE_URL)
        links = extract_post_links(homepage, BASE_URL, TOP_N)
        if not links:
            details = extract_post_details(homepage, BASE_URL)
            if details.get("rapidgator_url"):
                links = [BASE_URL]
        result["found"] = len(links)
        today_str = date.today().isoformat()

        with db_connect() as conn:
            for link in links:
                existing = conn.execute("SELECT images FROM videos WHERE url = ?", (link,)).fetchone()
                if existing and existing["images"] is not None:
                    continue
                try:
                    post_html = fetch_html(link)
                except Exception:
                    result["errors"] += 1
                    continue

                details = extract_post_details(post_html, link)
                if not existing and should_skip(details.get("post_date"), today_str):
                    continue

                save_video(conn, link, details)
                if not existing:
                    result["new"] += 1
                if REQUEST_DELAY_SEC > 0:
                    time.sleep(REQUEST_DELAY_SEC)

            checked_at = now_str()
            with DB_LOCK:
                set_setting(conn, "last_checked", checked_at)
            result["checked_at"] = checked_at
    except Exception:
        result["status"] = "error"
        result["errors"] += 1
    finally:
        FETCH_LOCK.release()

    return result


def backfill_range(start_str: str, end_str: str) -> Dict[str, Any]:
    try:
        start_date = date.fromisoformat(start_str)
        end_date = date.fromisoformat(end_str)
    except ValueError:
        return {"status": "error", "error": "Invalid date format"}
    if end_date < start_date:
        start_date, end_date = end_date, start_date

    result = {
        "status": "ok",
        "start": start_date.isoformat(),
        "end": end_date.isoformat(),
        "pages": 0,
        "scanned": 0,
        "added": 0,
        "skipped_old": 0,
        "skipped_future": 0,
        "skipped_undated": 0,
        "errors": 0,
    }

    next_url = BASE_URL
    visited = set()

    print(
        f"Backfill {start_date.isoformat()} -> {end_date.isoformat()} (max pages {BACKFILL_MAX_PAGES})",
        flush=True,
    )
    with db_connect() as conn:
        while next_url and next_url not in visited and result["pages"] < BACKFILL_MAX_PAGES:
            visited.add(next_url)
            try:
                html = fetch_html(next_url)
            except Exception:
                result["errors"] += 1
                print(f"[page {result['pages'] + 1}] ERROR fetching {next_url}", flush=True)
                break

            posts = extract_listing_posts(html, BASE_URL)
            if not posts:
                print(f"[page {result['pages'] + 1}] No posts found on {next_url}", flush=True)
                break

            result["pages"] += 1
            page_has_newer = False
            page_has_unknown = False
            page_scanned = 0
            page_added = 0
            page_skipped = 0
            page_errors = 0

            for post in posts:
                listing_date = post.get("post_date")
                if listing_date:
                    if listing_date >= start_date.isoformat():
                        page_has_newer = True
                else:
                    page_has_unknown = True

            for post in posts:
                url = post.get("url")
                listing_date = post.get("post_date")
                if not url:
                    continue
                page_scanned += 1
                result["scanned"] += 1
                exists = conn.execute("SELECT 1 FROM videos WHERE url = ?", (url,)).fetchone()
                if exists:
                    continue

                if listing_date and listing_date < start_date.isoformat():
                    result["skipped_old"] += 1
                    page_skipped += 1
                    continue
                if listing_date and listing_date > end_date.isoformat():
                    result["skipped_future"] += 1
                    page_skipped += 1
                    continue

                try:
                    post_html = fetch_html(url)
                except Exception:
                    result["errors"] += 1
                    page_errors += 1
                    continue

                details = extract_post_details(post_html, url)
                effective_date = details.get("post_date") or listing_date
                if effective_date:
                    if effective_date < start_date.isoformat():
                        result["skipped_old"] += 1
                        page_skipped += 1
                        continue
                    if effective_date > end_date.isoformat():
                        result["skipped_future"] += 1
                        page_skipped += 1
                        continue
                else:
                    result["skipped_undated"] += 1
                    page_skipped += 1
                    continue

                save_video(conn, url, details)
                result["added"] += 1
                page_added += 1
                if REQUEST_DELAY_SEC > 0:
                    time.sleep(REQUEST_DELAY_SEC)

            next_url = extract_next_page_url(html, BASE_URL)
            print(
                f"[page {result['pages']}] scanned {page_scanned}, added {page_added}, skipped {page_skipped}, errors {page_errors}",
                flush=True,
            )
            if not page_has_newer and not page_has_unknown:
                break

        with DB_LOCK:
            set_setting(conn, "last_checked", now_str())

    return result


def parse_rapidgator_urls(row: Any) -> List[str]:
    if isinstance(row, sqlite3.Row):
        raw = row["rapidgator_urls"]
        single = row["rapidgator_url"]
    else:
        raw = row.get("rapidgator_urls")
        single = row.get("rapidgator_url")
    urls = [single] if single else []
    if raw:
        try:
            parsed = json.loads(raw)
        except json.JSONDecodeError:
            parsed = raw.splitlines()
        if isinstance(parsed, list):
            urls = parsed
    return list(dict.fromkeys(
        url.strip() for url in urls if isinstance(url, str) and is_rapidgator_file(url)
    ))


def normalize_rows(rows: List[sqlite3.Row]) -> List[Dict[str, Any]]:
    items: List[Dict[str, Any]] = []
    for row in rows:
        item = dict(row)
        urls = parse_rapidgator_urls(row)
        if item["cover_url"]:
            item["cover_url"] = http_url(item["cover_url"])
        item["images"] = json.loads(row["images"]) if row["images"] is not None else None
        item["rapidgator_urls"] = urls
        item["rapidgator_url"] = urls[0] if urls else None
        items.append(item)
    return items


def list_new_videos() -> List[Dict[str, Any]]:
    with db_connect() as conn:
        rows = conn.execute(
            """
            SELECT id, url, title, cover_url, images, rapidgator_url, rapidgator_urls,
                   post_date, discovered_at, downloaded_at
            FROM videos
            WHERE viewed_at IS NULL
              AND (
                (post_date IS NOT NULL AND post_date >= ?)
                OR (post_date IS NULL AND substr(discovered_at, 1, 10) >= ?)
              )
            ORDER BY COALESCE(post_date, substr(discovered_at, 1, 10)) DESC, discovered_at DESC
            """,
            (START_DATE_STR, START_DATE_STR),
        ).fetchall()
    return normalize_rows(rows)


def list_all_videos() -> List[Dict[str, Any]]:
    with db_connect() as conn:
        rows = conn.execute(
            """
            SELECT id, url, title, cover_url, images, rapidgator_url, rapidgator_urls,
                   post_date, discovered_at, viewed_at, downloaded_at
            FROM videos
            ORDER BY COALESCE(post_date, substr(discovered_at, 1, 10)) DESC, discovered_at DESC
            """
        ).fetchall()
    return normalize_rows(rows)


def get_video_media(video_id: int) -> Optional[Dict[str, Any]]:
    with db_connect() as conn:
        query = """
            SELECT url, cover_url, images, rapidgator_url, rapidgator_urls, downloaded_at
            FROM videos WHERE id = ?
        """
        row = conn.execute(query, (video_id,)).fetchone()
        if row is None:
            return None
        if row["images"] is None:
            try:
                html = fetch_html(row["url"])
            except (OSError, requests.RequestException):
                # Leave the row eligible for a retry and show the saved links meanwhile.
                pass
            else:
                save_video(conn, row["url"], extract_post_details(html, row["url"]))
            row = conn.execute(query, (video_id,)).fetchone()

    return normalize_rows([row])[0]


def mark_viewed(video_id: int) -> None:
    with DB_LOCK:
        with db_connect() as conn:
            conn.execute(
                "UPDATE videos SET viewed_at = ? WHERE id = ?",
                (now_str(), video_id),
            )


def mark_all_viewed() -> int:
    with DB_LOCK:
        with db_connect() as conn:
            cursor = conn.execute(
                """
                UPDATE videos
                SET viewed_at = ?
                WHERE viewed_at IS NULL
                  AND (
                    (post_date IS NOT NULL AND post_date >= ?)
                    OR (post_date IS NULL AND substr(discovered_at, 1, 10) >= ?)
                  )
                """,
                (now_str(), START_DATE_STR, START_DATE_STR),
            )
            return cursor.rowcount


def mark_downloaded(video_id: int) -> Optional[str]:
    urls = mark_downloaded_and_get_links(video_id)
    return urls[0] if urls else None


def mark_downloaded_and_get_links(video_id: int) -> List[str]:
    with DB_LOCK:
        with db_connect() as conn:
            row = conn.execute(
                "SELECT rapidgator_url, rapidgator_urls FROM videos WHERE id = ?", (video_id,)
            ).fetchone()
            urls = parse_rapidgator_urls(row) if row else []
            if not urls:
                return []
            conn.execute(
                "UPDATE videos SET downloaded_at = ? WHERE id = ?",
                (now_str(), video_id),
            )
            return urls


def get_interval_minutes() -> int:
    with db_connect() as conn:
        value = get_setting(conn, "interval_minutes", str(DEFAULT_INTERVAL_MIN))
    try:
        parsed = int(value)
    except ValueError:
        parsed = DEFAULT_INTERVAL_MIN
    return max(10, min(parsed, 360))


def set_interval_minutes(minutes: int) -> int:
    normalized = max(10, min(int(minutes), 360))
    with DB_LOCK:
        with db_connect() as conn:
            set_setting(conn, "interval_minutes", str(normalized))
    return normalized


def get_status_payload() -> Dict[str, str]:
    with db_connect() as conn:
        last_checked = get_setting(conn, "last_checked", "") or ""
        interval_minutes = get_setting(conn, "interval_minutes", str(DEFAULT_INTERVAL_MIN)) or str(
            DEFAULT_INTERVAL_MIN
        )
    return {
        "last_checked": last_checked,
        "interval_minutes": interval_minutes,
        "start_date": START_DATE_STR,
    }


def get_daily_counts() -> Dict[str, Any]:
    today = date.today()
    window_start = today - timedelta(days=29)
    if START_DATE > window_start:
        window_start = START_DATE
    if window_start > today:
        return {"daily": [], "start": window_start.isoformat(), "end": today.isoformat(), "max": 0}

    start_str = window_start.isoformat()
    end_str = today.isoformat()
    with db_connect() as conn:
        rows = conn.execute(
            """
            SELECT COALESCE(post_date, substr(discovered_at, 1, 10)) AS day, COUNT(*) AS count
            FROM videos
            WHERE (
                (post_date IS NOT NULL AND post_date BETWEEN ? AND ?)
                OR (post_date IS NULL AND substr(discovered_at, 1, 10) BETWEEN ? AND ?)
            )
            GROUP BY day
            """,
            (start_str, end_str, start_str, end_str),
        ).fetchall()

    counts = {row["day"]: row["count"] for row in rows}
    daily = []
    cursor = window_start
    max_count = 0
    while cursor <= today:
        day_str = cursor.isoformat()
        count = int(counts.get(day_str, 0))
        daily.append({"date": day_str, "count": count})
        if count > max_count:
            max_count = count
        cursor += timedelta(days=1)

    return {"daily": daily, "start": start_str, "end": end_str, "max": max_count}


def get_summary_stats() -> Dict[str, Any]:
    today = date.today()
    if START_DATE > today:
        days_covered = 0
    else:
        days_covered = (today - START_DATE).days + 1

    with db_connect() as conn:
        base_filter = """
            (
                (post_date IS NOT NULL AND post_date >= ?)
                OR (post_date IS NULL AND substr(discovered_at, 1, 10) >= ?)
            )
        """
        params = (START_DATE_STR, START_DATE_STR)

        total = conn.execute(f"SELECT COUNT(*) AS c FROM videos WHERE {base_filter}", params).fetchone()["c"]
        viewed = conn.execute(
            f"SELECT COUNT(*) AS c FROM videos WHERE viewed_at IS NOT NULL AND {base_filter}", params
        ).fetchone()["c"]
        downloaded = conn.execute(
            f"SELECT COUNT(*) AS c FROM videos WHERE downloaded_at IS NOT NULL AND {base_filter}", params
        ).fetchone()["c"]
        unseen = conn.execute(
            f"""
            SELECT COUNT(*) AS c FROM videos
            WHERE viewed_at IS NULL AND {base_filter}
            """,
            params,
        ).fetchone()["c"]

        most_active = conn.execute(
            f"""
            SELECT COALESCE(post_date, substr(discovered_at, 1, 10)) AS day, COUNT(*) AS count
            FROM videos
            WHERE {base_filter}
            GROUP BY day
            ORDER BY count DESC, day DESC
            LIMIT 1
            """,
            params,
        ).fetchone()

        last_downloaded = conn.execute("SELECT MAX(downloaded_at) AS d FROM videos").fetchone()["d"]
        last_viewed = conn.execute("SELECT MAX(viewed_at) AS v FROM videos").fetchone()["v"]

    avg_per_day = round((total / days_covered) if days_covered else 0, 2)
    download_rate = round((downloaded / total) * 100, 1) if total else 0.0

    return {
        "total": total,
        "unseen": unseen,
        "viewed": viewed,
        "downloaded": downloaded,
        "avg_per_day": avg_per_day,
        "download_rate": download_rate,
        "days_covered": days_covered,
        "most_active_day": most_active["day"] if most_active else "",
        "most_active_count": most_active["count"] if most_active else 0,
        "last_downloaded": last_downloaded or "",
        "last_viewed": last_viewed or "",
    }


class DashboardHandler(BaseHTTPRequestHandler):
    def _send_json(self, payload: Dict[str, Any], status: int = 200) -> None:
        data = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def _send_text(self, data: str, content_type: str, status: int = 200) -> None:
        body = data.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _read_json_body(self) -> Optional[Dict[str, Any]]:
        length = int(self.headers.get("Content-Length", "0") or "0")
        if length <= 0:
            return None
        raw = self.rfile.read(length)
        try:
            return json.loads(raw.decode("utf-8"))
        except json.JSONDecodeError:
            return None

    def do_GET(self) -> None:
        if self.path == "/" or self.path.startswith("/index.html"):
            try:
                with open("dashboard.html", "r", encoding="utf-8") as f:
                    self._send_text(f.read(), "text/html; charset=utf-8")
            except FileNotFoundError:
                self._send_text("dashboard.html not found", "text/plain; charset=utf-8", 404)
            return

        if self.path.startswith("/api/items"):
            items = list_new_videos()
            self._send_json({"items": items})
            return

        if self.path.startswith("/api/media/"):
            try:
                video_id = int(self.path.removeprefix("/api/media/"))
            except ValueError:
                self._send_json({"error": "Invalid id"}, 400)
                return
            media = get_video_media(video_id)
            if media is None:
                self._send_json({"error": "Video not found"}, 404)
            else:
                self._send_json(media)
            return

        if self.path.startswith("/image/"):
            try:
                video_id = int(self.path.removeprefix("/image/"))
            except ValueError:
                self._send_text("Invalid image id", "text/plain; charset=utf-8", 400)
                return
            media = get_video_media(video_id)
            if media is None:
                self._send_text("Image not found", "text/plain; charset=utf-8", 404)
                return
            images = media["images"]
            target = images[0]["url"] if images else media["cover_url"] or http_url(media["url"])
            if not target:
                self._send_text("Image not found", "text/plain; charset=utf-8", 404)
                return
            self.send_response(302)
            self.send_header("Location", requests.utils.requote_uri(target))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            return

        if self.path.startswith("/api/settings"):
            self._send_json(get_status_payload())
            return

        if self.path.startswith("/api/stats"):
            self._send_json(get_daily_counts())
            return

        if self.path.startswith("/api/all"):
            items = list_all_videos()
            self._send_json({"items": items})
            return

        if self.path.startswith("/api/summary"):
            self._send_json(get_summary_stats())
            return

        if self.path.startswith("/download/"):
            try:
                video_id = int(self.path.split("/download/")[1])
            except (IndexError, ValueError):
                self._send_text("Invalid download id", "text/plain; charset=utf-8", 400)
                return
            rapidgator_url = mark_downloaded(video_id)
            if not rapidgator_url:
                self._send_text("Download link not found", "text/plain; charset=utf-8", 404)
                return
            self.send_response(302)
            self.send_header("Location", rapidgator_url)
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            return

        self._send_text("Not found", "text/plain; charset=utf-8", 404)

    def do_POST(self) -> None:
        if self.path.startswith("/api/view-all"):
            count = mark_all_viewed()
            self._send_json({"ok": True, "count": count})
            return

        if self.path.startswith("/api/view"):
            body = self._read_json_body() or {}
            try:
                video_id = int(body.get("id", 0))
            except (TypeError, ValueError):
                self._send_json({"error": "Invalid id"}, 400)
                return
            mark_viewed(video_id)
            self._send_json({"ok": True})
            return

        if self.path.startswith("/api/refresh"):
            result = run_fetch_once()
            self._send_json(result)
            return

        if self.path.startswith("/api/download"):
            body = self._read_json_body() or {}
            try:
                video_id = int(body.get("id", 0))
            except (TypeError, ValueError):
                self._send_json({"error": "Invalid id"}, 400)
                return
            urls = mark_downloaded_and_get_links(video_id)
            if not urls:
                self._send_json({"error": "Download link not found"}, 404)
                return
            self._send_json({"ok": True, "urls": urls})
            return

        if self.path.startswith("/api/settings"):
            body = self._read_json_body() or {}
            try:
                minutes = int(body.get("interval_minutes", DEFAULT_INTERVAL_MIN))
            except (TypeError, ValueError):
                self._send_json({"error": "Invalid interval"}, 400)
                return
            normalized = set_interval_minutes(minutes)
            payload = get_status_payload()
            payload["interval_minutes"] = str(normalized)
            self._send_json(payload)
            return

        self._send_text("Not found", "text/plain; charset=utf-8", 404)

    def log_message(self, format: str, *args: Any) -> None:
        return


def scheduler_loop() -> None:
    while True:
        run_fetch_once()
        time.sleep(get_interval_minutes() * 60)


def main() -> None:
    init_db()
    if "--backfill" in os.sys.argv:
        args = os.sys.argv
        idx = args.index("--backfill")
        start_arg = args[idx + 1] if len(args) > idx + 1 else os.getenv("BACKFILL_START", "")
        end_arg = args[idx + 2] if len(args) > idx + 2 else os.getenv("BACKFILL_END", "")
        if not start_arg:
            print("Usage: dashboard.py --backfill YYYY-MM-DD [YYYY-MM-DD]")
            return
        if not end_arg:
            end_arg = date.today().isoformat()
        result = backfill_range(start_arg, end_arg)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return
    if "--once" in os.sys.argv:
        run_fetch_once()
        return

    thread = threading.Thread(target=scheduler_loop, daemon=True)
    thread.start()

    server = ThreadingHTTPServer(("127.0.0.1", PORT), DashboardHandler)
    print(f"Dashboard running at http://127.0.0.1:{PORT}")
    server.serve_forever()


if __name__ == "__main__":
    main()
