#!/usr/bin/env bash

# Read dotenv entries literally. Commands prefer file values, except for keys
# listed in ENV_OVERRIDE_KEYS. Sourced scripts retain environment precedence.
load_env() {
  local env_file="${1:-.env}" precedence="${2:-environment}" line key value
  [[ -f "$env_file" ]] || return 0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]] || continue
    key="${BASH_REMATCH[2]}"
    value="${BASH_REMATCH[3]}"
    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%"${value##*[![:space:]]}"}"
    if [[ "$value" == \"*\" || "$value" == \'*\' ]]; then
      value="${value:1:${#value}-2}"
    fi
    if [[ "$precedence" == file && " ${ENV_OVERRIDE_KEYS:-} " != *" $key "* ]] || [[ ! -v "$key" ]]; then
      export "$key=$value"
    fi
  done < "$env_file"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  set -Eeuo pipefail
  if [[ $# -eq 0 ]]; then
    printf 'Usage: bash scripts/with-env.sh COMMAND [ARGS...]\n' >&2
    exit 2
  fi
  load_env "${ENV_FILE:-.env}" file
  exec "$@"
fi
