"""rarfile, with the one tool that the images have (bsdtar) made to work.

Python's `rarfile` builds the bsdtar command line as `bsdtar -x --to-stdout -f -- ARCHIVE MEMBER`: the "--" lands right after "-f", so bsdtar takes
"--" as the name of the archive and fails ("Failed to open '--'"), and every member that is not stored (rarfile reads those itself) comes out as
`BadRarFile: Failed the read enough data`. The command line is built here instead: the archive after "-f", then "--" and the member.

The member is given as a pattern to bsdtar, so its glob characters are escaped: "Page [01].jpg" is not the pattern for "Page 1.jpg".
Solid archives and the RARVM filters are not read by libarchive; those still fail, with the tool's own message.
"""
import re

import rarfile

_GLOB = re.compile(r'([\\*?\[\]])')
_patched = False


def glob_escape(name):
    return _GLOB.sub(r'\\\1', name)


def load():
    """The rarfile module, with the command line of bsdtar fixed (once)."""
    global _patched
    if _patched:
        return rarfile
    original = rarfile.ToolSetup.open_cmdline

    def open_cmdline(self, pwd, rarfn, filefn=None):
        if self.setup is not rarfile.BSDTAR_CONFIG:
            return original(self, pwd, rarfn, filefn)
        cmdline = [rarfile.BSDTAR_TOOL, '-x', '--to-stdout', '-f', rarfn]
        if filefn:
            cmdline += ['--', glob_escape(filefn)]
        return cmdline

    rarfile.ToolSetup.open_cmdline = open_cmdline
    _patched = True
    return rarfile
