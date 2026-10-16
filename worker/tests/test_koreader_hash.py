"""The hash KOReader knows a book by, as the device computes it."""
import hashlib

from koreader_hash import fill, partial_md5


def test_it_is_the_md5_of_the_samples_a_device_takes(tmp_path):
    data = bytes((i * 7 + i // 251) % 256 for i in range(70_000))
    path = tmp_path / 'book.epub'
    path.write_bytes(data)
    # the offsets the device reads at: the start, then 1 KiB, 4 KiB, 16 KiB and 64 KiB, and nothing past the end
    want = b''.join(data[o:o + 1024] for o in (0, 1024, 4096, 16384, 65536))
    assert partial_md5(str(path)) == hashlib.md5(want).hexdigest()


def test_a_file_smaller_than_a_sample_is_hashed_whole_and_an_empty_one_is_the_md5_of_nothing(tmp_path):
    (tmp_path / 'tiny').write_bytes(b'abc')
    assert partial_md5(str(tmp_path / 'tiny')) == hashlib.md5(b'abc').hexdigest()
    (tmp_path / 'empty').write_bytes(b'')
    assert partial_md5(str(tmp_path / 'empty')) == hashlib.md5(b'').hexdigest()
    assert partial_md5(str(tmp_path / 'missing')) is None


def test_it_matches_the_hash_a_kindle_sent_for_a_real_book():
    """836c0286607797847360cb16cbe688e30 is what a Kindle 10th generation sent for this EPUB (measured on 16/10/2026)."""
    import os
    path = os.path.expanduser('~/Downloads/As foices Contos do Universo de Sycthe.epub')
    if not os.path.exists(path):
        import pytest
        pytest.skip('the book is not on this machine')
    assert partial_md5(path) == '836c028607797847360cb16cbe688e30'


class FakeDb:
    def __init__(self, rows):
        self.rows, self.updated = rows, []

    def fetchall(self, query, params=None):
        return self.rows

    def execute(self, query, params=None):
        self.updated.append(params)


def test_the_worker_hashes_the_files_that_have_none_and_skips_what_is_not_there(tmp_path):
    (tmp_path / 'a.epub').write_bytes(b'x' * 3000)
    db = FakeDb([(1, 'managed', None, 'a.epub'), (2, 'managed', None, 'gone.epub'), (3, 'referenced', str(tmp_path), 'a.epub')])
    assert fill(db, str(tmp_path), log=lambda *_: None) == 2
    assert [u[1] for u in db.updated] == [1, 3]
    # 3000 bytes: the samples at 0 and at 1024, and the one at 4096 is past the end
    assert db.updated[0][0] == hashlib.md5(b'x' * 2048).hexdigest()
