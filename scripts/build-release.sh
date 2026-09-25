#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
version=${1:-}
go_command=${GO:-go}

if [ -z "$version" ]; then
	version=$(git -C "$project_dir" describe --tags --always --dirty 2>/dev/null || printf 'dev')
fi

case "$version" in
	''|*[!A-Za-z0-9._+-]*)
		printf 'invalid version %s; use only letters, numbers, dot, underscore, plus, and hyphen\n' "$version" >&2
		exit 2
		;;
esac

if ! command -v "$go_command" >/dev/null 2>&1; then
	printf 'Go was not found. Install Go 1.25 or newer, or set GO to its executable path.\n' >&2
	exit 1
fi

dist_dir="$project_dir/dist"
staging_dir="$dist_dir/.staging"

if [ "$project_dir" = / ]; then
	printf 'refusing to build from the filesystem root\n' >&2
	exit 1
fi

rm -rf -- "$dist_dir"
mkdir -p "$staging_dir"
trap 'rm -rf -- "$staging_dir"' EXIT HUP INT TERM

cd "$project_dir"

archives=''
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
	goos=${target%/*}
	goarch=${target#*/}
	base_name="aissh_${version}_${goos}_${goarch}"
	package_dir="$staging_dir/$base_name"
	archive_name="$base_name.tar.gz"

	mkdir -p "$package_dir"
	printf 'building %s/%s\n' "$goos" "$goarch"
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" "$go_command" build \
		-trimpath \
		-ldflags "-s -w -X main.version=$version" \
		-o "$package_dir/aissh" \
		./cmd/aissh

	tar -C "$package_dir" -czf "$dist_dir/$archive_name" aissh
	archives="$archives $archive_name"
done

if command -v sha256sum >/dev/null 2>&1; then
	(
		cd "$dist_dir"
		# shellcheck disable=SC2086
		sha256sum $archives > SHA256SUMS
	)
elif command -v shasum >/dev/null 2>&1; then
	(
		cd "$dist_dir"
		# shellcheck disable=SC2086
		shasum -a 256 $archives > SHA256SUMS
	)
else
	printf 'neither sha256sum nor shasum is available\n' >&2
	exit 1
fi

printf 'release files written to %s\n' "$dist_dir"
