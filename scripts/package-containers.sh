#!/bin/sh
set -eu

usage() {
  printf '%s\n' 'Usage: PLATFORM=linux/amd64 OUTPUT_DIR=dist ./scripts/package-containers.sh VERSION' >&2
  exit 2
}
[ "$#" -eq 1 ] || usage
version=$1
case "$version" in
  ''|[-.]*|*[!a-zA-Z0-9_.-]*) usage ;;
esac
[ "${#version}" -le 100 ] || usage
platform=${PLATFORM:-linux/amd64}
case "$platform" in
  linux/amd64) architecture=amd64 ;;
  linux/arm64) architecture=arm64 ;;
  *) printf '%s\n' 'Supported platforms: linux/amd64, linux/arm64.' >&2; exit 2 ;;
esac

repository=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repository"
source_commit=$(git rev-parse HEAD)
source_state=clean
if [ -n "$(git status --porcelain --untracked-files=normal)" ]; then
  source_state=dirty
  [ "${ALLOW_DIRTY:-0}" = 1 ] || {
    printf '%s\n' 'Commit the source before packaging a release, or use ALLOW_DIRTY=1 for a local test bundle.' >&2
    exit 1
  }
fi
output=${OUTPUT_DIR:-dist}
mkdir -p "$output"
output=$(CDPATH= cd -- "$output" && pwd)
name=campusjudge-${version}-linux-${architecture}
archive=$output/$name.tar.gz
[ ! -e "$archive" ] && [ ! -e "$archive.sha256" ] || {
  printf 'Release output already exists: %s\n' "$archive" >&2
  exit 1
}

work=$(mktemp -d "${TMPDIR:-/tmp}/campusjudge-package.XXXXXXXX")
trap 'rm -rf "$work"' EXIT
trap 'exit 1' HUP INT TERM
bundle=$work/$name
mkdir "$bundle"
app_image=campusjudge:${version}-${architecture}
runner_image=campusjudge-runner:${version}-${architecture}
postgres_image=campusjudge-postgres:${version}-${architecture}

docker buildx version >/dev/null 2>&1 || {
  printf '%s\n' 'Docker Buildx is required to build platform-specific release images.' >&2
  exit 1
}
export DOCKER_BUILDKIT=1
docker build --platform "$platform" --tag "$app_image" --file Dockerfile .
docker build --platform "$platform" --tag "$runner_image" --file judge/Dockerfile .
# A containerd image store can retain several platforms behind the base tag.
# Export a one-platform image through BuildKit instead of retagging that index.
# https://docs.docker.com/build/building/multi-platform/
printf '%s\n' 'FROM postgres:17-alpine' > "$work/Postgres.Dockerfile"
docker build --platform "$platform" --tag "$postgres_image" --file "$work/Postgres.Dockerfile" "$work"
for image in "$app_image" "$runner_image" "$postgres_image"; do
  actual=$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$image")
  [ "$actual" = "$platform" ] || {
    printf 'Image %s has platform %s, expected %s.\n' "$image" "$actual" "$platform" >&2
    exit 1
  }
done

cp deploy/compose.yaml deploy/.env.example deploy/init-env.sh deploy/load-images.sh "$bundle/"
cp docs/deployment.md "$bundle/README.md"
cp docs/postgresql.md docs/automatic-judge.md "$bundle/"
printf 'CAMPUS_APP_IMAGE=%s\nCAMPUS_RUNNER_IMAGE=%s\nCAMPUS_POSTGRES_IMAGE=%s\nCAMPUS_PLATFORM=%s\n' \
  "$app_image" "$runner_image" "$postgres_image" "$platform" > "$bundle/release.env"
docker image inspect --format '{{.Id}} {{join .RepoTags ","}}' \
  "$app_image" "$runner_image" "$postgres_image" > "$bundle/IMAGE_IDS"
printf '%s\n' "$source_commit" > "$bundle/SOURCE_COMMIT"
printf '%s\n' "$source_state" > "$bundle/SOURCE_STATE"
docker image save --output "$bundle/images.tar" "$app_image" "$runner_image" "$postgres_image"
(
  cd "$bundle"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum images.tar > SHA256SUMS
  else
    shasum -a 256 images.tar > SHA256SUMS
  fi
)
tar -czf "$work/$name.tar.gz" -C "$work" "$name"
mv "$work/$name.tar.gz" "$archive"
(
  cd "$output"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$name.tar.gz" > "$name.tar.gz.sha256"
  else
    shasum -a 256 "$name.tar.gz" > "$name.tar.gz.sha256"
  fi
)
printf 'Created %s\nChecksum %s.sha256\n' "$archive" "$archive"
