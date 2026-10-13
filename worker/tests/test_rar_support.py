"""rarfile with bsdtar: the command line that works, and the member given as a pattern."""
import rarfile
import pytest

import rar_support


@pytest.fixture(autouse=True)
def patched():
    rar_support.load()


class TestTheBsdtarCommandLine:
    def cmd(self, archive='/storage/c.cbr', member='01.jpg'):
        return rarfile.ToolSetup(rarfile.BSDTAR_CONFIG).open_cmdline(None, archive, member)

    def test_the_archive_follows_f_so_that_the_double_dash_is_not_taken_as_its_name(self):
        cmd = self.cmd()
        assert cmd[1:] == ['-x', '--to-stdout', '-f', '/storage/c.cbr', '--', '01.jpg']
        assert cmd[cmd.index('-f') + 1] != '--'

    def test_a_member_with_pattern_characters_is_given_as_the_literal_name(self):
        assert self.cmd(member='Page [01] *.jpg')[-1] == r'Page \[01\] \*.jpg'
        assert self.cmd(member='a?b\\c.jpg')[-1] == r'a\?b\\c.jpg'

    def test_the_other_tools_are_left_as_rarfile_makes_them(self):
        unrar = rarfile.ToolSetup(rarfile.UNRAR_CONFIG).open_cmdline(None, '/storage/c.cbr', '01.jpg')
        assert unrar[-2:] == ['/storage/c.cbr', '01.jpg']
        assert '-f' not in unrar

    def test_it_is_done_once(self):
        first = rarfile.ToolSetup.open_cmdline
        assert rar_support.load() is rarfile
        assert rarfile.ToolSetup.open_cmdline is first
