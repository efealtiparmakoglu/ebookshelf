package ebookshelf

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type epubSpec struct {
	title      string
	creators   []string
	series     string
	seriesIdx  string
	language   string
	publisher  string
	date       string
	identifier string
	coverFile  string // name inside the zip, "" for no cover
	coverProp  string // "cover-image" when the manifest flags it
}

// buildEPUB writes a minimal but valid EPUB to disk.
func buildEPUB(t *testing.T, dir, name string, spec epubSpec) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	write := func(entry, content string) {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	write("mimetype", "application/epub+zip")
	write("META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`)

	creators := ""
	for i, creator := range spec.creators {
		if i > 0 {
			creators += " "
		}
		creators += "<dc:creator>" + creator + "</dc:creator>"
	}
	meta := ""
	if spec.series != "" {
		meta += `<meta name="calibre:series" content="` + spec.series + `"/>`
	}
	if spec.seriesIdx != "" {
		meta += `<meta name="calibre:series_index" content="` + spec.seriesIdx + `"/>`
	}
	items := ""
	if spec.coverFile != "" {
		properties := ""
		if spec.coverProp == "cover-image" {
			properties = ` properties="cover-image"`
		}
		items += `<item id="cover" href="` + spec.coverFile + `" media-type="image/jpeg"` + properties + `/>`
	}
	write("OEBPS/content.opf", `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>`+spec.title+`</dc:title>
    `+creators+`
    <dc:language>`+spec.language+`</dc:language>
    <dc:publisher>`+spec.publisher+`</dc:publisher>
    <dc:date>`+spec.date+`</dc:date>
    <dc:identifier id="pub-id">`+spec.identifier+`</dc:identifier>
    `+meta+`
  </metadata>
  <manifest>
    <item id="text" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    `+items+`
  </manifest>
  <spine><itemref idref="text"/></spine>
</package>`)
	write("OEBPS/chapter1.xhtml", "<html><body>text</body></html>")
	if spec.coverFile != "" {
		write(spec.coverFile, "\xff\xd8\xff\xe0 fake jpeg bytes")
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadEPUBMetadata(t *testing.T) {
	dir := t.TempDir()
	path := buildEPUB(t, dir, "dune.epub", epubSpec{
		title:      "Dune",
		creators:   []string{"Frank Herbert"},
		language:   "en",
		publisher:  "Ace",
		date:       "1965-08-01",
		identifier: "urn:isbn:9780441013593",
		coverFile:  "OEBPS/cover.jpg",
		coverProp:  "cover-image",
	})
	book, err := ReadEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "Dune" {
		t.Errorf("title = %q", book.Title)
	}
	if len(book.Authors) != 1 || book.Authors[0] != "Frank Herbert" {
		t.Errorf("authors = %v", book.Authors)
	}
	if book.Language != "en" || book.Publisher != "Ace" || book.Date != "1965-08-01" {
		t.Errorf("metadata = %+v", book)
	}
	if book.Identifier != "urn:isbn:9780441013593" {
		t.Errorf("identifier = %q", book.Identifier)
	}
	if !book.HasCover || book.CoverPath != "OEBPS/cover.jpg" {
		t.Errorf("cover = %q (%v)", book.CoverPath, book.HasCover)
	}
}

func TestSeriesMetadata(t *testing.T) {
	dir := t.TempDir()
	path := buildEPUB(t, dir, "messiah.epub", epubSpec{
		title: "Messiah", creators: []string{"Frank Herbert"},
		series: "Dune", seriesIdx: "2", coverFile: "OEBPS/images/cover.jpg",
	})
	book, err := ReadEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if book.Series != "Dune" || book.SeriesIndex != "2" {
		t.Errorf("series = %q #%q", book.Series, book.SeriesIndex)
	}
	// a nested cover href must resolve against the OPF directory
	if book.CoverPath != "OEBPS/images/cover.jpg" {
		t.Errorf("cover path = %q", book.CoverPath)
	}
}

func TestMultipleCreators(t *testing.T) {
	dir := t.TempDir()
	path := buildEPUB(t, dir, "team.epub", epubSpec{
		title: "Shared Work", creators: []string{"Ada Lovelace", "Charles Babbage"},
	})
	book, err := ReadEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Authors) != 2 {
		t.Fatalf("authors = %v", book.Authors)
	}
	if PrimaryAuthor(book) != "Ada Lovelace" {
		t.Errorf("primary = %q", PrimaryAuthor(book))
	}
}

func TestMissingAuthor(t *testing.T) {
	dir := t.TempDir()
	path := buildEPUB(t, dir, "anon.epub", epubSpec{title: "Anonymous"})
	book, err := ReadEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if PrimaryAuthor(book) != "(unknown)" {
		t.Errorf("primary = %q", PrimaryAuthor(book))
	}
}

func TestBookWithoutCover(t *testing.T) {
	dir := t.TempDir()
	path := buildEPUB(t, dir, "plain.epub", epubSpec{title: "No Cover"})
	book, err := ReadEPUB(path)
	if err != nil {
		t.Fatal(err)
	}
	if book.HasCover {
		t.Error("reported a cover that does not exist")
	}
}

func TestNotAnEPUB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.epub")
	if err := os.WriteFile(path, []byte("this is not a zip file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEPUB(path); err == nil {
		t.Error("expected an error for a non-zip file")
	}
}

func TestScanFindsBooksAndReportsProblems(t *testing.T) {
	dir := t.TempDir()
	buildEPUB(t, dir, "a.epub", epubSpec{title: "Alpha", creators: []string{"Ann"}})
	buildEPUB(t, dir, "nested/b.epub", epubSpec{title: "Beta", creators: []string{"Bob"}})
	os.WriteFile(filepath.Join(dir, "broken.epub"), []byte("junk"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o644)

	books, problems := Scan(dir)
	if len(books) != 2 {
		t.Fatalf("books = %d", len(books))
	}
	if books[0].Title != "Alpha" || books[1].Title != "Beta" {
		t.Errorf("books not sorted by title: %v", books)
	}
	if len(problems) != 1 {
		t.Errorf("problems = %v", problems)
	}
}

func TestGroupByAuthor(t *testing.T) {
	books := []Book{
		{Title: "B", Authors: []string{"Frank Herbert"}},
		{Title: "A", Authors: []string{"frank  herbert"}}, // same author, messy spacing
		{Title: "C", Authors: []string{"Someone Else"}},
	}
	groups := GroupByAuthor(books)
	if len(groups) != 2 {
		t.Fatalf("groups = %d: %+v", len(groups), groups)
	}
	if len(groups[0].Books) != 2 {
		t.Errorf("expected the two Herberts together, got %+v", groups[0])
	}
	if groups[0].Books[0].Title != "A" {
		t.Errorf("group not sorted: %+v", groups[0].Books)
	}
}

func TestGroupBySeriesOrdersByIndex(t *testing.T) {
	books := []Book{
		{Title: "Third", Series: "Dune", SeriesIndex: "3"},
		{Title: "First", Series: "Dune", SeriesIndex: "1"},
		{Title: "Second", Series: "Dune", SeriesIndex: "2"},
		{Title: "Standalone"},
	}
	groups := GroupBySeries(books)
	if len(groups) != 2 {
		t.Fatalf("groups = %d", len(groups))
	}
	dune := groups[0]
	if dune.Name != "Dune" {
		t.Fatalf("first group = %q", dune.Name)
	}
	if dune.Books[0].Title != "First" || dune.Books[2].Title != "Third" {
		t.Errorf("series order wrong: %+v", dune.Books)
	}
}

func TestGroupBySeriesNonNumericIndex(t *testing.T) {
	books := []Book{{Title: "X", Series: "S", SeriesIndex: "one"}}
	groups := GroupBySeries(books)
	if len(groups) != 1 || groups[0].Books[0].Title != "X" {
		t.Errorf("groups = %+v", groups)
	}
}

func TestWriteCSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.csv")
	books := []Book{
		{Title: "Dune", Authors: []string{"Frank Herbert"}, Series: "Dune",
			SeriesIndex: "1", Language: "en", Publisher: "Ace", Date: "1965",
			Identifier: "urn:isbn:1", HasCover: true, Path: "/lib/dune.epub"},
		{Title: "Comma, and \"quotes\"", Authors: []string{"A; B"}, Path: "/lib/x.epub"},
	}
	if err := WriteCSV(path, books); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "title,authors,series") {
		t.Errorf("header = %q", strings.SplitN(text, "\n", 2)[0])
	}
	if !strings.Contains(text, `"Comma, and ""quotes"""`) {
		t.Errorf("quoting not applied:\n%s", text)
	}
	if !strings.Contains(text, "urn:isbn:1") {
		t.Error("identifier missing")
	}
}

func TestExtractCover(t *testing.T) {
	dir := t.TempDir()
	book := buildEPUB(t, dir, "dune.epub", epubSpec{
		title: "Dune", coverFile: "OEBPS/cover.jpg", coverProp: "cover-image"})
	target := filepath.Join(dir, "out.jpg")
	if err := ExtractCover(book, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "fake jpeg bytes") {
		t.Errorf("cover content = %q", data)
	}
}

func TestExtractCoverWithoutCover(t *testing.T) {
	dir := t.TempDir()
	book := buildEPUB(t, dir, "plain.epub", epubSpec{title: "No Cover"})
	err := ExtractCover(book, filepath.Join(dir, "out.jpg"))
	if err == nil {
		t.Error("expected an error when there is no cover")
	}
	if !strings.Contains(err.Error(), "no cover") {
		t.Errorf("error = %v", err)
	}
}

func TestSortBooksByTitleThenAuthor(t *testing.T) {
	books := []Book{
		{Title: "B", Authors: []string{"Zoe"}},
		{Title: "A", Authors: []string{"Ann"}},
		{Title: "A", Authors: []string{"Bob"}},
	}
	SortBooks(books)
	if books[0].Title != "A" || books[0].Authors[0] != "Ann" {
		t.Errorf("sorted = %+v", books)
	}
	if books[1].Authors[0] != "Bob" {
		t.Errorf("second = %+v", books[1])
	}
}
