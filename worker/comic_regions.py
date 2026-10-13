"""What is made of the lines an OCR engine finds in a comic page (DEC-176): balloons, the ones that are speech, and the order they are read in.

Pure functions, no engine and no image: the lines come as boxes with their text and confidence, so all of it is tested without a model.
It is the part of the laboratory (lab/ocr) that measured well, ported:

  * lines that are close are one balloon (a caption, a speech), and the lines of two balloons that touch are far enough to be two;
  * what is not speech is left out: letters too big are art or an onomatopoeia, an area too big is words of the art that were joined, a
    region with no word of three letters or one weak word is noise;
  * a word broken at the end of a line is joined ("CONCEN-" "TRAR"), which is what a search for the whole word needs;
  * the order of the balloons is a view, computed from a direction, and is never kept: the regions are kept without order.
"""
import re

PAD = 0.3              # how close the lines of a balloon are, in heights of a letter; the lines of two balloons are farther
MIN_CONFIDENCE = 60    # 0 to 100: below this a region is not speech
ONE_WORD_CONFIDENCE = 80
SFX_LETTER = 0.08      # letters taller than this part of the width of the page are art or onomatopoeia, not speech
MAX_AREA = 0.30        # a region over this part of the page is not a balloon
MIN_LETTER_WORD = 3    # a region without a word of this many letters is noise
ROW_TOLERANCE = 0.04   # balloons whose tops are within this part of the height of the page are one row

_LETTERS = re.compile(r'[^\W\d_]')


def letters(text):
    return len(_LETTERS.findall(text))


def join_text(words):
    """The text of a region from its words, in order; a word broken at the end of a line is joined, and a hyphen inside a word is kept."""
    out = []
    for word in words:
        text = word['text'] if isinstance(word, dict) else word
        if out and out[-1].endswith('-') and len(out[-1]) > 2 and text[:1].isalpha():
            out[-1] = out[-1][:-1] + text
        else:
            out.append(text)
    return ' '.join(out)


def _grow(word, pad):
    p = max(4, int(word['h'] * pad))
    return word['x'] - p, word['y'] - p, word['x'] + word['w'] + p, word['y'] + word['h'] + p


def cluster(lines, pad=PAD):
    """Joins the lines that are close into regions: [{'box': [x, y, w, h], 'text', 'conf', 'words', 'letter'}].

    Every line is `{'x', 'y', 'w', 'h', 'text', 'conf'}` in the pixels of the page, and its position in the list is the order the engine
    found it in, which is the order of the text of a region."""
    lines = [dict(l, n=i) for i, l in enumerate(lines) if (l.get('text') or '').strip()]
    parent = list(range(len(lines)))

    def find(i):
        while parent[i] != i:
            parent[i] = parent[parent[i]]
            i = parent[i]
        return i

    boxes = [_grow(l, pad) for l in lines]
    for i in range(len(lines)):
        for j in range(i + 1, len(lines)):
            a, b = boxes[i], boxes[j]
            if a[0] < b[2] and b[0] < a[2] and a[1] < b[3] and b[1] < a[3]:
                parent[find(i)] = find(j)
    groups = {}
    for i, line in enumerate(lines):
        groups.setdefault(find(i), []).append(line)
    regions = []
    for group in groups.values():
        group.sort(key=lambda l: l['n'])
        x0, y0 = min(l['x'] for l in group), min(l['y'] for l in group)
        x1, y1 = max(l['x'] + l['w'] for l in group), max(l['y'] + l['h'] for l in group)
        heights = sorted(l['h'] for l in group)
        regions.append({'box': [x0, y0, x1 - x0, y1 - y0], 'text': join_text(group),
                        'conf': round(sum(l['conf'] for l in group) / len(group), 1), 'words': len(group),
                        'letter': heights[len(heights) // 2]})
    return regions


def judge(region, page_width, page_height):
    """(kept, why not): whether a region is speech worth keeping."""
    text = region['text']
    if region['conf'] < MIN_CONFIDENCE:
        return False, f"confiança {region['conf']}"
    longest = max((letters(t) for t in text.split()), default=0)
    if longest < MIN_LETTER_WORD:   # also what has no letters at all
        return False, 'nenhuma palavra de três letras'
    if region['words'] == 1 and region['conf'] < ONE_WORD_CONFIDENCE:
        return False, 'uma palavra fraca'
    _x, _y, w, h = region['box']
    if page_width and page_height and w * h / (page_width * page_height) > MAX_AREA:
        return False, 'grande demais (palavras da arte juntas)'
    if page_width and region['letter'] / page_width > SFX_LETTER:
        return False, 'letra grande demais (arte ou onomatopeia)'
    return True, ''


def speech(lines, page_width, page_height):
    """The regions of speech of a page, from the lines the engine found, without order."""
    kept = []
    for region in cluster(lines):
        ok, _why = judge(region, page_width, page_height)
        if ok:
            kept.append({'box': region['box'], 'text': region['text'], 'conf': region['conf']})
    return kept


def reading_order(regions, direction, page_height):
    """The indexes of the regions in the order they are read. A strip is read from the top down; a page by rows, from the top, and in a row
    from the left, or from the right when the direction is 'rtl'. Regions whose tops are close are one row (a fixed band would split two
    balloons that sit a few pixels apart on either side of its edge). The regions themselves are not changed."""
    if direction == 'webtoon':
        return sorted(range(len(regions)), key=lambda i: (regions[i]['box'][1], regions[i]['box'][0]))
    tolerance = max(1, int((page_height or 0) * ROW_TOLERANCE))
    by_top = sorted(range(len(regions)), key=lambda i: (regions[i]['box'][1], regions[i]['box'][0]))
    rows, first = [], None
    for i in by_top:
        top = regions[i]['box'][1]
        if first is None or top - first > tolerance:
            rows.append([])
            first = top
        rows[-1].append(i)
    sign = -1 if direction == 'rtl' else 1
    return [i for row in rows for i in sorted(row, key=lambda i: sign * regions[i]['box'][0])]


def locator_region(box, page_width, page_height):
    """The box of a region in the unit square of the page, as the locator of an image says it (0 to 1)."""
    x, y, w, h = box
    pw, ph = max(1, page_width), max(1, page_height)
    clip = lambda v: max(0.0, min(1.0, v))
    x0, y0 = clip(x / pw), clip(y / ph)
    x1, y1 = clip((x + w) / pw), clip((y + h) / ph)
    return {'x': round(x0, 4), 'y': round(y0, 4), 'w': round(x1 - x0, 4), 'h': round(y1 - y0, 4)}
