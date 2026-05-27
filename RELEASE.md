# Release process

How to cut a SolderDB release.

## Pre-flight

```bash
go test ./...
cd frontend && npx tsc --noEmit && npm run build && cd ..
```

All three must pass clean.

## Version bump

A major or minor release updates the version string in **three** places:

1. `frontend/package.json` — `version`
2. `wails.json` — `info.productVersion`
3. `frontend/src/App.tsx` — the sidebar footer `v0.X.Y`
4. `sdk/solderdb-js/package.json` — `version` (if SDK contract changed)

Also append a section to `CHANGELOG.md` summarizing what shipped.

Commit the bump:

```bash
git add -A
git commit -m "chore: bump v1.X.Y"
git tag v1.X.Y
```

## Build artifacts

### Windows executable (always)

```bash
wails build -clean -platform windows/amd64
```

Output: `build/bin/SolderDB.exe` (~13 MB unsigned).

### Windows installer (optional)

Install NSIS first: <https://nsis.sourceforge.io/Download>. Make sure `makensis` is on `PATH`.

```bash
wails build -clean -platform windows/amd64 -nsis
```

Output: `build/bin/SolderDB.exe` + `build/bin/SolderDB-amd64-installer.exe`.

### Cross-platform (optional)

Wails can cross-compile on Linux:

```bash
wails build -clean -platform linux/amd64
wails build -clean -platform darwin/universal   # macOS only — signing needs Apple hardware
```

## Smoke test

Launch the built binary on a clean machine (or wipe `%APPDATA%\SolderDB` first):

1. **Splash plays** — copper rings, wordmark, boot log
2. **Register screen appears** — register a new admin
3. **Dashboard loads** — Stats card populated, hero greeting visible, no console errors
4. **Compact button works** — even with one SSTable, no error
5. **Create a collection + a record** — appears in the Lifecycle visualizer's SSTable lane after flush
6. **Cmd-K palette opens** and jumps work
7. **Dark mode toggle** in Profile flips theme cleanly
8. **Docs view renders** — index, then click a few links

If any of those fail, hold the release.

## Publish

```bash
git push --follow-tags origin main
gh release create v1.X.Y \
  --title "v1.X.Y" \
  --notes-file <(awk '/^## \[/{if(p)exit; p=1} p' CHANGELOG.md) \
  build/bin/SolderDB.exe \
  build/bin/SolderDB-amd64-installer.exe
```

The `awk` snippet extracts the latest CHANGELOG section as release notes.

## SDK publish (optional)

```bash
cd sdk/solderdb-js
npm publish --access public
```

(Requires npm auth.)

## After publish

- Verify the GitHub release page renders the README's relative links correctly
- Tweet / post / share — drive eyeballs while it's hot
- Open issues for anything you noticed during smoke-test that didn't block release
