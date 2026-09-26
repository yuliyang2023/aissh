#!/bin/bash
# Run from any directory. Requires Xcode, an installed iOS simulator, and Go.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p .cache
IOS_DESTINATION="${IOS_DESTINATION:-platform=iOS Simulator,name=iPhone 17 Pro}"
GO_BIN="${GO_BIN:-go}"
"$GO_BIN" build -o .cache/ios-ssh-fixture ./scripts/ios-ssh-fixture
.cache/ios-ssh-fixture > .cache/ios-ssh-fixture.log 2>&1 &
fixture_pid=$!
trap 'kill "$fixture_pid" 2>/dev/null || true' EXIT
sleep 1
if ! kill -0 "$fixture_pid" 2>/dev/null; then
  cat .cache/ios-ssh-fixture.log
  exit 1
fi
xcodebuild -project ios/WRDAISSH.xcodeproj -scheme WRDAISSH \
  -configuration Debug -destination "$IOS_DESTINATION" \
  -derivedDataPath .cache/ios-build -clonedSourcePackagesDirPath .cache/ios-packages \
  -disableAutomaticPackageResolution -skipPackageUpdates -skipPackagePluginValidation \
  CODE_SIGN_IDENTITY=- CODE_SIGNING_ALLOWED=YES build-for-testing
# Test-action variables do not expand arbitrary Xcode build settings. Set the
# fixture flag in the generated test plan, without changing the shared scheme.
python3 - <<'PY'
import pathlib, plistlib
folder = pathlib.Path('.cache/ios-build/Build/Products')
source = max(folder.glob('WRDAISSH_*.xctestrun'), key=lambda p: p.stat().st_mtime)
plan = plistlib.loads(source.read_bytes())
for target in plan.values():
    if isinstance(target, dict) and 'TestBundlePath' in target:
        target.setdefault('EnvironmentVariables', {})['WRD_SSH_INTEGRATION'] = '1'
(folder / 'PhoneIntegration.xctestrun').write_bytes(plistlib.dumps(plan))
PY
xcodebuild test-without-building \
  -xctestrun .cache/ios-build/Build/Products/PhoneIntegration.xctestrun \
  -destination "$IOS_DESTINATION" -parallel-testing-enabled NO "$@"
