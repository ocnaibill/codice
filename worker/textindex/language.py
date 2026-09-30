"""The language a text is written in, worked out from the text and nothing else (#35).

No model and nothing to download: a script that belongs to one language says it (Hangul, Greek, Thai),
and among the languages of the Latin alphabet the share of a text that is made of the small words every
language has (articles, prepositions, pronouns) is very different in each of them. What is answered is
the language (`pt`, not `pt-BR`): telling the two Portuguese apart is left to whoever declares it.

It says nothing when it cannot tell: too little text, or two languages that score alike (a book in two
languages). A wrong label is worse than none, because it is what a person will read on the sheet.
"""
import re
import unicodedata
from collections import Counter

MIN_LETTERS = 120        # of the Latin alphabet, to compare the languages by their words
SCRIPT_LETTERS = 50      # of a script that belongs to one language (a letter of Han or Hangul says much more)
MIN_WORDS = 30           # of the Latin alphabet, to compare the languages
MIN_SHARE = 0.12         # of the words that must be one language's small words
MARGIN = 1.3             # the best must beat the second by this factor
SCRIPT_SHARE = 0.6       # of the letters that must be of one script

# The small words of each language, with the accents they are written with. Words shared by several
# languages are in each list, because they are as frequent in each: what tells them apart is the rest.
STOPWORDS = {
    'en': 'the of and to in a is that it was for on as with he be at by his i this had not are but from or '
          'have an they which you were her all she there would their we him been has when who will more no if '
          'out so said what up its about into than them can only other new some could time these two may then do '
          'first any my now such like our over man me even most made after also did many before must through '
          'back years where much your way well down should because each just those people mr how too little '
          'state good very make world still own see men work long get here between both life being under never '
          'day same another know while last might us great old year off come since against go came right used '
          'take three'.split(),
    'pt': 'de a o que e do da em um para é com não uma os no se na por mais as dos como mas foi ao ele das tem '
          'à seu sua ou ser quando muito há nos já está eu também só pelo pela até isso ela entre era depois sem '
          'mesmo aos ter seus quem nas me esse eles estão você tinha foram essa num nem suas meu às minha têm '
          'numa pelos elas havia seja qual será nós tenho lhe deles essas esses pelas este fosse dele tu te '
          'vocês vos lhes meus minhas teu tua teus tuas nosso nossa nossos nossas dela delas esta estes estas '
          'aquele aquela aqueles aquelas isto aquilo'.split(),
    'es': 'de la que el en y a los del se las por un para con no una su al es lo como más pero sus le ya o fue '
          'este ha sí porque esta son entre está cuando muy sin sobre también me hasta hay donde quien desde '
          'todo nos durante todos uno les ni contra otros ese eso ante ellos e esto mí antes algunos qué unos '
          'yo otro otras otra él tanto esa estos mucho quienes nada muchos cual poco ella estar estas algunas '
          'algo nosotros mi mis tú te ti tu tus ellas nosotras vosotros vosotras os mío mía'.split(),
    'fr': 'de la le et les des en un du une que est pour qui dans a par plus pas au sur ne se ce il sont les '
          'mais avec ou son comme je on cette elle ses nous été aussi leur y vous ont fait tout était deux '
          'très bien sans être même sa aux où ces peut alors si avait après entre encore ils moi lui leurs '
          'dont cela toute tous rien faire dit dire comme puis quand celui notre votre mon ma mes ton ta tes '
          'te me nos vos lorsque depuis tandis jamais toujours déjà là'.split(),
    'it': 'di che e la il un a per in è non una sono con si da come ma le i del più al lo dei ha anche '
          'della nel o se gli suo alla ci ne ho questo quando mi tutto lui lei essere fatto dopo molto '
          'sua era aveva quella sul cui tra dal già degli nella dello delle nei senza loro due poi ancora '
          'proprio ogni solo perché questa quello quelli così altro altri sempre mio mia miei tuo tua suoi '
          'nostro vostro ai alle agli dall dalla dalle col sulla sulle'.split(),
    'de': 'der die und in den von zu das mit sich des auf für ist im dem nicht ein eine als auch es an werden '
          'aus er hat dass sie nach wird bei einer um am sind noch wie einem über einen so zum war haben nur '
          'oder aber vor zur bis mehr durch man sein wurde sei ich wir ihr du mich mir dir ihn ihm uns euch '
          'ihnen dieser diese dieses jede jeder kein keine wenn weil dann da hier dort schon immer wieder '
          'sehr kann können muss müssen will wollen soll sollen'.split(),
    'nl': 'de van het een en in is dat op te zijn voor met niet aan er die ook als maar om bij dan nog hij '
          'wordt door uit over naar zich hun wel kan zou of had al haar worden meer deze tot zo wat ze heeft '
          'geen dit andere veel toen ik je we jullie mij jou hem ons u mijn jouw zijn ons onze ze zullen '
          'moet moeten wil willen nu hier daar altijd nooit weer zeer heel'.split(),
}
# The same number of words for each language (the most frequent first), or the one with the longest list
# would win a text that is half in another: what is compared is how much of a text each language's small
# words cover, and that must not depend on how many of them someone wrote down.
LIST_SIZE = 100
_SETS = {lang: frozenset(words[:LIST_SIZE]) for lang, words in STOPWORDS.items()}

