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

type GoogleBooksClient struct {
	client  *http.Client
	baseURL string
	apiKey  string // Optional API key for higher rate limits
}

func NewGoogleBooksClient(apiKey string) *GoogleBooksClient {
	return &GoogleBooksClient{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		baseURL: "https://www.googleapis.com/books/v1",
		apiKey:  apiKey,
	}
}

type gbSearchResult struct {
	TotalItems int      `json:"totalItems"`
	Items      []gbItem `json:"items"`
}

type gbItem struct {
	ID         string       `json:"id"`
	VolumeInfo gbVolumeInfo `json:"volumeInfo"`
}

type gbVolumeInfo struct {
	Title               string   `json:"title"`
	Authors             []string `json:"authors"`
	Publisher           string   `json:"publisher"`
	PublishedDate       string   `json:"publishedDate"`
	Description         string   `json:"description"`
	IndustryIdentifiers []gbISBN `json:"industryIdentifiers"`
	Categories          []string `json:"categories"`
	AverageRating       float64  `json:"averageRating"`
	RatingsCount        int      `json:"ratingsCount"`
	PageCount           int      `json:"pageCount"`
	Language            string   `json:"language"`
}

type gbISBN struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier"`
}

// Search searches Google Books by query
func (c *GoogleBooksClient) Search(ctx context.Context, query string, limit int) ([]book.Book, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 40 {
		limit = 40 // Google Books API max
	}

	searchURL := fmt.Sprintf("%s/volumes?q=%s&maxResults=%d&printType=books&langRestrict=en",
		c.baseURL, url.QueryEscape(query), limit)

	if c.apiKey != "" {
		searchURL += "&key=" + c.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search Google Books: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Google Books returned status %d", resp.StatusCode)
	}

	var result gbSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}

	return c.itemsToBooks(result.Items), nil
}

// SearchByAuthor searches for books by a specific author
func (c *GoogleBooksClient) SearchByAuthor(ctx context.Context, author string, limit int) ([]book.Book, error) {
	query := fmt.Sprintf("inauthor:%s", author)
	return c.Search(ctx, query, limit)
}

// SearchBySubject searches for books in a specific subject/category
func (c *GoogleBooksClient) SearchBySubject(ctx context.Context, subject string, limit int) ([]book.Book, error) {
	query := fmt.Sprintf("subject:%s", subject)
	return c.Search(ctx, query, limit)
}

// SearchByISBN fetches a book by ISBN
func (c *GoogleBooksClient) SearchByISBN(ctx context.Context, isbn string) (*book.Book, error) {
	query := fmt.Sprintf("isbn:%s", isbn)
	books, err := c.Search(ctx, query, 1)
	if err != nil {
		return nil, err
	}
	if len(books) == 0 {
		return nil, fmt.Errorf("ISBN not found: %s", isbn)
	}
	return &books[0], nil
}

// SearchNewest searches for newest books in a category
func (c *GoogleBooksClient) SearchNewest(ctx context.Context, category string, limit int) ([]book.Book, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 40 {
		limit = 40
	}

	query := category
	if category != "" {
		query = fmt.Sprintf("subject:%s", category)
	}

	searchURL := fmt.Sprintf("%s/volumes?q=%s&maxResults=%d&orderBy=newest&printType=books&langRestrict=en",
		c.baseURL, url.QueryEscape(query), limit)

	if c.apiKey != "" {
		searchURL += "&key=" + c.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search newest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Google Books returned status %d", resp.StatusCode)
	}

	var result gbSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}

	return c.itemsToBooks(result.Items), nil
}

func (c *GoogleBooksClient) itemsToBooks(items []gbItem) []book.Book {
	books := make([]book.Book, 0, len(items))

	for _, item := range items {
		vi := item.VolumeInfo

		// Skip items without title or author
		if vi.Title == "" {
			continue
		}

		author := ""
		if len(vi.Authors) > 0 {
			author = strings.Join(vi.Authors, ", ")
		}

		// Get ISBN (prefer ISBN-13)
		isbn := ""
		for _, id := range vi.IndustryIdentifiers {
			if id.Type == "ISBN_13" {
				isbn = id.Identifier
				break
			}
			if id.Type == "ISBN_10" && isbn == "" {
				isbn = id.Identifier
			}
		}

		// Get genre from categories
		genre := ""
		var tags []string
		if len(vi.Categories) > 0 {
			genre = vi.Categories[0]
			for i, cat := range vi.Categories {
				if i >= 5 {
					break
				}
				tags = append(tags, cat)
			}
		}

		books = append(books, book.Book{
			Title:       vi.Title,
			Author:      author,
			ISBN:        isbn,
			Description: truncateGB(vi.Description, 500),
			Genre:       genre,
			Tags:        tags,
			Status:      book.StatusWantToRead,
			DateAdded:   time.Now(),
		})
	}

	return books
}

func truncateGB(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
