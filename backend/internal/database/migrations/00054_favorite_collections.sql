-- +goose Up
-- Favorite collections (#208, DEC-130): the person favorites a whole franchise or one of their own lists, and the home shows it
-- as one card, in the place of the cards of its works (the grouping that the home used to compute, #184).
CREATE TABLE favorite_collections (
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, collection_id)
);
CREATE INDEX idx_favorite_collections_collection ON favorite_collections(collection_id);

-- What the home already showed stays: a person who had favorited works of a series saw one card for the series, and now has the
-- collection among their favorites, as of the latest of those favorites. Their favorites of works are left as they are.
INSERT INTO favorite_collections (user_id, collection_id, created_at)
SELECT f.user_id, cw.collection_id, max(f.created_at)
FROM favorites f
JOIN collection_works cw ON cw.work_id = f.work_id AND cw.official
JOIN collections c ON c.id = cw.collection_id AND c.retired_at IS NULL
GROUP BY f.user_id, cw.collection_id;

-- +goose Down
DROP TABLE IF EXISTS favorite_collections;
