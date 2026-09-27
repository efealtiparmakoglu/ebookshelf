// Package shelf reads EPUB files: metadata, cover images, grouping
// and CSV export. Standard library only (archive/zip + encoding/xml).
package ebookshelf

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

// Book is the metadata ebookshelf cares about.
type Book struct {
	Path        string
	Title       string
	Authors     []string
	Series      string
	SeriesIndex string
	Language    string
	Publisher   string
	Date        string
	Identifier  string
	CoverPath   string // path inside the EPUB, e.g. OEBPS/cover.jpg
	HasCover    bool
}

var ErrNotEPUB = errors.New("not an EPUB file")

// OPF is the subset of the package document we read.
type OPF struct {
	XMLName  xml.Name `xml:"package"`
	Metadata struct {
		Title      []string `xml:"title"`
		Creator    []string `xml:"creator"`
		Language   []string `xml:"language"`
		Publisher  []string `xml:"publisher"`
		Date       []string `xml:"date"`
		Identifier []struct {
			Value string `xml:",chardata"`
			ID    string `xml:"id,attr"`
		} `xml:"identifier"`
		Meta []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		ItemRef []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

type containerXML struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

// ReadEPUB opens an EPUB and extracts its metadata.
func ReadEPUB(filePath string) (Book, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return Book{}, fmt.Errorf("%w: %v", ErrNotEPUB, err)
	}
	defer reader.Close()

	opfPath, err := findOPFPath(&reader.Reader)
	if err != nil {
		return Book{}, err
	}
	opf, err := parseOPF(readZipFile(&reader.Reader, opfPath))
	if err != nil {
		return Book{}, err
	}

	book := Book{Path: filePath}
	if len(opf.Metadata.Title) > 0 {
		book.Title = strings.TrimSpace(opf.Metadata.Title[0])
	}
	book.Authors = cleanList(opf.Metadata.Creator)
	book.Language = firstOr(opf.Metadata.Language, "")
	book.Publisher = firstOr(opf.Metadata.Publisher, "")
	book.Date = firstOr(opf.Metadata.Date, "")
	book.Identifier = findIdentifier(&opf)

	// series: calibre's meta tags, or a BelongsTo-collection marker
	for _, meta := range opf.Metadata.Meta {
		switch meta.Name {
		case "calibre:series":
			book.Series = meta.Content
		case "calibre:series_index":
			book.SeriesIndex = meta.Content
		case "belongs-to-collection":
			book.Series = meta.Content
		}
	}

	book.CoverPath, book.HasCover = findCover(&reader.Reader, opfPath, &opf)
	return book, nil
}

func findOPFPath(reader *zip.Reader) (string, error) {
	data := readZipFile(reader, "META-INF/container.xml")
	if data == nil {
		// tolerate a missing container by looking for any .opf
		for _, file := range reader.File {
			if strings.HasSuffix(file.Name, ".opf") {
				return file.Name, nil
			}
		}
		return "", errors.New("no META-INF/container.xml and no .opf inside")
	}
	var container containerXML
	if err := xml.Unmarshal(data, &container); err != nil {
		return "", fmt.Errorf("container.xml: %w", err)
	}
	if len(container.Rootfiles) == 0 {
		return "", errors.New("container.xml lists no rootfile")
	}
	return container.Rootfiles[0].FullPath, nil
}

func parseOPF(data []byte) (OPF, error) {
	var opf OPF
	if err := xml.Unmarshal(data, &opf); err != nil {
		return opf, fmt.Errorf("package document: %w", err)
	}
	return opf, nil
}

// findCover prefers the manifest item flagged cover-image, then the
// calibre cover meta tag, then a file that simply looks like a cover.
func findCover(reader *zip.Reader, opfPath string, opf *OPF) (string, bool) {
	var metaCoverID string
	for _, meta := range opf.Metadata.Meta {
		if meta.Name == "cover" {
			metaCoverID = meta.Content
		}
	}
	base := path.Dir(opfPath)
	for _, item := range opf.Manifest.Items {
		if strings.Contains(item.Properties, "cover-image") {
			return resolveHref(reader, base, item.Href), true
		}
		if metaCoverID != "" && item.ID == metaCoverID {
			return resolveHref(reader, base, item.Href), true
		}
	}
	for _, item := range opf.Manifest.Items {
		if strings.HasPrefix(item.MediaType, "image/") {
			return resolveHref(reader, base, item.Href), true
		}
	}
	return "", false
}

func joinEPUB(base, href string) string {
	href = strings.TrimPrefix(href, "/")
	if base == "." || base == "" {
		return href
	}
	return path.Join(base, href)
}

// resolveHref maps a manifest href to a real archive entry. Hrefs are
// normally relative to the OPF's directory, but some writers emit them
// rooted at the archive instead — both are accepted.
func resolveHref(reader *zip.Reader, base, href string) string {
	joined := joinEPUB(base, href)
	if zipHas(reader, joined) {
		return joined
	}
	if zipHas(reader, href) {
		return href
	}
	return joined
}

func zipHas(reader *zip.Reader, name string) bool {
	for _, file := range reader.File {
		if file.Name == name {
			return true
		}
	}
	return false
}

func findIdentifier(opf *OPF) string {
	for _, id := range opf.Metadata.Identifier {
		if strings.EqualFold(id.ID, "calibre:id") || id.ID == "" {
			return strings.TrimSpace(id.Value)
		}
	}
	if len(opf.Metadata.Identifier) > 0 {
		return strings.TrimSpace(opf.Metadata.Identifier[0].Value)
	}
	return ""
}

func readZipFile(reader *zip.Reader, name string) []byte {
	for _, file := range reader.File {
		if file.Name == name {
			handle, err := file.Open()
			if err != nil {
				return nil
			}
			defer handle.Close()
			data, err := io.ReadAll(handle)
			if err != nil {
				return nil
			}
			return data
		}
	}
	return nil
}

// ExtractCover writes the cover image of an EPUB to a file.
func ExtractCover(filePath string, destination string) error {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	opfPath, err := findOPFPath(&reader.Reader)
	if err != nil {
		return err
	}
	opf, err := parseOPF(readZipFile(&reader.Reader, opfPath))
	if err != nil {
		return err
	}
	coverPath, ok := findCover(&reader.Reader, opfPath, &opf)
	if !ok {
		return fmt.Errorf("%s has no cover image", filePath)
	}
	for _, file := range reader.File {
		if file.Name == coverPath {
			source, err := file.Open()
			if err != nil {
				return err
			}
			defer source.Close()
			target, err := os.Create(destination)
			if err != nil {
				return err
			}
			defer target.Close()
			_, err = io.Copy(target, source)
			return err
		}
	}
	return fmt.Errorf("cover %s missing from the archive", coverPath)
}

func cleanList(values []string) []string {
	var out []string
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func firstOr(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.TrimSpace(values[0])
}

// SortBooks orders books by title, then author.
func SortBooks(books []Book) {
	sort.Slice(books, func(i, j int) bool {
		if books[i].Title != books[j].Title {
			return books[i].Title < books[j].Title
		}
		return PrimaryAuthor(books[i]) < PrimaryAuthor(books[j])
	})
}

// PrimaryAuthor is the first author, or "(unknown)".
func PrimaryAuthor(book Book) string {
	if len(book.Authors) == 0 {
		return "(unknown)"
	}
	return book.Authors[0]
}
