package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
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
		"join": func(s []string) string {
			return strings.Join(s, ", ")
		},
		"atoi": func(s string) int {
			i, _ := strconv.Atoi(s)
			return i
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
	s.mux.HandleFunc("/discover", s.handleDiscover)
	s.mux.HandleFunc("/discover/add", s.handleDiscoverAdd)
	s.mux.HandleFunc("/scrape", s.handleScrape)
	s.mux.HandleFunc("/scrape/execute", s.handleScrapeExecute)
	s.mux.HandleFunc("/add", s.handleAdd)
	s.mux.HandleFunc("/edit/", s.handleEdit)
	s.mux.HandleFunc("/delete/", s.handleDelete)
	s.mux.HandleFunc("/export", s.handleExport)
	s.mux.HandleFunc("/import", s.handleImport)
	s.mux.HandleFunc("/stats", s.handleStats)
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
		if currentPage := r.FormValue("current_page"); currentPage != "" {
			b.CurrentPage, _ = strconv.Atoi(currentPage)
		}
		b.Notes = r.FormValue("notes")

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

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "openlibrary"
	}

	data := struct {
		Query   string
		Source  string
		Results []book.Book
		Error   string
	}{
		Query:  query,
		Source: source,
	}

	if query != "" {
		ctx := r.Context()
		var books []book.Book
		var err error

		if source == "google" {
			client := scraper.NewGoogleBooksClient("")
			books, err = client.Search(ctx, query, 20)
		} else {
			client := scraper.NewOpenLibraryClient()
			books, err = client.Search(ctx, query, 20)
		}

		if err != nil {
			data.Error = err.Error()
		} else {
			// Mark books that are already in our library
			for i := range books {
				if s.bookService.IsDuplicate(ctx, &books[i]) {
					books[i].ID = -1 // Mark as already in library
				}
			}
			data.Results = books
		}
	}

	s.render(w, "discover.html", data)
}

