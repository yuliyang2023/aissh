#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"
go_command=${GO:-go}
npm_command=${NPM:-npm}
version=${VERSION:-dev}
desktop_tags=desktop,production
if [ -n "${AISSH_DESKTOP_TAGS:-}" ]; then
  desktop_tags="$desktop_tags,$AISSH_DESKTOP_TAGS"
fi

if [ "${AISSH_SKIP_INSTALL:-0}" != 1 ]; then
  (cd frontend && "$npm_command" ci --no-audit --no-fund)
fi
(cd frontend && "$npm_command" run build)
mkdir -p bin dist/desktop
"$go_command" build -trimpath -ldflags "-s -w -X main.version=$version" -o bin/aissh ./cmd/aissh

case "$(uname -s)" in
 Darwin)
   bundle="dist/desktop/WRD AI SSH.app"
   mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
   "$go_command" build -tags "$desktop_tags" -trimpath -ldflags "-s -w" -o "$bundle/Contents/MacOS/aissh-desktop" ./cmd/aissh-desktop
   cp bin/aissh "$bundle/Contents/Resources/aissh"
   cp build/desktop/app-icon.icns "$bundle/Contents/Resources/app-icon.icns"
   cp build/desktop/Info.plist "$bundle/Contents/Info.plist"
   codesign --force --deep --sign - "$bundle"
   printf 'Built: %s/%s\n' "$project_dir" "$bundle"
   ;;
 *)
   "$go_command" build -tags "$desktop_tags" -trimpath -ldflags "-s -w" -o dist/desktop/aissh-desktop ./cmd/aissh-desktop
   cp bin/aissh dist/desktop/aissh
   printf 'Built: %s/dist/desktop/aissh-desktop\n' "$project_dir"
   ;;
esac
