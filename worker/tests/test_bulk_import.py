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
        if "INSERT INTO editions" in self._last:
            return (7,)
        if "INSERT INTO files" in self._last:
            return (9,)
        return None

    def writes(self):
        """The statements that change data, as verb and table."""
        return [" ".join(q.split(" ")[:3]) if q.startswith("INSERT") else " ".join(q.split(" ")[:2])
                for q, _ in self.statements if not q.startswith("SELECT")]


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
        # Work, edition, file with its hash, location and job are all written on the
        # caller's transaction; the caller commits.
        assert cursor.writes() == ["INSERT INTO works", "INSERT INTO editions", "INSERT INTO files",
                                   "INSERT INTO storage_locations", "INSERT INTO jobs", "UPDATE works"]
        file_insert = next(p for q, p in cursor.statements if q.startswith("INSERT INTO files"))
        assert file_insert == (7, 'epub', sha256_of(src), len(b"some book bytes"))
        location = next(p for q, p in cursor.statements if q.startswith("INSERT INTO storage_locations"))
        assert location == (9, "1_1_Duna.epub")
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