func (s *Server) handleDiscoverAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/discover", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	b := &book.Book{
		Title:       r.FormValue("title"),
		Author:      r.FormValue("author"),
		ISBN:        r.FormValue("isbn"),
		Genre:       r.FormValue("genre"),
		Description: r.FormValue("description"),
		CoverURL:    r.FormValue("cover_url"),
		Status:      book.StatusWantToRead,
		DateAdded:   time.Now(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := s.bookService.Add(ctx, b); err != nil {
		http.Redirect(w, r, "/discover?q="+r.FormValue("query")+"&error="+err.Error(), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/books/"+strconv.FormatInt(b.ID, 10), http.StatusSeeOther)
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
		fmt.Println(err)
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
		Error   string
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

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/edit/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	if r.Method == http.MethodPost {
		r.ParseForm()
		b, err := s.bookService.Get(ctx, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		b.Title = r.FormValue("title")
		b.Author = r.FormValue("author")
		b.ISBN = r.FormValue("isbn")
		b.Genre = r.FormValue("genre")
		b.Description = r.FormValue("description")
		b.CoverURL = r.FormValue("cover_url")
		b.Notes = r.FormValue("notes")
		b.Status = book.Status(r.FormValue("status"))

		if rating := r.FormValue("rating"); rating != "" {
			b.Rating, _ = strconv.Atoi(rating)
		}
		if pageCount := r.FormValue("page_count"); pageCount != "" {
			b.PageCount, _ = strconv.Atoi(pageCount)
		}
		if currentPage := r.FormValue("current_page"); currentPage != "" {
			b.CurrentPage, _ = strconv.Atoi(currentPage)
		}

		if tags := r.FormValue("tags"); tags != "" {
			b.Tags = strings.Split(tags, ",")
			for i := range b.Tags {
				b.Tags[i] = strings.TrimSpace(b.Tags[i])
			}
		} else {
			b.Tags = nil
		}

		if b.Status == book.StatusRead && b.DateRead.IsZero() {
			b.DateRead = time.Now()
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

	s.render(w, "edit.html", b)
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

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	books, err := s.bookService.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=pka-books.csv")

		writer := csv.NewWriter(w)
		writer.Write([]string{"ID", "Title", "Author", "ISBN", "Genre", "Description", "Tags", "Rating", "Status", "Notes", "CoverURL", "DateAdded", "DateRead"})

		for _, b := range books {
			tags := strings.Join(b.Tags, "|")
			dateRead := ""
			if !b.DateRead.IsZero() {
				dateRead = b.DateRead.Format(time.RFC3339)
			}
			writer.Write([]string{
				strconv.FormatInt(b.ID, 10),
				b.Title,
				b.Author,
				b.ISBN,
				b.Genre,
				b.Description,
				tags,
				strconv.Itoa(b.Rating),
				string(b.Status),
				b.Notes,
				b.CoverURL,
				b.DateAdded.Format(time.RFC3339),
				dateRead,
			})
		}
		writer.Flush()

	default: // json
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=pka-books.json")
		json.NewEncoder(w).Encode(books)
	}
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.render(w, "import.html", nil)
		return
	}

	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/import", http.StatusSeeOther)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.render(w, "import.html", map[string]string{"Error": "Please select a file to import"})
		return
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	var imported, skipped int
	var importErr error

	if strings.HasSuffix(header.Filename, ".json") {
		imported, skipped, importErr = s.importJSON(ctx, file)
	} else if strings.HasSuffix(header.Filename, ".csv") {
		imported, skipped, importErr = s.importCSV(ctx, file)
	} else {
		s.render(w, "import.html", map[string]string{"Error": "Unsupported file format. Please use .json or .csv"})
		return
	}

	if importErr != nil {
		s.render(w, "import.html", map[string]string{"Error": importErr.Error()})
		return
	}

	s.render(w, "import.html", map[string]any{
		"Success":  true,
		"Imported": imported,
		"Skipped":  skipped,
	})
}

func (s *Server) importJSON(ctx context.Context, r io.Reader) (imported, skipped int, err error) {
	var books []book.Book
	if err := json.NewDecoder(r).Decode(&books); err != nil {
		return 0, 0, fmt.Errorf("invalid JSON: %w", err)
	}

	for i := range books {
		books[i].ID = 0 // Reset ID for new insert
		books[i].DateAdded = time.Now()
		if err := s.bookService.Add(ctx, &books[i]); err != nil {
			if _, ok := err.(*book.DuplicateError); ok {
				skipped++
			}
		} else {
			imported++
		}
	}

	return imported, skipped, nil
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	books, err := s.bookService.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stats := struct {
		Total        int
		WantToRead   int
		Reading      int
		Read         int
		AvgRating    float64
		RatedBooks   int
		GenreCounts  map[string]int
		TopRated     []book.Book
		RecentlyRead []book.Book
		ReadByMonth  map[string]int
	}{
		GenreCounts: make(map[string]int),
		ReadByMonth: make(map[string]int),
	}

	var totalRating int

	for _, b := range books {
		stats.Total++

		switch b.Status {
		case book.StatusWantToRead:
			stats.WantToRead++
		case book.StatusReading:
			stats.Reading++
		case book.StatusRead:
			stats.Read++
			if !b.DateRead.IsZero() {
				monthKey := b.DateRead.Format("2006-01")
				stats.ReadByMonth[monthKey]++
			}
		}

		if b.Rating > 0 {
			totalRating += b.Rating
			stats.RatedBooks++
		}

		if b.Genre != "" {
			stats.GenreCounts[b.Genre]++
		}
	}

	if stats.RatedBooks > 0 {
		stats.AvgRating = float64(totalRating) / float64(stats.RatedBooks)
	}

	// Get top rated books (rating 4 or 5)
	for _, b := range books {
		if b.Rating >= 4 {
			stats.TopRated = append(stats.TopRated, b)
			if len(stats.TopRated) >= 5 {
				break
			}
		}
	}

	// Get recently read books
	for _, b := range books {
		if b.Status == book.StatusRead && !b.DateRead.IsZero() {
			stats.RecentlyRead = append(stats.RecentlyRead, b)
			if len(stats.RecentlyRead) >= 5 {
				break
			}
		}
	}

	s.render(w, "stats.html", stats)
}

func (s *Server) importCSV(ctx context.Context, r io.Reader) (imported, skipped int, err error) {
	reader := csv.NewReader(r)

	// Skip header
	if _, err := reader.Read(); err != nil {
		return 0, 0, fmt.Errorf("invalid CSV: %w", err)
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return imported, skipped, fmt.Errorf("CSV read error: %w", err)
		}

		if len(record) < 9 {
			continue // Skip invalid rows
		}

		rating, _ := strconv.Atoi(record[7])
		var tags []string
		if record[6] != "" {
			tags = strings.Split(record[6], "|")
		}

		b := &book.Book{
			Title:       record[1],
			Author:      record[2],
			ISBN:        record[3],
			Genre:       record[4],
			Description: record[5],
			Tags:        tags,
			Rating:      rating,
			Status:      book.Status(record[8]),
			DateAdded:   time.Now(),
		}

		if len(record) > 9 {
			b.Notes = record[9]
		}
		if len(record) > 10 {
			b.CoverURL = record[10]
		}

		if err := s.bookService.Add(ctx, b); err != nil {
			if _, ok := err.(*book.DuplicateError); ok {
				skipped++
			}
		} else {
			imported++
		}
	}

	return imported, skipped, nil
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
