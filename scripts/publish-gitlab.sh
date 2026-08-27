#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
dist_dir="$project_dir/dist"
version=${1:-}
package_name=${GITLAB_PACKAGE_NAME:-aissh}
api_url=${CI_API_V4_URL:-${GITLAB_API_URL:-}}
project_id=${CI_PROJECT_ID:-${GITLAB_PROJECT_ID:-}}
create_release=${CREATE_RELEASE:-true}

if [ -z "$version" ]; then
	printf 'usage: %s VERSION\n' "$0" >&2
	exit 2
fi

case "$version" in
	''|*[!A-Za-z0-9._+-]*)
		printf 'invalid version %s; use only letters, numbers, dot, underscore, plus, and hyphen\n' "$version" >&2
		exit 2
		;;
esac

if [ -z "$api_url" ] || [ -z "$project_id" ]; then
	printf 'set GITLAB_API_URL and GITLAB_PROJECT_ID (CI_API_V4_URL and CI_PROJECT_ID are used automatically in CI)\n' >&2
	exit 2
fi

if [ -z "${CI_JOB_TOKEN:-}" ] && [ -z "${GITLAB_TOKEN:-}" ]; then
	printf 'set GITLAB_TOKEN for local publishing (CI_JOB_TOKEN is used automatically in CI)\n' >&2
	exit 2
fi

if ! command -v curl >/dev/null 2>&1; then
	printf 'curl is required to publish release files\n' >&2
	exit 1
fi

api_url=${api_url%/}
package_url="$api_url/projects/$project_id/packages/generic/$package_name/$version"
release_url="$api_url/projects/$project_id/releases"

curl_auth() {
	if [ -n "${CI_JOB_TOKEN:-}" ]; then
		curl --header "JOB-TOKEN: $CI_JOB_TOKEN" "$@"
	else
		curl --header "PRIVATE-TOKEN: $GITLAB_TOKEN" "$@"
	fi
}

files="aissh_${version}_linux_amd64.tar.gz
aissh_${version}_linux_arm64.tar.gz
aissh_${version}_darwin_amd64.tar.gz
aissh_${version}_darwin_arm64.tar.gz
SHA256SUMS"

for file_name in $files; do
	file_path="$dist_dir/$file_name"
	if [ ! -f "$file_path" ]; then
		printf 'missing release file: %s\n' "$file_path" >&2
		exit 1
	fi
done

for file_name in $files; do
	file_path="$dist_dir/$file_name"
	file_url="$package_url/$file_name"
	status=$(curl_auth --location --silent --output /dev/null --write-out '%{http_code}' "$file_url")
	case "$status" in
		200)
			printf 'package already exists, skipping: %s\n' "$file_name"
			;;
		404)
			printf 'uploading %s\n' "$file_name"
			curl_auth --fail --location --silent --show-error \
				--header 'Content-Type: application/octet-stream' \
				--upload-file "$file_path" \
				"$file_url"
			;;
		*)
			printf 'could not check package %s (HTTP %s)\n' "$file_name" "$status" >&2
			exit 1
			;;
	esac
done

case "$create_release" in
	false|0|no)
		printf 'packages published; release creation disabled\n'
		exit 0
		;;
esac

response_file=$(mktemp "${TMPDIR:-/tmp}/aissh-release.XXXXXX")
trap 'rm -f -- "$response_file"' EXIT HUP INT TERM

status=$(curl_auth --silent --show-error \
	--output "$response_file" \
	--write-out '%{http_code}' \
	--request POST \
	--data-urlencode "name=aissh $version" \
	--data-urlencode "tag_name=$version" \
	--data-urlencode "description=Automated release for aissh $version." \
	"$release_url")

case "$status" in
	201)
		printf 'created GitLab release %s\n' "$version"
		;;
	409)
		printf 'GitLab release already exists: %s\n' "$version"
		;;
	*)
		printf 'failed to create GitLab release (HTTP %s):\n' "$status" >&2
		sed -n '1,20p' "$response_file" >&2
		exit 1
		;;
esac

for file_name in $files; do
	file_url="$package_url/$file_name"
	status=$(curl_auth --silent --show-error \
		--output "$response_file" \
		--write-out '%{http_code}' \
		--request POST \
		--data-urlencode "name=$file_name" \
		--data-urlencode "url=$file_url" \
		--data-urlencode "filepath=/$file_name" \
		--data-urlencode 'link_type=package' \
		"$release_url/$version/assets/links")

	case "$status" in
		201)
			printf 'created release link: %s\n' "$file_name"
			;;
		409)
			printf 'release link already exists: %s\n' "$file_name"
			;;
		*)
			printf 'failed to create release link for %s (HTTP %s):\n' "$file_name" "$status" >&2
			sed -n '1,20p' "$response_file" >&2
			exit 1
			;;
	esac
done

printf 'published aissh %s to GitLab\n' "$version"
