# Site Monitor Dashboard

Local web dashboard to monitor a video sharing site for new posts, store them in
SQLite, and show items until you mark them as viewed.

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

Open `http://127.0.0.1:8765`.

## Start at login on macOS

Install a per-user LaunchAgent:

```bash
./install-launchagent.sh
```

This starts the dashboard when the current macOS user logs in. The LaunchAgent
runs `run-dashboard.sh`, which sources the repo-wide venv from the parent
directory before starting `dashboard.py`.

Useful commands:

```bash
launchctl print gui/$(id -u)/com.kevin.site-monitor-dashboard
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.kevin.site-monitor-dashboard.plist
tail -f ~/Library/Logs/site-monitor-dashboard.log
tail -f ~/Library/Logs/site-monitor-dashboard.err.log
```

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
- Click a cover to open the first image linked in the original post. Additional
  images are linked on the card.
- Clicking "Download" opens Rapidgator and marks single-file posts as downloaded.
  For multiple files, open each link in the download dialog and select
  "Mark downloaded" when done. Downloaded cards show a checkmark and stay in the
  list until you mark them as viewed.
- Existing posts load image links from the source on the next crawl or when you
  first open their image or download controls.
- Manual refresh is available in the UI; the scheduler runs in the background.

## API endpoints

- `GET /api/items`: list unseen items
- `GET /api/media/<id>`: get image and download links, fetching missing metadata on demand
- `POST /api/view`: mark viewed (`{"id": 123}`)
- `GET /download/<id>`: mark downloaded and redirect
- `POST /api/refresh`: fetch now
- `GET /api/settings`: current settings
- `POST /api/settings`: update interval (`{"interval_minutes": 60}`)
