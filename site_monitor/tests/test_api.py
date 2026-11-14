import http.client
import importlib
import json
import os
import sqlite3
import tempfile
import threading
import unittest
from datetime import date
from pathlib import Path
from typing import Optional
from unittest.mock import patch


class DashboardDbConnectionTests(unittest.TestCase):
    def test_db_connect_closes_connection_after_context(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            os.environ["SITE_URL"] = f"file://{Path(tmpdir) / 'index.html'}"
            os.environ["DB_PATH"] = str(Path(tmpdir) / "test.db")

            import dashboard

            module = importlib.reload(dashboard)
            module.init_db()

            with module.db_connect() as conn:
                conn.execute("SELECT 1")

            with self.assertRaises(sqlite3.ProgrammingError):
                conn.execute("SELECT 1")


class DashboardApiTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.tmpdir = tempfile.TemporaryDirectory()
        base = Path(cls.tmpdir.name)
        today = date.today()
        date_dot = today.strftime("%Y.%m.%d")
        cls.today_iso = today.isoformat()

        index_html = base / "index.html"
        post1_html = base / "post1.html"
        post2_html = base / "post2.html"

        index_html.write_text(
            """
            <html><body>
              <h2 class="entry-title"><a href="post1.html">Post 1</a></h2>
              <h2 class="entry-title"><a href="post2.html">Post 2</a></h2>
            </body></html>
            """,
            encoding="utf-8",
        )

        post1_html.write_text(
            f"""
            <html><body>
              <h2 class="title">Sample One</h2>
              <div class="post_info"><span class="p_date">{date_dot}</span></div>
              <div class="entry">
                <img src="https://example.com/cover1.jpg" />
                <a href="https://imgpv.com/example/image1.jpg.html">
                  <img src="https://example.com/preview1.jpg" />
                </a>
                <a href="https://imgpv.com/example/image1.jpg.html">Duplicate image</a>
                <a href="https://example.com/image2.png">
                  <img data-src="https://example.com/preview2.jpg" />
                </a>
                <a href="https://rapidgator.net/file/aaaa/one.mp4.html">rg1</a>
                <a href="https://rapidgator.net/file/aaaa/one.mp4.html">Duplicate download</a>
                <a href="https://rapidgator.net/file/bbbb/one-gift.mp4.html">rg2</a>
              </div>
              <a href="https://rapidgator.net/article/premium/ref/123">
                <img src="https://example.com/banner.jpg" />
              </a>
            </body></html>
            """,
            encoding="utf-8",
        )

        post2_html.write_text(
            f"""
            <html><body>
              <h2 class="title">Sample Two</h2>
              <div class="post_info"><span class="p_date">{date_dot}</span></div>
              <div class="entry">
                <img src="https://example.com/cover2.jpg" />
                <a href="https://rapidgator.net/file/cccc/two.mp4.html">rg1</a>
                <a href="https://rapidgator.net/file/dddd/two-gift.mp4.html">rg2</a>
              </div>
            </body></html>
            """,
            encoding="utf-8",
        )

        os.environ["SITE_URL"] = f"file://{index_html}"
        os.environ["DB_PATH"] = str(base / "test.db")
        os.environ["START_DATE"] = cls.today_iso
        os.environ["TOP_N"] = "10"
        os.environ["REQUEST_DELAY_SEC"] = "0"
        os.environ["TIMEOUT_SEC"] = "5"
        os.environ["INTERVAL_MINUTES"] = "30"

        import dashboard

        cls.dashboard = importlib.reload(dashboard)
        cls.dashboard.init_db()

        cls.server = cls.dashboard.ThreadingHTTPServer(
            ("127.0.0.1", 0), cls.dashboard.DashboardHandler
        )
        cls.port = cls.server.server_address[1]
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.shutdown()
        cls.server.server_close()
        cls.tmpdir.cleanup()

    def setUp(self) -> None:
        with self.dashboard.DB_LOCK:
            with self.dashboard.db_connect() as conn:
                conn.execute("DELETE FROM videos")
                conn.execute("DELETE FROM settings")
                conn.execute(
                    "INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)",
                    ("interval_minutes", "30"),
                )
                conn.execute(
                    "INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)",
                    ("last_checked", ""),
                )

    def request_json(self, method: str, path: str, body: Optional[dict] = None):
        conn = http.client.HTTPConnection("127.0.0.1", self.port, timeout=10)
        headers = {}
        data = None
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        conn.request(method, path, body=data, headers=headers)
        resp = conn.getresponse()
        raw = resp.read().decode("utf-8")
        conn.close()
        payload = json.loads(raw) if raw else None
        return resp.status, payload, dict(resp.getheaders())

    def request_raw(self, method: str, path: str):
        conn = http.client.HTTPConnection("127.0.0.1", self.port, timeout=10)
        conn.request(method, path)
        resp = conn.getresponse()
        body = resp.read()
        headers = dict(resp.getheaders())
        conn.close()
        return resp.status, headers, body

    def seed_items(self):
        status, payload, _ = self.request_json("POST", "/api/refresh")
        self.assertEqual(status, 200)
        self.assertEqual(payload["status"], "ok")

    def test_refresh_and_items(self):
        self.seed_items()
        status, payload, _ = self.request_json("GET", "/api/items")
        self.assertEqual(status, 200)
        self.assertEqual(len(payload["items"]), 2)
        for item in payload["items"]:
            self.assertEqual(len(item.get("rapidgator_urls", [])), 2)
        post = next(item for item in payload["items"] if item["title"] == "Sample One")
        self.assertEqual(
            post["images"],
            [
                {
                    "url": "https://imgpv.com/example/image1.jpg.html",
                    "thumbnail_url": "https://example.com/preview1.jpg",
                },
                {
                    "url": "https://example.com/image2.png",
                    "thumbnail_url": "https://example.com/preview2.jpg",
                },
            ],
        )

        status, payload, _ = self.request_json("GET", "/api/all")
        self.assertEqual(status, 200)
        self.assertEqual(len(payload["items"]), 2)

    def test_legacy_media_retries_and_caches_without_changing_viewed_state(self):
        self.seed_items()
        with self.dashboard.db_connect() as conn:
            target = conn.execute("SELECT id FROM videos WHERE title = 'Sample One'").fetchone()["id"]
            saved_url = "https://rapidgator.net/file/aaaa/one.mp4.html"
            conn.execute(
                """
                UPDATE videos SET images = NULL, rapidgator_urls = ?, viewed_at = 'original'
                WHERE id = ?
                """,
                (json.dumps([saved_url, "https://rapidgator.net/article/premium/ref/123"]), target),
            )

        with patch.object(self.dashboard, "fetch_html", side_effect=OSError):
            status, media, _ = self.request_json("GET", f"/api/media/{target}")
            redirect_status, headers, _ = self.request_raw("GET", f"/image/{target}")
        self.assertEqual(status, 200)
        self.assertIsNone(media["images"])
        self.assertEqual(media["rapidgator_urls"], [saved_url])
        self.assertEqual(redirect_status, 302)
        self.assertEqual(headers["Location"], "https://example.com/cover1.jpg")

        status, headers, _ = self.request_raw("GET", f"/image/{target}")
        self.assertEqual(status, 302)
        self.assertEqual(headers["Location"], "https://imgpv.com/example/image1.jpg.html")

        with patch.object(self.dashboard, "fetch_html") as fetch:
            _, cached, _ = self.request_json("GET", f"/api/media/{target}")
            fetch.assert_not_called()
        self.assertEqual(len(cached["images"]), 2)
        self.assertEqual(len(cached["rapidgator_urls"]), 2)
        _, payload, _ = self.request_json("GET", "/api/all")
        item = next(item for item in payload["items"] if item["id"] == target)
        self.assertEqual(item["viewed_at"], "original")
        self.assertIsNone(item["downloaded_at"])

    def test_view_and_view_all(self):
        self.seed_items()
        status, payload, _ = self.request_json("GET", "/api/items")
        first_id = payload["items"][0]["id"]

        status, _, _ = self.request_json("POST", "/api/view", {"id": first_id})
        self.assertEqual(status, 200)

        status, payload, _ = self.request_json("GET", "/api/items")
        self.assertEqual(len(payload["items"]), 1)

        status, payload, _ = self.request_json("POST", "/api/view-all")
        self.assertEqual(status, 200)
        self.assertGreaterEqual(payload["count"], 1)

        status, payload, _ = self.request_json("GET", "/api/items")
        self.assertEqual(len(payload["items"]), 0)

    def test_download_endpoints(self):
        self.seed_items()
        status, payload, _ = self.request_json("GET", "/api/items")
        item_ids = [item["id"] for item in payload["items"]]
        first_id, second_id = item_ids

        status, payload, _ = self.request_json("POST", "/api/download", {"id": first_id})
        self.assertEqual(status, 200)
        self.assertEqual(len(payload["urls"]), 2)

        status, headers, _ = self.request_raw("GET", f"/download/{second_id}")
        self.assertEqual(status, 302)
        self.assertIn("Location", headers)

        _, payload, _ = self.request_json("GET", "/api/items")
        self.assertEqual([item["id"] for item in payload["items"]], item_ids)
        self.assertTrue(all(item["downloaded_at"] for item in payload["items"]))
        _, payload, _ = self.request_json("GET", f"/api/media/{second_id}")
        self.assertTrue(payload["downloaded_at"])

        _, payload, _ = self.request_json("GET", "/api/summary")
        self.assertEqual((payload["viewed"], payload["unseen"], payload["downloaded"]), (0, 2, 2))

        self.request_json("POST", "/api/view", {"id": first_id})
        _, payload, _ = self.request_json("GET", "/api/items")
        self.assertEqual([item["id"] for item in payload["items"]], [second_id])

        _, payload, _ = self.request_json("POST", "/api/view-all")
        self.assertEqual(payload["count"], 1)
        _, payload, _ = self.request_json("GET", "/api/items")
        self.assertEqual(payload["items"], [])

    def test_settings_stats_summary(self):
        self.seed_items()
        status, payload, _ = self.request_json("GET", "/api/settings")
        self.assertEqual(status, 200)
        self.assertEqual(payload["start_date"], self.today_iso)

        status, payload, _ = self.request_json("POST", "/api/settings", {"interval_minutes": 60})
        self.assertEqual(status, 200)
        self.assertEqual(payload["interval_minutes"], "60")

        status, payload, _ = self.request_json("GET", "/api/stats")
        self.assertEqual(status, 200)
        self.assertTrue(len(payload["daily"]) >= 1)

        status, payload, _ = self.request_json("GET", "/api/summary")
        self.assertEqual(status, 200)
        self.assertEqual(payload["total"], 2)
        self.assertEqual(payload["unseen"], 2)


if __name__ == "__main__":
    unittest.main()
