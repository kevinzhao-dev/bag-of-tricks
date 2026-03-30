import http.client
import importlib
import json
import os
import tempfile
import threading
import unittest
from datetime import date
from pathlib import Path
from typing import Optional


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
                <a href="https://rapidgator.net/file/aaaa/one.mp4.html">rg1</a>
                <a href="https://rapidgator.net/file/bbbb/one-gift.mp4.html">rg2</a>
              </div>
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

        status, payload, _ = self.request_json("GET", "/api/all")
        self.assertEqual(status, 200)
        self.assertEqual(len(payload["items"]), 2)

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
        target_id = payload["items"][0]["id"]

        status, payload, _ = self.request_json("POST", "/api/download", {"id": target_id})
        self.assertEqual(status, 200)
        self.assertEqual(len(payload["urls"]), 2)

        status, headers, _ = self.request_raw("GET", f"/download/{target_id}")
        self.assertEqual(status, 302)
        self.assertIn("Location", headers)

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
