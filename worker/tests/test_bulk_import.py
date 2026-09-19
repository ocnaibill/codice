"""Tests for the bulk import CLI's per-file step."""
import json
import os

from bulk_import import enqueue_file, sha256_of


class FakeCursor:
    """Answers the two reads the function makes and records its writes."""

    def __init__(self, existing_work=None, next_work_id=42):
        self.existing_work = existing_work
        self.next_work_id = next_work_id
        self.statements = []
        self._last = ""

    def execute(self, query, params=()):
        self._last = " ".join(query.split())
        self.statements.append((self._last, params))

    def fetchone(self):
        if "FROM files f JOIN editions" in self._last:
            return (self.existing_work,) if self.existing_work else None
        if "INSERT INTO works" in self._last:
            return (self.next_work_id,)
        return None

    def writes(self):
        return [q.split(" ")[0] + " " + q.split(" ")[1] for q, _ in self.statements if not q.startswith("SELECT")]


def source_file(tmp_path, content=b"some book bytes"):
    src = tmp_path / "Duna.epub"
    src.write_bytes(content)
    return str(src)


class TestEnqueueFile:
    def test_a_new_file_is_copied_hashed_and_queued_together(self, tmp_path):
        src = source_file(tmp_path)
        dst = str(tmp_path / "storage" / "1_1_Duna.epub")
        os.makedirs(os.path.dirname(dst))
        cursor = FakeCursor()

        outcome, work_id = enqueue_file(cursor, src, dst)

        assert (outcome, work_id) == ('enqueued', 42)
        assert open(dst, 'rb').read() == b"some book bytes"
        # Work, hash and job are all written on the caller's transaction; the caller commits.
        assert cursor.writes() == ["INSERT INTO", "UPDATE files", "INSERT INTO", "UPDATE works"]
        hash_update = next(p for q, p in cursor.statements if q.startswith("UPDATE files"))
        assert hash_update == (sha256_of(src), len(b"some book bytes"), 42)
        job_insert = next((q, p) for q, p in cursor.statements if q.startswith("INSERT INTO jobs"))
        assert "ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING" in job_insert[0]
        assert json.loads(job_insert[1][1]) == {"file_path": os.path.abspath(dst)}
        assert "priority" in job_insert[0] and job_insert[0].rstrip().endswith("DO NOTHING")

    def test_identical_bytes_are_never_stored_twice(self, tmp_path):
        src = source_file(tmp_path)
        dst = str(tmp_path / "copy.epub")
        cursor = FakeCursor(existing_work=7)

        assert enqueue_file(cursor, src, dst) == ('duplicate', 7)
        assert not os.path.exists(dst)  # nothing copied
        assert cursor.writes() == []    # nothing written

    def test_the_hash_depends_on_the_bytes_not_the_name(self, tmp_path):
        a = tmp_path / "a.epub"
        b = tmp_path / "b.epub"
        a.write_bytes(b"same")
        b.write_bytes(b"same")
        assert sha256_of(str(a)) == sha256_of(str(b))
        b.write_bytes(b"different")
        assert sha256_of(str(a)) != sha256_of(str(b))
