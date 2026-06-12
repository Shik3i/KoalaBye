#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 BACKUP_PATH RESTORE_PATH" >&2
  exit 2
fi

backup=$1
restore=$2

if [ ! -f "$backup" ]; then
  echo "backup does not exist: $backup" >&2
  exit 1
fi
if [ -e "$restore" ] || [ -e "$restore-wal" ] || [ -e "$restore-shm" ]; then
  echo "refusing to overwrite an existing database or WAL files: $restore" >&2
  exit 1
fi
if ! command -v sqlite3 >/dev/null 2>&1; then
  echo "sqlite3 is required to verify the backup before restore" >&2
  exit 1
fi

result=$(sqlite3 "$backup" "PRAGMA integrity_check;")
if [ "$result" != "ok" ]; then
  echo "backup integrity check failed: $result" >&2
  exit 1
fi
mkdir -p "$(dirname "$restore")"
cp "$backup" "$restore"
echo "database restored: $restore"
