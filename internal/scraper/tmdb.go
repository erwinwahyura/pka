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

type TMDBClient struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

func NewTMDBClient(apiKey string) *TMDBClient {
	return &TMDBClient{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		baseURL: "https://api.themoviedb.org/3",
		apiKey:  apiKey,
	}
}

type tmdbSearchResult struct {
	Page         int              `json:"page"`
	Results      []tmdbSearchItem `json:"results"`
	TotalResults int              `json:"total_results"`
}

type tmdbSearchItem struct {
	ID           int     `json:"id"`
	MediaType    string  `json:"media_type"` // "movie" or "tv"
	Title        string  `json:"title"`       // for movies
	Name         string  `json:"name"`        // for TV shows
	ReleaseDate  string  `json:"release_date"` // for movies (YYYY-MM-DD)
	FirstAirDate string  `json:"first_air_date"` // for TV shows (YYYY-MM-DD)
	VoteAverage  float64 `json:"vote_average"`
	Popularity   float64 `json:"popularity"`
	PosterPath   string  `json:"poster_path"`
}

// SearchAdaptations searches TMDB for adaptations of a book
// Returns both movie and TV adaptations
func (c *TMDBClient) SearchAdaptations(ctx context.Context, bookTitle, bookAuthor string) ([]book.Adaptation, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("TMDB API key not configured")
	}

	// Search query: book title (keep it simple - author can cause false negatives)
	query := bookTitle

	searchURL := fmt.Sprintf("%s/search/multi?api_key=%s&query=%s&include_adult=false",
		c.baseURL, c.apiKey, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search TMDB: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TMDB returned status %d", resp.StatusCode)
	}

	var result tmdbSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}

	return c.itemsToAdaptations(result.Results, bookTitle), nil
}

func (c *TMDBClient) itemsToAdaptations(items []tmdbSearchItem, bookTitle string) []book.Adaptation {
	var adaptations []book.Adaptation

	for _, item := range items {
		// Only include movies and TV shows
		if item.MediaType != "movie" && item.MediaType != "tv" {
			continue
		}

		// Get title
		title := item.Title
		if item.MediaType == "tv" {
			title = item.Name
		}

		// Skip if title is too dissimilar (basic filtering)
		if !strings.Contains(strings.ToLower(title), strings.ToLower(bookTitle)) &&
			!strings.Contains(strings.ToLower(bookTitle), strings.ToLower(title)) {
			continue
		}

		// Extract year
		year := 0
		dateStr := item.ReleaseDate
		if item.MediaType == "tv" {
			dateStr = item.FirstAirDate
		}
		if len(dateStr) >= 4 {
			fmt.Sscanf(dateStr[:4], "%d", &year)
		}

		// Build poster URL
		posterURL := ""
		if item.PosterPath != "" {
			posterURL = "https://image.tmdb.org/t/p/w500" + item.PosterPath
		}

		// Map media_type to AdaptationType
		adaptType := book.AdaptationMovie
		if item.MediaType == "tv" {
			adaptType = book.AdaptationTVSeries
		}

		adaptations = append(adaptations, book.Adaptation{
			Type:       adaptType,
			Title:      title,
			Year:       year,
			Rating:     item.VoteAverage,
			Popularity: item.Popularity,
			TMDBID:     item.ID,
			PosterURL:  posterURL,
		})
	}

	return adaptations
}

// GetPosterURL constructs the full poster URL from TMDB poster path
func (c *TMDBClient) GetPosterURL(posterPath string, size string) string {
	if posterPath == "" {
		return ""
	}
	if size == "" {
		size = "w500" // default size
	}
	return fmt.Sprintf("https://image.tmdb.org/t/p/%s%s", size, posterPath)
}
