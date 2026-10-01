import pytest

from people import normalize_name, parse_name

# The same vectors as backend/internal/people/people_test.go.
VECTORS = [
    ('Herbert, Frank, author', 'Frank Herbert'),
    ('Herbert, Frank, Author', 'Frank Herbert'),
    ('  Herbert ,  Frank ,  author  ', 'Frank Herbert'),
    ('Herbert, Frank, author.', 'Frank Herbert'),
    ('García Márquez, Gabriel, autor', 'Gabriel García Márquez'),
    ('Saint-Exupéry, Antoine de, auteur', 'Antoine de Saint-Exupéry'),
    ('Schoenherr, John, illustrator', 'John Schoenherr'),
    ('Macedo, Henrique de, tradutor', 'Henrique de Macedo'),
    ('Frank Herbert, author', 'Frank Herbert'),
    ('Frank Herbert (author)', 'Frank Herbert'),
    ('Herbert, Frank (author)', 'Frank Herbert'),
    ('Frank Herbert', 'Frank Herbert'),
    ('Herbert, Frank', 'Herbert, Frank'),
    ('Plato, Aristotle', 'Plato, Aristotle'),
    ('Frank Herbert, Brian Herbert', 'Frank Herbert, Brian Herbert'),
    ('Herbert, Frank, Schoenherr, John, author', 'Herbert, Frank, Schoenherr, John, author'),
    ('King, Martin Luther, Jr., author', 'King, Martin Luther, Jr., author'),
    ('author', 'author'),
    ('', ''),
]


@pytest.mark.parametrize('raw,want', VECTORS)
def test_normalize_name(raw, want):
    name, changed = normalize_name(raw)
    assert name == want
    assert changed == (name != raw)


def test_none_is_nothing():
    assert normalize_name(None) == ('', False)


PARTS = [
    ('Herbert, Frank, author', 'Frank Herbert', 'Herbert', 'Frank'),
    ('Herbert, Frank (author)', 'Frank Herbert', 'Herbert', 'Frank'),
    ('  García Márquez ,  Gabriel , autor ', 'Gabriel García Márquez', 'García Márquez', 'Gabriel'),
    ('Saint-Exupéry, Antoine de, auteur', 'Antoine de Saint-Exupéry', 'Saint-Exupéry', 'Antoine de'),
    ('Frank Herbert, author', 'Frank Herbert', None, None),     # a role, but nothing says the surname
    ('Frank Herbert', 'Frank Herbert', None, None),             # depends on the culture
    ('Herbert, Frank', 'Herbert, Frank', None, None),           # no role: not certain
    ('Herbert, Frank, Schoenherr, John, author', 'Herbert, Frank, Schoenherr, John, author', None, None),
    ('', '', None, None),
]


@pytest.mark.parametrize('raw,name,family,given', PARTS)
def test_only_the_catalogues_way_with_a_role_says_which_is_the_surname(raw, name, family, given):
    got = parse_name(raw)
    assert got[:3] == (name, family, given)
    assert got[3] == (name != raw)


class TestNameKeyAndRoles:
    def test_name_key_is_the_same_for_the_same_words_in_any_order_without_accents_or_role_words(self):
        from people import name_key
        assert name_key('Herbert, Frank') == name_key('Frank Herbert') == name_key('Frank Herbert (author)')
        assert name_key('Antônio da Conceição') == name_key('antonio da conceicao')
        assert name_key('Herbert, Frank') != name_key('Brian Herbert')
        assert name_key('') == '' and name_key(None) == ''

    def test_a_provider_that_says_nothing_about_the_role_says_author(self):
        from people import library_roles
        assert library_roles(None) == ['author'] and library_roles('  ') == ['author']

    def test_provider_roles_are_the_librarys_roles_and_a_role_it_has_no_word_for_is_left_out(self):
        from people import library_roles
        assert library_roles('writer') == ['author']
        assert library_roles('Penciller') == ['illustrator']
        assert library_roles('translator') == ['translator']
        assert library_roles('colorist') == [] and library_roles('letterer') == []

    def test_several_roles_are_each_kept_once_in_order(self):
        from people import library_roles
        assert library_roles('writer, inker') == ['author', 'illustrator']
        assert library_roles('penciller, inker') == ['illustrator']
        assert library_roles('writer and editor') == ['author', 'editor']
