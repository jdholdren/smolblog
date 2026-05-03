# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

- `make test` — run the full test suite (`go test ./...`)
- `make example` — run the dev server against `./example/smolmanifest.json` on port 4444
- `go run . -manifest=<path>` — run against an arbitrary manifest
- `go run . -manifest=<path> -output=<dir>` — start the server, then shell out to `wget` to crawl it into static files under `./build` (note: the `-output` flag value itself is currently unused; output always lands in `./build`)

`wget` must be installed on the host for static export to work.

## Architecture

This is a single-binary site generator. All logic lives in `main.go` (~300 lines). The model:

- A **Manifest** (JSON) declares two things: a list of HTML template files (`layouts`) and a map of URL paths to **Routes**.
- A **Route** is either a static file passthrough (`static_path` + `content_type`) or a templated page (`template` name + arbitrary `args` map).
- The HTTP handler **re-reads the manifest and re-parses all layouts on every GET request**. This is the central design choice — it trades throughput for zero-restart iteration during authoring. Don't add caching without preserving that property (or making it opt-in).
- All paths inside the manifest (layouts, static files, markdown sources) are resolved **relative to the manifest file's directory**, not the cwd. The handler stores `manifestDir` for this purpose.
- The `renderMarkdown` template func (registered in `templateFuncs`) reads a markdown file relative to the manifest dir and returns rendered HTML via goldmark. It **panics on error** — template execution will surface it as a 500.
- Static export works by booting the server and pointing `wget -r` at it; there is no separate "build" code path. Whatever the server serves is what gets exported.

Templates receive a `routeArgs{Path, Args}` struct, so templates can reference both the current URL and the manifest-supplied data.

## Known issues

`main.go` has a documented bug: if template execution fails partway through, output has already been flushed, so the 500 status write is a no-op and the client sees a 200 with truncated HTML.
