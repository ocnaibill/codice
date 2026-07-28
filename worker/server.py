"""HTTP server exposing metadata search for the frontend.
 
Provides a REST endpoint that the backend Go can proxy to for
manual metadata search from the EditBookModal.
 
Uses stdlib only (http.server, json).
"""
import json
import os
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs

# Ensure worker package is importable
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from providers import ProviderRegistry
from dotenv import load_dotenv

load_dotenv("../.env")


class SearchHandler(BaseHTTPRequestHandler):
    """Handle GET /search?q=...&format=... requests."""

    registry = ProviderRegistry()

    def do_GET(self):
        parsed = urlparse(self.path)

        if parsed.path == "/health":
            self._json_response({"status": "ok"})
            return

        if parsed.path != "/search":
            self._json_response({"error": "Not found"}, 404)
            return

        params = parse_qs(parsed.query)
        query = params.get("q", [None])[0]
        fmt = params.get("format", [""])[0] or None

        if not query:
            self._json_response({"error": "Missing 'q' parameter"}, 400)
            return

        results = self.registry.search_all(query, fmt)

        # Serialize results
        serialized = []
        for r in results:
            serialized.append({
                "source": r.source,
                "title": r.title,
                "author": r.author,
                "series": r.series,
                "series_index": r.series_index,
                "isbn": r.isbn,
                "language": r.language,
                "publisher": r.publisher,
                "publication_date": r.publication_date,
                "description": r.description,
                "tags": r.tags,
                "cover_url": r.cover_url,
            })

        self._json_response({"results": serialized, "query": query})

    def _json_response(self, data, status=200):
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        self.wfile.write(json.dumps(data, ensure_ascii=False).encode("utf-8"))

    def log_message(self, format, *args):
        print(f"   🌐 Worker HTTP: {args[0]} {args[1]} {args[2]}")


def run_server(host="0.0.0.0", port=5000):
    print(f"   🌐 Worker search server starting on {host}:{port}")
    server = HTTPServer((host, port), SearchHandler)
    print(f"   ✅ Worker search server ready on http://{host}:{port}/search")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n   👋 Worker search server shutting down")
        server.server_close()


if __name__ == "__main__":
    run_server()