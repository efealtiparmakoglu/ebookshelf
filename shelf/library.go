package ebookshelf

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Scan walks a directory and reads every .epub it finds.
func Scan(root string) ([]Book, []error) {
	var books []Book
	var problems []error

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %v", path, err))
			return nil
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".epub") {
			return nil
		}
		book, err := ReadEPUB(path)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %v", path, err))
			return nil
		}
		books = append(books, book)
		return nil
	})
	if err != nil {
		return books, append(problems, err)
	}
	SortBooks(books)
	return books, problems
}

// Group is a set of books under one author or series heading.
type Group struct {
	Name  string
	Books []Book
}

// GroupByAuthor groups books, merging the same author under one heading
// regardless of how the name is cased or spaced.
func GroupByAuthor(books []Book) []Group {
	index := map[string]*Group{}
	var order []string
	for _, book := range books {
		key := normalizeKey(PrimaryAuthor(book))
		group, ok := index[key]
		if !ok {
			group = &Group{Name: PrimaryAuthor(book)}
			index[key] = group
			order = append(order, key)
		}
		group.Books = append(group.Books, book)
	}
	return finishGroups(index, order)
}

// noSeriesName is the heading for books without a series.
const noSeriesName = "(no series)"

// GroupBySeries groups books that declare a series; the rest land in a
// final "no series" group so nothing disappears from the listing.
func GroupBySeries(books []Book) []Group {
	index := map[string]*Group{}
	var order []string
	for _, book := range books {
		name := book.Series
		key := normalizeKey(name)
		if name == "" {
			name = noSeriesName
			key = noSeriesName
		}
		group, ok := index[key]
		if !ok {
			group = &Group{Name: name}
			index[key] = group
			order = append(order, key)
		}
		group.Books = append(group.Books, book)
	}
	groups := finishGroups(index, order)
	for _, group := range groups {
		sort.SliceStable(group.Books, func(i, j int) bool {
			return seriesIndex(group.Books[i]) < seriesIndex(group.Books[j])
		})
	}
	// the catch-all group reads better last than first
	sort.SliceStable(groups, func(i, j int) bool {
		if (groups[i].Name == noSeriesName) != (groups[j].Name == noSeriesName) {
			return groups[j].Name == noSeriesName
		}
		return strings.ToLower(groups[i].Name) < strings.ToLower(groups[j].Name)
	})
	return groups
}

func finishGroups(index map[string]*Group, order []string) []Group {
	groups := make([]Group, 0, len(order))
	for _, key := range order {
		group := index[key]
		SortBooks(group.Books)
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return strings.ToLower(groups[i].Name) < strings.ToLower(groups[j].Name)
	})
	return groups
}

func seriesIndex(book Book) float64 {
	if book.SeriesIndex == "" {
		return 0
	}
	value, err := strconv.ParseFloat(book.SeriesIndex, 64)
	if err != nil {
		return 0
	}
	return value
}

func normalizeKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// WriteCSV writes the catalog with one row per book.
func WriteCSV(path string, books []Book) error {
	handle, err := os.Create(path)
	if err != nil {
		return err
	}
	defer handle.Close()

	writer := csv.NewWriter(handle)
	defer writer.Flush()

	if err := writer.Write([]string{"title", "authors", "series", "series_index",
		"language", "publisher", "date", "identifier", "has_cover", "path"}); err != nil {
		return err
	}
	for _, book := range books {
		record := []string{
			book.Title,
			strings.Join(book.Authors, "; "),
			book.Series,
			book.SeriesIndex,
			book.Language,
			book.Publisher,
			book.Date,
			book.Identifier,
			strconv.FormatBool(book.HasCover),
			book.Path,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