# Scripts that belong (for this purpose) to one language; Latin is the one that needs the words.
_SCRIPTS = {
    'HANGUL': 'ko', 'GREEK': 'el', 'THAI': 'th', 'HEBREW': 'he', 'ARABIC': 'ar', 'CYRILLIC': 'ru',
    'DEVANAGARI': 'hi', 'HIRAGANA': 'ja', 'KATAKANA': 'ja',
}
_WORD = re.compile(r"[^\W\d_]+", re.UNICODE)


def _script(ch):
    try:
        name = unicodedata.name(ch)
    except ValueError:
        return None
    first = name.split(' ', 1)[0]
    if first == 'CJK':
        return 'HAN'
    if first in ('LATIN',) or first in _SCRIPTS or first == 'HAN':
        return first
    return None


def _by_script(text):
    counts = Counter()
    for ch in text:
        if ch.isalpha():
            script = _script(ch)
            if script:
                counts[script] += 1
    total = sum(counts.values())
    if total < SCRIPT_LETTERS:
        return '', total, counts
    if counts['HIRAGANA'] + counts['KATAKANA'] > total * 0.05:
        return 'ja', total, counts  # kana settle it: Japanese writes Han letters too
    script, n = counts.most_common(1)[0]
    if script == 'LATIN':
        return None, total, counts
    if n < total * SCRIPT_SHARE:
        return '', total, counts  # a mix: nothing to say
    return ('zh' if script == 'HAN' else _SCRIPTS.get(script)), total, counts


WINDOW = 60              # words decided together
AGREE = 0.8              # of the windows that decide, the share that must say the same language
DECIDE = 0.5             # of the windows, the share that must be able to decide at all


def _window(words):
    """The language one stretch of words is, or None."""
    hits = {lang: sum(1 for w in words if w in known) for lang, known in _SETS.items()}
    ranked = sorted(hits.items(), key=lambda kv: kv[1], reverse=True)
    (best, top), (_second, next_best) = ranked[0], ranked[1]
    if top / len(words) < MIN_SHARE or top < next_best * MARGIN:
        return None
    return best


def detect(text):
    """The language of a text (`en`, `pt`, `es`, `fr`, `it`, `de`, `nl`, `ko`, `ja`, `zh`, `ar`, `ru`, `el`,
    `he`, `th`, `hi`) or None when it cannot be told.

    Languages of the Latin alphabet are decided window by window, and the text has a language only when
    nearly all the windows that can decide say the same one: a book half in one language and half in
    another has none, and a stray quotation in a third does not change a book's."""
    if not text:
        return None
    language, total, counts = _by_script(text)
    if language is not None:
        return language or None
    if total < MIN_LETTERS:
        return None
    words = [w.lower() for w in _WORD.findall(text)]
    if len(words) < MIN_WORDS:
        return None
    if len(words) < 2 * WINDOW:
        return _window(words)
    windows = [words[i:i + WINDOW] for i in range(0, len(words) - WINDOW + 1, WINDOW)]
    votes = Counter(v for v in (_window(w) for w in windows) if v)
    decided = sum(votes.values())
    if decided < len(windows) * DECIDE:
        return None
    best, n = votes.most_common(1)[0]
    return best if n >= decided * AGREE else None
