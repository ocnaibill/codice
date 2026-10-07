#!/usr/bin/env python3
"""Upload the files of testdata/generate_hostile.py to a Codice and say what happened to each one.

ONLY for a disposable stack: it uploads dozens of files (some on purpose broken) into the library it points at.

    worker/venv/bin/python testdata/generate_hostile.py --out DIR
    python3 testdata/run_hostile.py --dir DIR --base http://127.0.0.1:8080 --token-file TOKEN \\
        --psql "docker exec -i codice-e2e-97-postgres-1 psql -U codice_user -d codice_db -At -F '|'"

The token is a session token of an admin or the owner (the `token` that POST /auth/login or /auth/setup answers).
The verdict compares what happened with `expect` of manifest.json (see generate_hostile.py):
  PASS    what a healthy Codice does;
  REVIEW  not wrong for sure, but worth a look (a legitimate file whose text was not read, a broken one that was read anyway);
  FAIL    a 5xx, a legitimate file refused, a broken one accepted when it should be refused, or a job that never ends.
Exit code 1 when any FAIL.
"""
import argparse
import json
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path


def upload(base, token, path, name):
    boundary = uuid.uuid4().hex
    body = (
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"document\"; filename=\"{name.replace(chr(34), '%22')}\"\r\n"
        "Content-Type: application/octet-stream\r\n\r\n"
    ).encode("utf-8", "surrogatepass") + path.read_bytes() + f"\r\n--{boundary}--\r\n".encode()
    req = urllib.request.Request(
        base + "/upload", data=body, method="POST",
        headers={"Authorization": f"Bearer {token}", "Content-Type": f"multipart/form-data; boundary={boundary}"},
    )
    try:
        with urllib.request.urlopen(req, timeout=120) as r:
            return r.status, r.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")
    except Exception as e:  # the server went away
        return 0, f"{type(e).__name__}: {e}"


def sql(psql, query):
    out = subprocess.run(psql, shell=True, input=query, capture_output=True, text=True, timeout=60)
    if out.returncode != 0:
        raise SystemExit(f"psql failed: {out.stderr.strip()}")
    return out.stdout.strip()


def wait_idle(psql, timeout):
    end = time.time() + timeout
    while time.time() < end:
        if sql(psql, "SELECT count(*) FROM jobs WHERE state IN ('pending','running');") == "0":
            return True
        time.sleep(3)
    return False


def state_of(psql, work_id):
    row = sql(psql, f"""
        SELECT coalesce(te.status, '-'), coalesce(replace(left(te.error, 90), '|', '/'), ''), te.segment_count,
               coalesce((SELECT string_agg(j.type || ':' || j.state || coalesce('(' || replace(left(j.last_error, 70), '|', '/') || ')', ''), ' ' ORDER BY j.id)
                         FROM jobs j WHERE j.work_id = {work_id}), ''),
               coalesce((SELECT w.original_title FROM works w WHERE w.id = {work_id}), '')
        FROM works w LEFT JOIN editions e ON e.work_id = w.id LEFT JOIN files f ON f.edition_id = e.id
        LEFT JOIN text_extractions te ON te.file_id = f.id WHERE w.id = {work_id} LIMIT 1;""")
    parts = row.split("|") if row else []
    parts += [""] * (5 - len(parts))
    return {"text": parts[0], "text_error": parts[1], "segments": parts[2], "jobs": parts[3], "title": parts[4][:40]}


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--dir", type=Path, required=True)
    ap.add_argument("--base", required=True)
    ap.add_argument("--token-file", type=Path, required=True)
    ap.add_argument("--psql", required=True, help="a command that runs SQL from stdin and prints unaligned rows with | between columns")
    ap.add_argument("--wait", type=int, default=900, help="seconds to wait for the queue to empty")
    ap.add_argument("--json", type=Path, help="also write the result here")
    args = ap.parse_args()

    token = args.token_file.read_text().strip()
    if token.startswith("{"):
        token = json.loads(token)["token"]
    files = json.loads((args.dir / "manifest.json").read_text())["files"]
    results = []
    for f in files:
        code, msg = upload(args.base, token, args.dir / f["file"], f.get("upload_as") or f["file"])
        r = {**f, "http": code, "answer": " ".join(msg.split())[:110]}
        if code == 200:
            try:
                r["work_id"] = json.loads(msg)["work_id"]
            except Exception:
                pass
        results.append(r)
        print(f"{code:>3}  {f['file']}", flush=True)

    idle = wait_idle(args.psql, args.wait)
    for r in results:
        if "work_id" in r:
            r.update(state_of(args.psql, r["work_id"]))
        verdict, why = "PASS", ""
        accepted = r["http"] == 200
        if r["http"] == 0 or r["http"] >= 500:
            verdict, why = "FAIL", "the server failed or went away"
        elif r["expect"] == "refuse" and accepted:
            verdict, why = "FAIL", "accepted a file that is not what its name says"
        elif r["expect"] == "ok" and not accepted:
            verdict, why = "FAIL", "refused a legitimate file"
        elif accepted and not idle and ("running" in r.get("jobs", "") or "pending" in r.get("jobs", "")):
            verdict, why = "FAIL", "a job never ended"
        elif r["expect"] == "ok" and accepted and r.get("text") not in ("ready", "unsupported"):
            verdict, why = "REVIEW", f"legitimate file, text {r.get('text')}"
        elif r["expect"] == "clean" and accepted and r.get("text") == "ready":
            verdict, why = "REVIEW", "broken file, but its text was read"
        elif r["expect"] == "clean" and accepted and r.get("text") in ("failed", "empty") and not r.get("text_error") and r.get("text") == "failed":
            verdict, why = "REVIEW", "failed without saying why"
        r["verdict"], r["why"] = verdict, why

    print("\n" + "=" * 100)
    for r in results:
        detail = r["answer"] if r["http"] != 200 else f"work {r.get('work_id')} text={r.get('text')} {r.get('text_error', '')} | {r.get('jobs', '')}"
        print(f"{r['verdict']:<6} {r['http']:>3} {r['file'][:42]:<42} {detail[:150]}" + (f"  <- {r['why']}" if r["why"] else ""))
    counts = {v: sum(1 for r in results if r["verdict"] == v) for v in ("PASS", "REVIEW", "FAIL")}
    print(f"\n{counts}  (queue {'empty' if idle else 'NOT empty'})")
    if args.json:
        args.json.write_text(json.dumps(results, ensure_ascii=False, indent=2))
    sys.exit(1 if counts["FAIL"] else 0)


if __name__ == "__main__":
    main()
