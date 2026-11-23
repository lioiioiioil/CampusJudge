#!/bin/sh
set -eu

directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$directory"
[ -f images.tar ] && [ -f SHA256SUMS ] && [ -f release.env ] || {
  printf '%s\n' 'Run this command from an extracted CampusJudge container release.' >&2
  exit 1
}

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum -c SHA256SUMS
else
  shasum -a 256 -c SHA256SUMS
fi

expected=$(sed -n 's/^CAMPUS_PLATFORM=//p' release.env)
server=$(docker info --format '{{.OSType}}/{{.Architecture}}')
case "$server" in
  linux/x86_64) server=linux/amd64 ;;
  linux/aarch64) server=linux/arm64 ;;
esac
[ "$expected" = "$server" ] || {
  printf 'Image platform %s does not match Docker server %s. Download the matching release.\n' "$expected" "$server" >&2
  exit 1
}
docker image load --input images.tar
