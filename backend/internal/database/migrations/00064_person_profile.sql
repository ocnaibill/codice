-- +goose Up
-- What Wikidata and Wikipedia say about a person, for the page of an author (DEC-146): a short description, the years, the first paragraphs of the
-- biography and a photo with its credit and license. It is read only for a person who holds a Wikidata identifier that a human confirmed
-- (DEC-095, DEC-098), and shown with where it came from. A staff member can hide it, or only the photo, when it is the wrong person.
CREATE TABLE person_profile (
	person_id INTEGER PRIMARY KEY REFERENCES person(id) ON DELETE CASCADE,
	wikidata_id VARCHAR(16) NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	-- A year, a month or a day, as far as Wikidata knows it: "1920", "1920-10", "1920-10-08"; "-0384" is before the common era.
	born VARCHAR(16),
	died VARCHAR(16),
	bio TEXT NOT NULL DEFAULT '',
	bio_language VARCHAR(8),
	bio_title TEXT,
	bio_url TEXT,
	-- pending: Wikipedia was off when the profile was read (it is read when it is turned on); done; none: the person has no page.
	bio_state VARCHAR(8) NOT NULL DEFAULT 'pending' CHECK (bio_state IN ('pending', 'done', 'none')),
	image_path TEXT,
	image_credit TEXT,
	image_license TEXT,
	image_license_url TEXT,
	image_page_url TEXT,
	hidden BOOLEAN NOT NULL DEFAULT FALSE,
	image_hidden BOOLEAN NOT NULL DEFAULT FALSE,
	fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_by UUID REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX idx_person_profile_wikidata ON person_profile (wikidata_id);
CREATE INDEX idx_person_profile_bio_pending ON person_profile (person_id) WHERE bio_state = 'pending';

-- +goose Down
DROP TABLE person_profile;
