#!/usr/bin/env bash
# Reset the DEVELOPMENT PostgreSQL database of Codice (drop and recreate the
# public schema). The API recreates the tables on next startup.
#
# Safe by default:
#   * With no flags it only prints what it WOULD do (dry run).
#   * --yes is required to act, and you must then type the database name.
#   * A pg_dump backup is written to ./tmp/reset-backups/ first (skip with --no-backup).
#   * Refuses to run when APP_ENV=production.
#   * Never touches uploaded files, covers or the managed storage. Delete those by hand.
#   * Never prints credentials.
#
# How it reaches the database (first match wins):
#   docker : a running container named $POSTGRES_CONTAINER (default codice_db),
#            using `docker exec`; no password needed.
#   psql   : `psql` and `pg_dump` in PATH plus DATABASE_URL in the environment
#            (export it yourself; this script does not read .env files).
#
# Usage:
#   scripts/reset-dev-db.sh                 # dry run
#   scripts/reset-dev-db.sh --yes           # backup, then reset the database
#   scripts/reset-dev-db.sh --yes --redis   # also FLUSHDB on Redis
#
# Overridable through the environment (defaults match docker-compose.yml):
#   POSTGRES_CONTAINER=codice_db  POSTGRES_USER=codice_user  POSTGRES_DB=codice_db
#   REDIS_CONTAINER=codice_redis  REDIS_URL (psql mode, needs redis-cli)

set -euo pipefail

PG_CONTAINER="${POSTGRES_CONTAINER:-codice_db}"
PG_USER="${POSTGRES_USER:-codice_user}"
PG_DB="${POSTGRES_DB:-codice_db}"
REDIS_CONTAINER="${REDIS_CONTAINER:-codice_redis}"

DO_IT=0
DO_REDIS=0
DO_BACKUP=1
for arg in "$@"; do
  case "$arg" in
    --yes) DO_IT=1 ;;
    --redis) DO_REDIS=1 ;;
    --no-backup) DO_BACKUP=0 ;;
    -h|--help) sed -n '2,26p' "$0"; exit 0 ;;
    *) echo "Unknown option: $arg (use --help)" >&2; exit 2 ;;
  esac
done

if [ "${APP_ENV:-}" = "production" ]; then
  echo "Refusing to run: APP_ENV=production." >&2
  exit 1
fi

# --- pick a way to reach the database -------------------------------------
MODE="none"
if command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$PG_CONTAINER"; then
  MODE="docker"
  TARGET="container '$PG_CONTAINER', database '$PG_DB', user '$PG_USER'"
elif command -v psql >/dev/null 2>&1 && command -v pg_dump >/dev/null 2>&1 && [ -n "${DATABASE_URL:-}" ]; then
  MODE="psql"
  url_tail="${DATABASE_URL##*/}"
  PG_DB="${url_tail%%\?*}"
  TARGET="DATABASE_URL (not printed), database '$PG_DB'"
else
  TARGET="(none found: no running '$PG_CONTAINER' container, and no psql + pg_dump + DATABASE_URL)"
fi

pg_dump_cmd() {
  if [ "$MODE" = "docker" ]; then docker exec "$PG_CONTAINER" pg_dump -U "$PG_USER" -d "$PG_DB"
  else pg_dump "$DATABASE_URL"; fi
}
psql_cmd() {
  if [ "$MODE" = "docker" ]; then docker exec "$PG_CONTAINER" psql -v ON_ERROR_STOP=1 -U "$PG_USER" -d "$PG_DB" "$@"
  else psql -v ON_ERROR_STOP=1 "$DATABASE_URL" "$@"; fi
}

BACKUP_DIR="$(cd "$(dirname "$0")/.." && pwd)/tmp/reset-backups"
BACKUP_FILE="$BACKUP_DIR/${PG_DB}_$(date +%Y%m%d_%H%M%S).sql"

echo "Mode   : $MODE"
echo "Target : $TARGET"
echo "Plan"
if [ "$DO_BACKUP" -eq 1 ]; then echo "  1. pg_dump to $BACKUP_FILE"; else echo "  1. (backup skipped by --no-backup)"; fi
echo "  2. DROP SCHEMA public CASCADE; CREATE SCHEMA public;  (ALL tables and data in '$PG_DB' are lost)"
if [ "$DO_REDIS" -eq 1 ]; then echo "  3. FLUSHDB on Redis"; else echo "  3. Redis untouched (add --redis to flush it)"; fi
echo "  Files in uploads/, covers/ and managed storage are NOT touched."

if [ "$DO_IT" -ne 1 ]; then
  echo
  echo "Dry run only. Nothing was changed. Re-run with --yes to proceed."
  exit 0
fi

if [ "$MODE" = "none" ]; then
  echo "Cannot reach a database (see Target above). Nothing was changed." >&2
  exit 1
fi

echo
read -r -p "Type the database name ($PG_DB) to confirm: " typed
if [ "$typed" != "$PG_DB" ]; then
  echo "Confirmation did not match. Aborting; nothing was changed." >&2
  exit 1
fi

if [ "$DO_BACKUP" -eq 1 ]; then
  mkdir -p "$BACKUP_DIR"
  pg_dump_cmd > "$BACKUP_FILE"
  if [ ! -s "$BACKUP_FILE" ]; then
    rm -f "$BACKUP_FILE"
    echo "Backup is empty or failed; aborting before any change." >&2
    exit 1
  fi
  echo "Backup written: $BACKUP_FILE ($(wc -c < "$BACKUP_FILE") bytes)"
fi

psql_cmd -c 'DROP SCHEMA public CASCADE;' -c 'CREATE SCHEMA public;'
echo "Database '$PG_DB' reset. Restart the API to recreate the schema."

if [ "$DO_REDIS" -eq 1 ]; then
  if [ "$MODE" = "docker" ]; then
    docker exec "$REDIS_CONTAINER" redis-cli FLUSHDB
  elif command -v redis-cli >/dev/null 2>&1 && [ -n "${REDIS_URL:-}" ]; then
    redis-cli -u "$REDIS_URL" FLUSHDB
  else
    echo "Redis not flushed: need the redis container, or redis-cli plus REDIS_URL." >&2
  fi
fi
