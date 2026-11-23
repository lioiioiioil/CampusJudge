#!/bin/sh
set -eu

directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$directory"
if [ -e .env ] || [ -L .env ]; then
  printf '%s\n' '.env already exists; keeping its passwords and settings.' >&2
  exit 1
fi

random_password() {
  value=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
  [ "${#value}" -eq 64 ] || { printf '%s\n' 'Could not generate a password.' >&2; exit 1; }
  printf '%s' "$value"
}

release_image() {
  key=$1
  fallback=$2
  value=$fallback
  if [ -f release.env ]; then
    value=$(sed -n "s/^${key}=//p" release.env)
    [ -n "$value" ] || value=$fallback
  fi
  case "$value" in
    -*|*[!a-zA-Z0-9_.:/@-]*) printf '%s\n' 'Invalid release image reference.' >&2; exit 1 ;;
  esac
  printf '%s' "$value"
}

app_image=$(release_image CAMPUS_APP_IMAGE campusjudge:local)
runner_image=$(release_image CAMPUS_RUNNER_IMAGE campusjudge-runner:local)
postgres_image=$(release_image CAMPUS_POSTGRES_IMAGE postgres:17-alpine)
database_password=$(random_password)
teacher_password=$(random_password)

# Noclobber also protects against a second initializer racing this process.
(
  umask 077
  set -C
  {
    printf 'CAMPUS_APP_IMAGE=%s\n' "$app_image"
    printf 'CAMPUS_RUNNER_IMAGE=%s\n' "$runner_image"
    printf 'CAMPUS_POSTGRES_IMAGE=%s\n' "$postgres_image"
    printf 'POSTGRES_PASSWORD=%s\n' "$database_password"
    printf 'CAMPUS_TEACHER_PASSWORD=%s\n' "$teacher_password"
    printf '%s\n' 'CAMPUS_BIND_ADDRESS=127.0.0.1' 'CAMPUS_PORT=8080' \
      'CAMPUS_TRUSTED_PROXIES=' 'CAMPUS_DOCKER_SOCKET=/var/run/docker.sock' \
      'CAMPUS_VOLUME_NAME=campusjudge-server-postgres'
  } > .env
)
printf '%s\n' 'Created private .env. Teacher login: teacher. Read its password from CAMPUS_TEACHER_PASSWORD in .env.'
