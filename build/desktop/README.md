# Application icon

The editable artwork is `frontend/public/app-icon.svg`. The simplified
`frontend/public/favicon.svg` keeps the prompt and AI sparkle legible at 16 px.

Run `python3 scripts/generate-icons.py` after editing the artwork. The export
script needs Pillow and Node.js with `sharp`; `NODE` and `SHARP_MODULE` can point
to existing installations. Normal application builds use the generated files
directly and do not require those dependencies.

- `app-icon.png`: 1024 px transparent, rounded artwork.
- `app-icon.icns`: macOS icon with padding and shadow, copied into the app bundle
  by `scripts/build-desktop.sh` and referenced by `Info.plist`.
- `app-icon.ico`: multiple resolutions for Windows packaging; not currently
  embedded by the Windows build script.
- The generator also updates the iOS asset catalog with an opaque 1024 px square
  icon, plus the browser favicon and Apple touch icon in `frontend/public`.
