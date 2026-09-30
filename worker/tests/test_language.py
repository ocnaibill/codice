"""The language of a text, worked out from the text alone (#35)."""
import pytest

from textindex.language import detect

SAMPLES = {
    'en': [
        "It was a bright cold day in April, and the clocks were striking thirteen. The man who had come down from the hills "
        "walked slowly along the road, and he thought that the people in the village did not know what was going to happen to them "
        "when the winter came and the river was frozen over, as it had been once before in the time of his father.",
        "She had not wanted to go, but there was nothing else that could be done, and so they set out before the sun was up, "
        "with all that they could carry, and did not stop until they had reached the top of the hill where they could see the sea.",
    ],
    'pt': [
        "Era uma manhã clara e fria de abril, e os relógios davam treze horas. O homem que tinha descido das montanhas caminhava "
        "devagar pela estrada, e pensava que as pessoas da aldeia não sabiam o que ia acontecer com elas quando chegasse o inverno "
        "e o rio estivesse gelado, como já tinha acontecido uma vez no tempo do seu pai.",
        "Ela não queria ir, mas não havia mais nada que se pudesse fazer, e por isso saíram antes de o sol nascer, com tudo o que "
        "podiam levar, e não pararam até chegar ao alto da colina de onde se via o mar.",
    ],
    'es': [
        "Era una mañana clara y fría de abril, y los relojes daban las trece. El hombre que había bajado de las montañas caminaba "
        "despacio por el camino, y pensaba que la gente del pueblo no sabía lo que les iba a ocurrir cuando llegara el invierno y "
        "el río estuviera helado, como ya había sucedido una vez en tiempos de su padre.",
        "Ella no quería ir, pero no había nada más que se pudiera hacer, y por eso salieron antes de que saliera el sol, con todo "
        "lo que podían llevar, y no se detuvieron hasta llegar a lo alto de la colina desde donde se veía el mar.",
    ],
    'fr': [
        "C'était une journée d'avril claire et froide, et les horloges sonnaient treize heures. L'homme qui était descendu des "
        "montagnes marchait lentement sur la route, et il pensait que les gens du village ne savaient pas ce qui allait leur "
        "arriver quand l'hiver viendrait et que la rivière serait gelée, comme cela était déjà arrivé au temps de son père.",
        "Elle ne voulait pas partir, mais il n'y avait plus rien à faire, et ils se mirent donc en route avant que le soleil se "
        "lève, avec tout ce qu'ils pouvaient porter, et ne s'arrêtèrent pas avant d'avoir atteint le sommet de la colline.",
    ],
    'it': [
        "Era una mattina di aprile chiara e fredda, e gli orologi suonavano le tredici. L'uomo che era sceso dalle montagne "
        "camminava lentamente lungo la strada, e pensava che la gente del villaggio non sapesse che cosa sarebbe accaduto loro "
        "quando fosse arrivato l'inverno e il fiume fosse gelato, come era già successo una volta ai tempi di suo padre.",
        "Lei non voleva andare, ma non c'era più nulla da fare, e così partirono prima che il sole si alzasse, con tutto quello "
        "che potevano portare, e non si fermarono finché non ebbero raggiunto la cima della collina da cui si vedeva il mare.",
    ],
    'de': [
        "Es war ein heller, kalter Tag im April, und die Uhren schlugen dreizehn. Der Mann, der von den Bergen herabgekommen war, "
        "ging langsam die Straße entlang, und er dachte, dass die Leute im Dorf nicht wussten, was mit ihnen geschehen würde, wenn "
        "der Winter käme und der Fluss zugefroren wäre, wie es schon einmal zur Zeit seines Vaters gewesen war.",
        "Sie hatte nicht gehen wollen, aber es war nichts anderes mehr zu tun, und so brachen sie auf, bevor die Sonne aufging, "
        "mit allem, was sie tragen konnten, und hielten nicht an, bis sie die Spitze des Hügels erreicht hatten.",
    ],
    'nl': [
        "Het was een heldere, koude dag in april, en de klokken sloegen dertien. De man die van de bergen was afgedaald liep "
        "langzaam over de weg, en hij dacht dat de mensen in het dorp niet wisten wat er met hen zou gebeuren wanneer de winter "
        "kwam en de rivier bevroren was, zoals dat al eens was gebeurd in de tijd van zijn vader.",
        "Ze had niet willen gaan, maar er was niets anders meer aan te doen, en dus vertrokken ze voordat de zon opkwam, met alles "
        "wat ze konden dragen, en ze hielden niet op tot ze de top van de heuvel hadden bereikt waar ze de zee konden zien.",
    ],
}
SCRIPTS = {
    'ar': "كان يوما مشرقا وباردا في شهر أبريل وكانت الساعات تدق الثالثة عشرة وكان الرجل الذي نزل من الجبال يسير ببطء على الطريق وهو يفكر في أن أهل القرية لا يعرفون ما سيحدث لهم عندما يأتي الشتاء ويتجمد النهر كما حدث مرة في زمن أبيه.",
    'ru': "Был ясный холодный апрельский день, и часы пробили тринадцать. Человек, спустившийся с гор, медленно шёл по дороге и думал о том, что жители деревни не знают, что с ними будет, когда придёт зима и река покроется льдом, как это уже было однажды во времена его отца.",
    'el': "Ήταν μια φωτεινή και κρύα μέρα του Απριλίου και τα ρολόγια χτυπούσαν δεκατρείς. Ο άνθρωπος που είχε κατέβει από τα βουνά περπατούσε αργά στον δρόμο και σκεφτόταν ότι οι κάτοικοι του χωριού δεν ήξεραν τι θα τους συνέβαινε όταν θα ερχόταν ο χειμώνας και το ποτάμι θα πάγωνε.",
    'he': "היה זה יום אפריל בהיר וקר והשעונים צלצלו שלוש עשרה והאיש שירד מההרים הלך לאט בדרך וחשב שאנשי הכפר אינם יודעים מה יקרה להם כאשר יגיע החורף והנהר יקפא כפי שקרה פעם אחת בימי אביו והם יצאו לדרך לפני שהשמש זרחה.",
    'ko': "사월의 맑고 추운 날이었고 시계들이 열세 시를 치고 있었다. 산에서 내려온 남자는 길을 따라 천천히 걸으며 겨울이 오고 강이 얼어붙으면 마을 사람들에게 무슨 일이 일어날지 그들이 알지 못한다고 생각했다. 그녀는 가고 싶지 않았지만 달리 할 수 있는 일이 없었다.",
    'ja': "四月の晴れた寒い日で、時計は十三時を打っていた。山から下りてきた男は道をゆっくりと歩きながら、冬が来て川が凍ったときに村の人々に何が起こるのかを彼らは知らないのだと考えていた。彼女は行きたくなかったが、ほかにできることは何もなかった。",
    'zh': "那是四月里一个晴朗而寒冷的日子，钟敲了十三下。从山上下来的那个人沿着大路慢慢地走着，心里想着村里的人们并不知道冬天来临、河水结冰的时候会发生什么事，就像他父亲那个时代曾经发生过的那样。她本来不想去，可是再也没有别的办法了。",
    'th': "วันนั้นเป็นวันเดือนเมษายนที่อากาศแจ่มใสและหนาวเย็น นาฬิกาตีบอกเวลาสิบสามนาฬิกา ชายผู้ลงมาจากภูเขาเดินไปตามถนนอย่างช้าๆ และคิดว่าชาวบ้านไม่รู้ว่าจะเกิดอะไรขึ้นกับพวกเขาเมื่อฤดูหนาวมาถึงและแม่น้ำกลายเป็นน้ำแข็ง",
    'hi': "अप्रैल का एक उजला और ठंडा दिन था और घड़ियों ने तेरह बजाए। पहाड़ों से उतरा हुआ आदमी सड़क पर धीरे धीरे चल रहा था और सोच रहा था कि गाँव के लोग नहीं जानते कि जब सर्दी आएगी और नदी जम जाएगी तब उनके साथ क्या होगा।",
}


