#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 DATABASE_PATH BACKUP_PATH" >&2
  exit 2
fi

database=$1
backup=$2

if [ ! -f "$database" ]; then
  echo "database does not exist: $database" >&2
  exit 1
fi
if [ -e "$backup" ]; then
  echo "refusing to overwrite existing backup: $backup" >&2
  exit 1
fi
if ! command -v sqlite3 >/dev/null 2>&1; then
  echo "sqlite3 is required for an online-safe backup; otherwise stop KoalaBye and copy the database while stopped" >&2
  exit 1
fi

mkdir -p "$(dirname "$backup")"
sqlite3 "$database" ".backup '$backup'"
result=$(sqlite3 "$backup" "PRAGMA integrity_check;")
if [ "$result" != "ok" ]; then
  rm -f "$backup"
  echo "backup integrity check failed: $result" >&2
  exit 1
fi
echo "backup created: $backup"
