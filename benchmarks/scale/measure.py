#!/usr/bin/env python3
"""Measure how a Códice instance answers with a large library.

    python3 benchmarks/scale/measure.py --base http://127.0.0.1:18186 --token-file TOKEN \
        [--manifest DIR/manifest.jsonl] [--label 1k] [--json out.json]

Standard library only. Reads what the screens read (the list, the search, the sidebar counts, the work
sheet, the OPDS feeds, the covers, the administration lists) and times each: one request at a time
(what one person feels) and then many at once (what a family feels). It only reads, except for the
sign-in, which it times a few times (the login limit is 10 a minute per client, so it stays under).

The manifest is the one written by testdata/generate_scale.py; it tells which marker word to search
for (a word that is in exactly one work) and which titles exist. WITHOUT a manifest (a real library) the
search by a word in the title uses a word of a title the library itself returns, the search for a marker
is left out, and nothing printed names a work: the output is only times, sizes and counts, so it can be
shared.
"""
import argparse
import json
import statistics
import time
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor


def pct(values, p):
    values = sorted(values)
    if not values:
        return float("nan")
    k = min(len(values) - 1, max(0, round(p / 100 * (len(values) - 1))))
    return values[k]


class Client:
    def __init__(self, base, token):
        self.base, self.token = base.rstrip("/"), token

    def get(self, path, headers=None):
        req = urllib.request.Request(self.base + path, headers={"Authorization": f"Bearer {self.token}", **(headers or {})})
        t = time.perf_counter()
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                body = r.read()
                status = r.status
        except urllib.error.HTTPError as e:
            body, status = e.read(), e.code
        return time.perf_counter() - t, status, body


def timed_series(client, path, n=30):
    client.get(path)  # warm-up (plans, caches): the first one is not what a person feels every day
    times, size, status = [], 0, 0
    for _ in range(n):
        dt, status, body = client.get(path)
        times.append(dt * 1000)
        size = len(body)
    return {"p50": statistics.median(times), "p95": pct(times, 95), "max": max(times), "bytes": size, "status": status}


def concurrent(client, paths, total=300, workers=20):
    jobs = [paths[i % len(paths)] for i in range(total)]
    t0 = time.perf_counter()
    with ThreadPoolExecutor(workers) as ex:
        results = list(ex.map(lambda p: client.get(p), jobs))
    wall = time.perf_counter() - t0
    times = [r[0] * 1000 for r in results]
    errors = sum(1 for r in results if r[1] >= 400)
    return {"rps": total / wall, "p50": statistics.median(times), "p95": pct(times, 95), "max": max(times), "errors": errors}


def login_times(base, user, password, n=6):
    out = []
    for _ in range(n):
        req = urllib.request.Request(
            base + "/auth/login", data=json.dumps({"username": user, "password": password}).encode(),
            headers={"Content-Type": "application/json"},
        )
        t = time.perf_counter()
        with urllib.request.urlopen(req, timeout=30) as r:
            r.read()
        out.append((time.perf_counter() - t) * 1000)
    return {"p50": statistics.median(out), "max": max(out)}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", required=True)
    ap.add_argument("--token-file", required=True)
    ap.add_argument("--manifest")
    ap.add_argument("--label", default="")
    ap.add_argument("--json")
    ap.add_argument("--login-user")
    ap.add_argument("--login-password")
    a = ap.parse_args()

    token = open(a.token_file).read().strip()
    c = Client(a.base, token)
    _, _, body = c.get("/works?limit=50&page=1")
    first = json.loads(body)
    total, pages = first["total"], first["totalPages"]
    if a.manifest:
        manifest = [json.loads(l) for l in open(a.manifest, encoding="utf-8")]
        texty = [m for m in manifest if m["format"] in ("epub", "txt", "pdf")]
        marker = texty[len(texty) // 3]["marker"]
        word = manifest[len(manifest) // 2]["title"].split()[0].lower()
    else:
        # A real library: a word that one of its own titles has (the first long one, from a title in the middle of the page).
        marker = None
        titles = [w["title"] for w in first["data"]]
        words = [x.lower() for t in titles[len(titles) // 2:] + titles for x in t.split() if len(x) >= 4 and x.isalpha()]
        word = words[0] if words else "livro"
    cover = next((w["coverUrl"] for w in first["data"] if w.get("coverUrl")), None)
    work_id = first["data"][0]["id"]
    comics_page = "/works?limit=50&formatGroup=comics"

    cases = [
        ("list: first page (50)", "/works?limit=50&page=1"),
        ("list: middle page", f"/works?limit=50&page={pages // 2}"),
        ("list: last page", f"/works?limit=50&page={pages}"),
        ("list: only comics", comics_page),
        ("list: filter by title word", f"/works?limit=50&search={urllib.parse.quote(word)}"),
        ("sidebar counts (/stats)", "/stats"),
        ("favorites", "/favorites"),
        ("work sheet", f"/works/{work_id}"),
        ("search: title word (many hits)", f"/search?q={urllib.parse.quote(word)}"),
        ("search: content, common word", "/search?q=silêncio".replace("ê", "%C3%AA")),
        ("search: no hit", "/search?q=xyzxyzxyz"),
        ("OPDS: recent (50)", "/opds/v1.2/recent"),
        ("OPDS: search", f"/opds/v1.2/search?q={urllib.parse.quote(word)}"),
        ("admin: jobs", "/admin/jobs"),
        ("admin: duplicates", "/admin/duplicates"),
        ("admin: suggestions queue", "/admin/suggestions"),
        ("admin: people to merge", "/admin/people/merges"),
        ("notes", "/notes?limit=50"),
    ]
    if marker:
        cases.insert(10, ("search: content, one-work marker", f"/search?q={marker}"))
    if cover:
        cases.append(("cover (one image)", cover))

    result = {"label": a.label, "works": total, "sequential": {}, "concurrent": {}}
    print(f"\n## {a.label or 'measure'}: {total} works\n")
    print("| Request | p50 ms | p95 ms | max ms | bytes | status |")
    print("|---|---:|---:|---:|---:|---:|")
    for name, path in cases:
        r = timed_series(c, path)
        result["sequential"][name] = r
        print(f"| {name} | {r['p50']:.0f} | {r['p95']:.0f} | {r['max']:.0f} | {r['bytes']} | {r['status']} |")

    print("\n20 at once, 300 requests of a mix:\n")
    print("| Mix | req/s | p50 ms | p95 ms | max ms | errors |")
    print("|---|---:|---:|---:|---:|---:|")
    mixes = {
        "the list (pages 1, middle, last)": ["/works?limit=50&page=1", f"/works?limit=50&page={pages // 2}", f"/works?limit=50&page={pages}"],
        "the search (title, common word, no hit)": [cases[8][1], cases[9][1], dict(cases)["search: no hit"]],
        "what opening the app asks": ["/works?limit=50&page=1", "/stats", "/favorites", "/auth/me"],
    }
    if cover:
        mixes["covers"] = [cover]
    for name, paths in mixes.items():
        r = concurrent(c, paths)
        result["concurrent"][name] = r
        print(f"| {name} | {r['rps']:.0f} | {r['p50']:.0f} | {r['p95']:.0f} | {r['max']:.0f} | {r['errors']} |")

    if a.login_user:
        r = login_times(a.base, a.login_user, a.login_password)
        result["login"] = r
        print(f"\nSign-in (6 times): p50 {r['p50']:.0f} ms, max {r['max']:.0f} ms")

    if a.json:
        with open(a.json, "w") as f:
            json.dump(result, f, indent=1)


if __name__ == "__main__":
    main()
