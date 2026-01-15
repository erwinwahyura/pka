package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/erwar/pka/internal/book"
)

type OpenLibraryClient struct {
	client  *http.Client
	baseURL string
}

func NewOpenLibraryClient() *OpenLibraryClient {
	return &OpenLibraryClient{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		baseURL: "https://openlibrary.org",
	}
}

type olWork struct {
	Title       string   `json:"title"`
	Authors     []olRef  `json:"authors"`
	Description any      `json:"description"` // can be string or {type, value}
	Subjects    []string `json:"subjects"`
	Covers      []int    `json:"covers"`
}

type olRef struct {
	Author olAuthorRef `json:"author"`
}

type olAuthorRef struct {
	Key string `json:"key"`
}

type olAuthor struct {
	Name string `json:"name"`
}

type olEdition struct {
	Title      string   `json:"title"`
	Authors    []olRef2 `json:"authors"`
	ISBN10     []string `json:"isbn_10"`
	ISBN13     []string `json:"isbn_13"`
	Publishers []string `json:"publishers"`
	Works      []olRef2 `json:"works"`
}

type olRef2 struct {
	Key string `json:"key"`
}

type olSearchResult struct {
	NumFound int              `json:"numFound"`
	Docs     []olSearchDoc    `json:"docs"`
}

type olSearchDoc struct {
	Key            string   `json:"key"`           // e.g., "/works/OL123W"
	Title          string   `json:"title"`
	AuthorName     []string `json:"author_name"`
	FirstPublishYear int    `json:"first_publish_year"`
	ISBN           []string `json:"isbn"`
	Subject        []string `json:"subject"`
	CoverI         int      `json:"cover_i"`
}

// FetchByISBN fetches book metadata by ISBN
func (c *OpenLibraryClient) FetchByISBN(ctx context.Context, isbn string) (*book.Book, error) {
	isbn = normalizeISBN(isbn)

	url := fmt.Sprintf("%s/isbn/%s.json", c.baseURL, isbn)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch ISBN: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("ISBN not found: %s", isbn)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("OpenLibrary returned status %d", resp.StatusCode)
	}

	var edition olEdition
	if err := json.NewDecoder(resp.Body).Decode(&edition); err != nil {
		return nil, fmt.Errorf("decode edition: %w", err)
	}

	// Get work details for description
	var description string
	var subjects []string
	if len(edition.Works) > 0 {
		work, err := c.fetchWork(ctx, edition.Works[0].Key)
		if err == nil {
			description = extractDescription(work.Description)
			subjects = work.Subjects
		}
	}

	// Get author names
	var authorNames []string
	for _, a := range edition.Authors {
		author, err := c.fetchAuthor(ctx, a.Key)
		if err == nil {
			authorNames = append(authorNames, author.Name)
		}
	}

	// Build tags from subjects (limit to 5)
	var tags []string
	for i, s := range subjects {
		if i >= 5 {
			break
		}
		tags = append(tags, s)
	}

	// Prefer ISBN-13
	finalISBN := isbn
	if len(edition.ISBN13) > 0 {
		finalISBN = edition.ISBN13[0]
	} else if len(edition.ISBN10) > 0 {
		finalISBN = edition.ISBN10[0]
	}

	return &book.Book{
		Title:       edition.Title,
		Author:      strings.Join(authorNames, ", "),
		ISBN:        finalISBN,
		Description: truncate(description, 500),
		Tags:        tags,
		Status:      book.StatusWantToRead,
		DateAdded:   time.Now(),
	}, nil
}

// Search searches OpenLibrary for books matching a query
func (c *OpenLibraryClient) Search(ctx context.Context, query string, limit int) ([]book.Book, error) {
	if limit <= 0 {
		limit = 10
	}

	searchURL := fmt.Sprintf("%s/search.json?q=%s&limit=%d",
		c.baseURL, url.QueryEscape(query), limit)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("OpenLibrary returned status %d", resp.StatusCode)
	}

	var result olSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}

	books := make([]book.Book, 0, len(result.Docs))
	for _, doc := range result.Docs {
		author := ""
		if len(doc.AuthorName) > 0 {
			author = strings.Join(doc.AuthorName, ", ")
		}

		isbn := ""
		if len(doc.ISBN) > 0 {
			isbn = doc.ISBN[0]
		}

		var tags []string
		for i, s := range doc.Subject {
			if i >= 5 {
				break
			}
			tags = append(tags, s)
		}

		books = append(books, book.Book{
			Title:     doc.Title,
			Author:    author,
			ISBN:      isbn,
			Tags:      tags,
			Status:    book.StatusWantToRead,
			DateAdded: time.Now(),
		})
	}

	return books, nil
}

func (c *OpenLibraryClient) fetchWork(ctx context.Context, key string) (*olWork, error) {
	url := fmt.Sprintf("%s%s.json", c.baseURL, key)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var work olWork
	if err := json.NewDecoder(resp.Body).Decode(&work); err != nil {
		return nil, err
	}

	return &work, nil
}

func (c *OpenLibraryClient) fetchAuthor(ctx context.Context, key string) (*olAuthor, error) {
	url := fmt.Sprintf("%s%s.json", c.baseURL, key)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var author olAuthor
	if err := json.NewDecoder(resp.Body).Decode(&author); err != nil {
		return nil, err
	}

	return &author, nil
}

func normalizeISBN(isbn string) string {
	// Remove hyphens and spaces
	isbn = strings.ReplaceAll(isbn, "-", "")
	isbn = strings.ReplaceAll(isbn, " ", "")
	return isbn
}

func extractDescription(desc any) string {
	switch v := desc.(type) {
	case string:
		return v
	case map[string]any:
		if val, ok := v["value"].(string); ok {
			return val
		}
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
