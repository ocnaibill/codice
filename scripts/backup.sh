#!/usr/bin/env bash
# Backup of a Códice instance installed with docker-compose.full.yml: the database, the list of
# files and the files themselves, in one package written to DEST_DIR, followed by the removal of
# old packages (7 daily, 4 weekly, 3 monthly; pass --daily/--weekly/--monthly to change).
#
#   CODICE_BACKUP_PASSPHRASE='...' scripts/backup.sh /mnt/backups/codice [--daily 14]
#
# It runs the backup as the container's own user (the one that owns the library and can read every
# file) and streams the package to a file of yours, with permission 600. Run it from any folder.
set -euo pipefail

dest=${1:?usage: scripts/backup.sh DEST_DIR [prune-backups options]}
shift
cd "$(dirname "$0")/.."
compose=(docker compose -f docker-compose.full.yml)

mkdir -p "$dest"
dest=$(cd "$dest" && pwd)
name="codice-backup-$(date +%Y%m%d-%H%M%S).tar${CODICE_BACKUP_PASSPHRASE:+.age}"

umask 077
# Written aside and renamed at the end: a backup that fails leaves no half file that could be
# mistaken for a good one (prune-backups ignores anything that is not a finished package).
trap 'rm -f "$dest/$name.part"' EXIT
"${compose[@]}" run --rm --no-deps -T -e CODICE_BACKUP_PASSPHRASE \
  backend codice-admin backup --out - --include-files > "$dest/$name.part"
mv "$dest/$name.part" "$dest/$name"
echo "Package: $dest/$name"

# Pruning only reads names, so it runs as you and needs no access to the library.
"${compose[@]}" run --rm --no-deps -T --user "$(id -u):$(id -g)" -v "$dest:/backups" \
  backend codice-admin prune-backups --dir /backups --yes "$@"
