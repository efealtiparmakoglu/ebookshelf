// Command ebookshelf — local EPUB library organizer: scan, group by
// author or series, extract covers, export a CSV catalog.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/efealtiparmakoglu/ebookshelf/shelf"
)

const version = "1.0.0"

const usage = `ebookshelf %s — local EPUB library organizer

usage:
  ebookshelf scan <dir> [--json]                 read every .epub under dir
  ebookshelf list <dir> [--by author|series]     grouped listing
  ebookshelf covers <dir> --out <dir>            extract cover images
  ebookshelf catalog <dir> --csv <file>          write the catalog
  ebookshelf -h | --help

exit codes: 0 ok, 1 runtime error, 2 usage error
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintf(os.Stderr, usage, version)
		return 2
	}
	command := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("ebookshelf-"+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "", "library directory")
	by := fs.String("by", "author", "group by: author or series")
	out := fs.String("out", "", "output directory for covers")
	csvPath := fs.String("csv", "", "catalog file to write")
	asJSON := fs.Bool("json", false, "JSON output")

	var positional []string
	for i := 0; i < len(rest); i++ {
		if strings.HasPrefix(rest[i], "-") {
			// flags that take a value may be given as --flag value
			switch rest[i] {
			case "--dir", "--by", "--out", "--csv":
				if i+1 < len(rest) && !strings.HasPrefix(rest[i+1], "-") {
					name := strings.TrimPrefix(rest[i], "--")
					_ = fs.Set(name, rest[i+1])
					i++
					continue
				}
			}
			continue
		}
		positional = append(positional, rest[i])
	}
	if err := fs.Parse(nil); err != nil {
		return 2
	}
	library := *dir
	if library == "" && len(positional) > 0 {
		library = positional[0]
	}
	if library == "" {
		fmt.Fprintln(os.Stderr, "ebookshelf: a library directory is required")
		fmt.Fprintf(os.Stderr, usage, version)
		return 2
	}

	books, problems := ebookshelf.Scan(library)
	for _, problem := range problems {
		fmt.Fprintf(os.Stderr, "ebookshelf: %v\n", problem)
	}

	switch command {
	case "scan":
		if *asJSON {
			return emitJSON(books)
		}
		fmt.Printf("%d book(s) under %s\n", len(books), library)
		for _, book := range books {
			marker := " "
			if book.HasCover {
				marker = "c"
			}
			authors := strings.Join(book.Authors, ", ")
			if authors == "" {
				authors = "(unknown)"
			}
			fmt.Printf(" %s %-40s %s\n", marker, truncate(book.Title, 40), authors)
		}
		return 0

	case "list":
		groups := ebookshelf.GroupByAuthor(books)
		if *by == "series" {
			groups = ebookshelf.GroupBySeries(books)
		} else if *by != "author" {
			fmt.Fprintf(os.Stderr, "ebookshelf: --by must be author or series\n")
			return 2
		}
		fmt.Printf("%d group(s), %d book(s)\n", len(groups), len(books))
		for _, group := range groups {
			fmt.Printf("\n%s (%d)\n", group.Name, len(group.Books))
			for _, book := range group.Books {
				index := ""
				if book.SeriesIndex != "" {
					index = " #" + book.SeriesIndex
				}
				fmt.Printf("  %s%s\n", book.Title, index)
			}
		}
		return 0

	case "covers":
		if *out == "" {
			fmt.Fprintln(os.Stderr, "ebookshelf: --out is required for covers")
			return 2
		}
		if err := os.MkdirAll(*out, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "ebookshelf: %v\n", err)
			return 1
		}
		extracted, missing := 0, 0
		for _, book := range books {
			base := strings.TrimSuffix(filepath.Base(book.Path), filepath.Ext(book.Path))
			target := filepath.Join(*out, sanitize(base)+coverExtension(book.CoverPath))
			if err := ebookshelf.ExtractCover(book.Path, target); err != nil {
				fmt.Fprintf(os.Stderr, "ebookshelf: %v\n", err)
				missing++
				continue
			}
			extracted++
		}
		fmt.Printf("extracted %d cover(s), %d without\n", extracted, missing)
		return 0

	case "catalog":
		if *csvPath == "" {
			fmt.Fprintln(os.Stderr, "ebookshelf: --csv is required for catalog")
			return 2
		}
		if err := ebookshelf.WriteCSV(*csvPath, books); err != nil {
			fmt.Fprintf(os.Stderr, "ebookshelf: %v\n", err)
			return 1
		}
		fmt.Printf("wrote %d row(s) to %s\n", len(books), *csvPath)
		return 0

	default:
		fmt.Fprintf(os.Stderr, "ebookshelf: unknown command '%s'\n", command)
		fmt.Fprintf(os.Stderr, usage, version)
		return 2
	}
}

func emitJSON(books []ebookshelf.Book) int {
	type entry struct {
		Title    string   `json:"title"`
		Authors  []string `json:"authors"`
		Series   string   `json:"series"`
		HasCover bool     `json:"has_cover"`
		Path     string   `json:"path"`
	}
	payload := make([]entry, 0, len(books))
	for _, book := range books {
		payload = append(payload, entry{book.Title, book.Authors, book.Series,
			book.HasCover, book.Path})
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ebookshelf: %v\n", err)
		return 1
	}
	fmt.Println(string(data))
	return 0
}

func truncate(text string, width int) string {
	if len(text) <= width {
		return text
	}
	if width <= 3 {
		return text[:width]
	}
	return text[:width-3] + "..."
}

func coverExtension(coverPath string) string {
	switch strings.ToLower(filepath.Ext(coverPath)) {
	case ".png":
		return ".png"
	case ".gif":
		return ".gif"
	case ".svg":
		return ".svg"
	default:
		return ".jpg"
	}
}

func sanitize(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return replacer.Replace(name)
}
