-- +goose Up
-- The links the other way (#109, the bridge through English): the translations an entry lists are looked up by the word
-- they are of ("what does the English word run list in Portuguese"), which the index of the word listed does not serve.
CREATE INDEX dictionary_links_target ON dictionary_links (target_lang, target_word, lang);

-- +goose Down
DROP INDEX IF EXISTS dictionary_links_target;
