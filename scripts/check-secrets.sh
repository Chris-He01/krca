#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

patterns='AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9_]{20,}|xox[baprs]-[A-Za-z0-9-]{10,}|-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----|(?i)(api[_-]?key|access[_-]?token|client[_-]?secret|password)[[:space:]]*[:=][[:space:]]*["'\''][A-Za-z0-9_./+=-]{12,}["'\'']'

if rg -n --hidden \
  -g '!.git/**' \
  -g '!node_modules/**' \
  -g '!frontend/node_modules/**' \
  -g '!go.sum' \
  -g '!scripts/check-secrets.sh' \
  "$patterns" .; then
  echo "Potential secret material detected." >&2
  exit 1
fi

echo "No high-confidence secret patterns detected."