@pytest.mark.parametrize('language', sorted(SAMPLES))
def test_each_language_of_the_latin_alphabet_is_told_from_its_words(language):
    for sample in SAMPLES[language]:
        assert detect(sample) == language


@pytest.mark.parametrize('language', sorted(SCRIPTS))
def test_a_script_that_belongs_to_one_language_says_which(language):
    assert detect(SCRIPTS[language]) == language


def test_the_languages_that_look_alike_are_not_mistaken_for_each_other():
    # Portuguese and Spanish, Spanish and Italian, German and Dutch share many words and letters.
    for pair in (('pt', 'es'), ('es', 'it'), ('de', 'nl'), ('fr', 'it'), ('en', 'nl')):
        for wanted in pair:
            for sample in SAMPLES[wanted]:
                assert detect(sample) == wanted


def test_a_long_text_is_told_even_with_noise_in_it():
    noisy = ' '.join(SAMPLES['pt']) + ' 154 Frank Herbert \n ~~ ¶¶ ' + ' '.join(SAMPLES['pt']) + ' Ee ee Ales 5 i eC ce ' + ' '.join(SAMPLES['pt'])
    assert detect(noisy) == 'pt'


def test_a_book_in_two_languages_is_not_given_one():
    assert detect(' '.join(SAMPLES['pt'] + SAMPLES['en'] + SAMPLES['pt'] + SAMPLES['en'])) is None


def test_there_is_no_answer_for_too_little_text_or_for_no_words():
    assert detect('') is None and detect(None) is None
    assert detect('The end.') is None
    assert detect(SAMPLES['en'][0][:120]) is None
    assert detect('1 2 3 4 5 6 7 8 9 ' * 60) is None
    assert detect('xqz wvk jhg ' * 80) is None


def test_a_mixed_script_text_is_not_given_a_language():
    assert detect(SCRIPTS['ru'][:150] + SCRIPTS['ar'][:150] + SCRIPTS['ko'][:150]) is None


def test_han_letters_with_kana_are_japanese_and_without_them_chinese():
    assert detect(SCRIPTS['ja']) == 'ja' and detect(SCRIPTS['zh']) == 'zh'


def test_a_handful_of_the_smallest_words_is_not_a_text():
    assert detect('de a o e um da do em se na ' * 4) is None  # all Portuguese, but 40 words of 2 letters


def test_a_few_long_words_are_not_enough_either():
    few = 'o desenvolvimento extraordinariamente complicado da administração governamental e a responsabilidade institucional dos profissionais que não'
    assert len(few.split()) < 30 and sum(c.isalpha() for c in few) > 120
    assert detect(few) is None


def test_a_text_that_two_languages_score_alike_is_not_given_either():
    # Words that Portuguese and Spanish both have, and only those.
    assert detect('de que a e para no se por como ' * 6) is None


def test_han_letters_with_a_little_kana_are_still_japanese():
    kanji_heavy = ('日本政府は経済成長を促進するために新たな政策を発表し、国内外の企業に対して投資を呼びかけた。'
                   '専門家は、この政策が雇用の拡大と地域経済の活性化につながると指摘している。')
    assert detect(kanji_heavy) == 'ja'
