# AGENTS.md

Notes for future coding agents working in this repository.

## Project Shape

- Go command source lives in `src/`.
- Vue 3 SPA source lives in `js/`.
- Built SPA assets live in `src/web/dist/` and are embedded into the Go binary by `src/web.go`.
- The default index database is `checksums.db`.

## Build And Verify

Use these checks after meaningful changes:

```sh
go test ./...
```

```sh
cd js
npm run build
```

If the SPA source changes, run `npm run build` so `src/web/dist/` stays in sync with the embedded binary assets.

## Running Locally

Scan:

```sh
go run ./src --report-changes /path/to/files
```

Serve:

```sh
go run ./src serve --db checksums.db --addr 127.0.0.1:8080
```

## Code Guidelines

- Keep code files under 500 lines. This applies to source code, not generated dependency metadata such as `js/package-lock.json`.
- Keep the Go entrypoint small. Put scanner, store, web, and utility code in focused files.
- Keep comments useful and explanatory. Explain why something exists, not what every obvious line does.
- Keep the implementation simple. Avoid new abstractions unless they remove real repetition or clarify ownership.
- Do not edit files under `js/node_modules/`.
- Do not hand-edit built SPA bundles unless there is no alternative. Change `js/src/*`, then run `npm run build`.

## Important Implementation Details

- File metadata is persisted as `FileRecord` JSON in bbolt.
- Duplicate lookup is maintained through checksum buckets.
- Legacy index rows may contain relative paths; store reads normalize those to absolute paths for API display.
- The web layer uses `github.com/go-chi/chi/v5`.
- The SPA uses Vue 3 and `@tanstack/vue-table`.

## Dependency Notes

- Go dependencies are tracked in `go.mod` and `go.sum`.
- Frontend dependencies are pinned in `js/package.json` and locked in `js/package-lock.json`.
- `js/node_modules/` should remain ignored.
