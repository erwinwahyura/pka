package web

import (
	"bytes"
	"context"
	"embed"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/erwar/pka/internal/book"
	"github.com/erwar/pka/internal/scraper"
	"github.com/erwar/pka/internal/search"
)

//go:embed templates/*.html
var templateFS embed.FS

type Server struct {
	bookService  *book.Service
	searchEngine *search.Engine
	templates    *template.Template
	mux          *http.ServeMux
}

func NewServer(bookService *book.Service, searchEngine *search.Engine) *Server {
	// Parse templates with custom functions
	funcMap := template.FuncMap{
		"stars": func(n int) string {
			return strings.Repeat("★", n) + strings.Repeat("☆", 5-n)
		},
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return "-"
			}
			return t.Format("Jan 2, 2006")
		},
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"statusColor": func(s book.Status) string {
			switch s {
			case book.StatusWantToRead:
				return "bg-yellow-100 text-yellow-800"
			case book.StatusReading:
				return "bg-blue-100 text-blue-800"
			case book.StatusRead:
				return "bg-green-100 text-green-800"
			default:
				return "bg-gray-100 text-gray-800"
			}
		},
		"mul": func(a float32, b int) float32 {
			return a * float32(b)
		},
	}

	tmpl := template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))

	s := &Server{
		bookService:  bookService,
		searchEngine: searchEngine,
		templates:    tmpl,
		mux:          http.NewServeMux(),
	}

	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/", s.handleDashboard)
	s.mux.HandleFunc("/books", s.handleBooks)
	s.mux.HandleFunc("/books/", s.handleBookDetail)
	s.mux.HandleFunc("/search", s.handleSearch)
	s.mux.HandleFunc("/scrape", s.handleScrape)
	s.mux.HandleFunc("/scrape/execute", s.handleScrapeExecute)
	s.mux.HandleFunc("/add", s.handleAdd)
	s.mux.HandleFunc("/delete/", s.handleDelete)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()
	books, _ := s.bookService.List(ctx)

	// Calculate stats
	stats := struct {
		Total       int
		WantToRead  int
		Reading     int
		Read        int
		RecentBooks []book.Book
	}{
		Total: len(books),
	}

	for _, b := range books {
		switch b.Status {
		case book.StatusWantToRead:
			stats.WantToRead++
		case book.StatusReading:
			stats.Reading++
		case book.StatusRead:
			stats.Read++
		}
	}

	// Get recent books (last 5)
	if len(books) > 5 {
		stats.RecentBooks = books[:5]
	} else {
		stats.RecentBooks = books
	}

	s.render(w, "dashboard.html", stats)
}

func (s *Server) handleBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := r.URL.Query().Get("status")

	var books []book.Book
	var err error

	if status != "" {
		books, err = s.bookService.ListByStatus(ctx, book.Status(status))
	} else {
		books, err = s.bookService.List(ctx)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Books  []book.Book
		Status string
	}{
		Books:  books,
		Status: status,
	}

	s.render(w, "books.html", data)
}

func (s *Server) handleBookDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/books/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	// Handle update
	if r.Method == http.MethodPost {
		r.ParseForm()
		b, err := s.bookService.Get(ctx, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		if status := r.FormValue("status"); status != "" {
			b.Status = book.Status(status)
			if b.Status == book.StatusRead && b.DateRead.IsZero() {
				b.DateRead = time.Now()
			}
		}
		if rating := r.FormValue("rating"); rating != "" {
			b.Rating, _ = strconv.Atoi(rating)
		}
		if notes := r.FormValue("notes"); notes != "" {
			b.Notes = notes
		}

		s.bookService.Update(ctx, b)
		http.Redirect(w, r, "/books/"+idStr, http.StatusSeeOther)
		return
	}

	b, err := s.bookService.Get(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	s.render(w, "book_detail.html", b)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	data := struct {
		Query   string
		Results []book.SearchResult
	}{
		Query: query,
	}

	if query != "" {
		results, err := s.searchEngine.Search(r.Context(), query, 20)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.Results = results
	}

	s.render(w, "search.html", data)
}

func (s *Server) handleScrape(w http.ResponseWriter, r *http.Request) {
	s.render(w, "scrape.html", nil)
}

func (s *Server) handleScrapeExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/scrape", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	scrapeType := r.FormValue("type")
	query := r.FormValue("query")
	source := r.FormValue("source")
	limitStr := r.FormValue("limit")
	limit := 20
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	ctx := r.Context()
	var books []book.Book
	var err error

	switch scrapeType {
	case "author":
		if source == "google" {
			client := scraper.NewGoogleBooksClient("")
			books, err = client.SearchByAuthor(ctx, query, limit)
		} else {
			client := scraper.NewOpenLibraryClient()
			books, err = client.FetchAuthorBooks(ctx, query, limit)
		}
	case "subject":
		if source == "google" {
			client := scraper.NewGoogleBooksClient("")
			books, err = client.SearchBySubject(ctx, query, limit)
		} else {
			client := scraper.NewOpenLibraryClient()
			books, err = client.FetchBySubject(ctx, query, limit)
		}
	case "trending":
		client := scraper.NewOpenLibraryClient()
		books, err = client.FetchTrending(ctx, query, limit) // query is period here
	}

	if err != nil {
		data := struct {
			Error string
		}{Error: err.Error()}
		s.render(w, "scrape_results.html", data)
		return
	}

	// Add books and track results
	var added, skipped int
	for i := range books {
		if addErr := s.bookService.Add(ctx, &books[i]); addErr != nil {
			if _, ok := addErr.(*book.DuplicateError); ok {
				skipped++
			}
		} else {
			added++
		}
	}

	data := struct {
		Query   string
		Type    string
		Added   int
		Skipped int
		Total   int
	}{
		Query:   query,
		Type:    scrapeType,
		Added:   added,
		Skipped: skipped,
		Total:   len(books),
	}

	s.render(w, "scrape_results.html", data)
}

func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()

		b := &book.Book{
			Title:       r.FormValue("title"),
			Author:      r.FormValue("author"),
			ISBN:        r.FormValue("isbn"),
			Genre:       r.FormValue("genre"),
			Description: r.FormValue("description"),
			Notes:       r.FormValue("notes"),
			Status:      book.Status(r.FormValue("status")),
			DateAdded:   time.Now(),
		}

		if rating := r.FormValue("rating"); rating != "" {
			b.Rating, _ = strconv.Atoi(rating)
		}

		if tags := r.FormValue("tags"); tags != "" {
			b.Tags = strings.Split(tags, ",")
			for i := range b.Tags {
				b.Tags[i] = strings.TrimSpace(b.Tags[i])
			}
		}

		if b.Status == book.StatusRead {
			b.DateRead = time.Now()
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		if err := s.bookService.Add(ctx, b); err != nil {
			data := struct {
				Error string
				Book  *book.Book
			}{Error: err.Error(), Book: b}
			s.render(w, "add.html", data)
			return
		}

		http.Redirect(w, r, "/books/"+strconv.FormatInt(b.ID, 10), http.StatusSeeOther)
		return
	}

	s.render(w, "add.html", nil)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/books", http.StatusSeeOther)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/delete/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	s.bookService.Delete(r.Context(), id)
	http.Redirect(w, r, "/books", http.StatusSeeOther)
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}
