-- +goose Up
-- The audit log becomes append-only. Its foreign key to users is dropped: with
-- ON DELETE SET NULL, deleting an account would UPDATE audit rows, which this
-- migration forbids. The username is already copied into each entry, so an
-- entry stays readable after its actor is gone.
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_actor_id_fkey;

-- +goose StatementBegin
CREATE FUNCTION audit_log_append_only() RETURNS trigger AS $$
BEGIN
	RAISE EXCEPTION 'audit_log is append-only' USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_log_no_change BEFORE UPDATE OR DELETE ON audit_log
FOR EACH ROW EXECUTE FUNCTION audit_log_append_only();
CREATE TRIGGER audit_log_no_truncate BEFORE TRUNCATE ON audit_log
FOR EACH STATEMENT EXECUTE FUNCTION audit_log_append_only();

-- +goose Down
DROP TRIGGER IF EXISTS audit_log_no_truncate ON audit_log;
DROP TRIGGER IF EXISTS audit_log_no_change ON audit_log;
DROP FUNCTION IF EXISTS audit_log_append_only();
-- Entries of deleted accounts would break the key; clear only their pointer.
ALTER TABLE audit_log DISABLE TRIGGER USER;
UPDATE audit_log SET actor_id = NULL WHERE actor_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = audit_log.actor_id);
ALTER TABLE audit_log ENABLE TRIGGER USER;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL;
