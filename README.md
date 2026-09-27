# ebookshelf

**Local EPUB library organizer — read the metadata, find the covers, group by author or series, export a CSV catalog. Go standard library only (`archive/zip` + `encoding/xml`).**

```
$ ebookshelf scan ~/Books
3 book(s) under /home/you/Books
 c Dune                                     Frank Herbert
 c Messiah                                  Frank Herbert
   Standalone                               Other Author

$ ebookshelf list ~/Books --by series
2 group(s), 3 book(s)

Dune (2)
  Dune #1
  Messiah #2

(no series) (1)
  Standalone

$ ebookshelf covers ~/Books --out ~/Covers
extracted 2 cover(s), 1 without

$ ebookshelf catalog ~/Books --csv ~/catalog.csv
wrote 3 row(s) to /home/you/catalog.csv
```

EPUB is a ZIP with an XML package document inside, which means every
librarian you own — Calibre, Kobo, Apple Books — is only half using it.
ebookshelf reads the file itself: the container points at the OPF, the
OPF's metadata gives you title, creators, publisher, date, language and
identifier, and the manifest tells you where the cover is.

## Why ebookshelf

- **No Calibre, no GUI, no re-import.** Point it at a directory and get
  a listing, a grouping, a cover dump and a CSV. It reads the files; it
  never rewrites them.
- **Metadata that actually exists in the file.** Title, multiple
  creators, language, publisher, date, `dc:identifier` — plus calibre's
  `calibre:series` / `calibre:series_index` and the generic
  `belongs-to-collection` marker, because that is where series actually
  live in real libraries.
- **Cover finding that handles real-world mess.** The
  `properties="cover-image"` manifest item first, then the legacy
  `meta name="cover"` id, then any image; and hrefs are accepted both
  relative to the OPF *and* rooted at the archive, because both spellings
  exist in the wild.
- **Grouping that respects people.** Authors are matched case- and
  spacing-insensitively, so "Frank Herbert" and "frank  herbert" land in
  one group. Series groups order by their index number, not
  alphabetically, and the "no series" bucket sorts last.
- **A catalog you can diff.** CSV with proper quoting (a title containing
  a comma and quotes survives the round trip), written by `encoding/csv`
  rather than string concatenation.
- **Robust scanning.** Broken EPUBs become a warning on stderr, not a
  crash; hidden directories are skipped; the walk continues.

## Usage

```sh
ebookshelf scan <dir> [--json]
ebookshelf list <dir> [--by author|series]
ebookshelf covers <dir> --out <dir>
ebookshelf catalog <dir> --csv <file>
```

Exit codes: 0 ok, 1 runtime error, 2 usage error.

## Development

```sh
gofmt -l .
go vet ./...
go test -race ./...
```

The tests build real EPUB archives with `archive/zip` — valid container
documents, package metadata, nested cover paths — and read them back
through the same code paths the CLI uses.

## License

MIT — see [LICENSE](LICENSE).
