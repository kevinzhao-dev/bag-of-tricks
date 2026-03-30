# Site Monitor Dashboard

Local web dashboard to monitor a video sharing site for new posts, store them in
SQLite, and show only items you have not viewed or downloaded.

## What is included

- `dashboard.py`: crawler + SQLite database + HTTP API + scheduler
- `dashboard.html`: web UI served by the Python server

## Requirements

- Python 3.9+
- Dependencies: `requests`, `beautifulsoup4`, `lxml`

Install deps (example):

```bash
python -m pip install requests beautifulsoup4 lxml
```

## Quick start

```bash
python dashboard.py
```

Open `http://127.0.0.1:8000`.

## Environment variables

- `SITE_URL`: target site or local file (default `https://javip.net/`)
- `START_DATE`: ignore posts older than this date (`YYYY-MM-DD`, default `2025-12-25`)
- `INTERVAL_MINUTES`: auto fetch interval (default `30`, min 10, max 360)
- `TOP_N`: number of posts to scan from the homepage (default `20`)
- `TIMEOUT_SEC`: request timeout (default `20`)
- `REQUEST_DELAY_SEC`: delay between post fetches (default `0.5`)
- `DB_PATH`: SQLite file path (default `videos.db`)

## Behavior

- Only items with `post_date` >= `START_DATE` are shown.
- Clicking "Viewed" marks a post as viewed and hides it.
- Clicking "Download" marks a post as downloaded and redirects to Rapidgator.
- Manual refresh is available in the UI; the scheduler runs in the background.

## API endpoints

- `GET /api/items`: list unseen items
- `POST /api/view`: mark viewed (`{"id": 123}`)
- `GET /download/<id>`: mark downloaded and redirect
- `POST /api/refresh`: fetch now
- `GET /api/settings`: current settings
- `POST /api/settings`: update interval (`{"interval_minutes": 60}`)
