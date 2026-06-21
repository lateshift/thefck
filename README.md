# thefck

Ladies and gentlemen, may I present thefck: a simple tool for finding duplicate files and browsing the indexed results through an embedded Vue 3 app.

A single binary containing all the fun you can think of. And since it’s written in Go, it might even compile and run on your system.

## What?

`thefck` indexes files in a directory tree, stores file metadata in a bbolt database, flags duplicate content, and serves a bundled Vue 3 SPA for browsing the index.

## Why?

Because most duplicate-file finders are a giant **clusterfuck**: gazillions of esoteric options, wrapped in build requirements that make your package manager weep.

## Required & mildly funny AI-generated image

<img width="800" alt="yomama" src="https://github.com/user-attachments/assets/022cf840-849b-426d-831c-3c24cbcb0b14" />

### Let’s drop some rhymes.

> Yo, I crawl the tree, I hash the block,<br>
> I find the dupe and I make it talk.<br>
> bbolt in the back, Vue in the front,<br>
> one binary doing the whole damn stunt.<br>
>  
> **Mic drop.**


## What Gets Indexed

Each indexed file stores:

- File date
- File size
- Absolute path
- Filename
- Checksum
- Duplicate status

Checksums use `github.com/cespare/xxhash/v2`. The index is stored in bbolt.

## Requirements

- Go 1.25 or newer
- Node.js and npm, only when rebuilding the SPA

## Scan Files

Scan the current directory:

```sh
go run ./src
```

Scan a specific directory:

```sh
go run ./src /path/to/files
```

Use a custom database:

```sh
go run ./src --db /path/to/checksums.db /path/to/files
```

Report new, changed, and missing files while scanning:

```sh
go run ./src --report-changes /path/to/files
```

Duplicate reporting is enabled by default. Disable it with:

```sh
go run ./src --report-duplicates=false /path/to/files
```

## Serve The SPA

Start the bundled web UI:

```sh
go run ./src serve --db checksums.db --addr 127.0.0.1:8080
```

Then open:

```text
http://127.0.0.1:8080
```

The API is available at:

```text
http://127.0.0.1:8080/api/files
```

The SPA table is searchable and sortable. It starts filtered to duplicate files, shows the active filters above the table, and paginates at 200 files per page.

## Build A Binary

Build the command-line app:

```sh
go build -o thefck ./src
```

Run it:

```sh
./thefck --report-changes /path/to/files
./thefck serve --db checksums.db --addr 127.0.0.1:8080
```

## Rebuild The SPA

The SPA source lives in `js/`. It builds into `src/web/dist/`, which is embedded into the Go binary.

Install dependencies:

```sh
cd js
npm install
```

Build the SPA:

```sh
npm run build
```

Then rebuild the Go binary:

```sh
cd ..
go build -o thefck ./src
```

## Notes

- `js/node_modules/` is ignored.
- `js/package-lock.json` is intentionally kept for reproducible frontend installs.
- Generated SPA assets in `src/web/dist/` are part of the Go embed flow.
- The database file is skipped during scans when it lives inside the scanned directory.
