package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/erwar/pka/internal/book"
	"github.com/erwar/pka/internal/embedding"
	"github.com/erwar/pka/internal/scraper"
	"github.com/erwar/pka/internal/search"
	"github.com/erwar/pka/internal/storage"
	"github.com/spf13/cobra"
)

var (
	dbPath     string
	ollamaURL  string
	ollamaModel string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "pka",
		Short: "Personal Knowledge Assistant - Your semantic book library",
		Long:  `PKA helps you manage your book collection with semantic search capabilities.
Search by vibes ("dark thriller with plot twists"), track reading status, and find similar books.`,
	}

	homeDir, _ := os.UserHomeDir()
	defaultDB := filepath.Join(homeDir, ".pka", "books.db")

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDB, "path to SQLite database")
	rootCmd.PersistentFlags().StringVar(&ollamaURL, "ollama-url", "http://localhost:11434", "Ollama API URL")
	rootCmd.PersistentFlags().StringVar(&ollamaModel, "ollama-model", "nomic-embed-text", "Ollama embedding model")

	rootCmd.AddCommand(
		addCmd(),
		listCmd(),
		searchCmd(),
		similarCmd(),
		updateCmd(),
		deleteCmd(),
		showCmd(),
		importCmd(),
		discoverCmd(),
		bulkImportCmd(),
		exportCmd(),
		statsCmd(),
		scrapeAuthorCmd(),
		scrapeSubjectCmd(),
		scrapeTrendingCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func initServices() (*book.Service, *search.Engine, func(), error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, nil, nil, fmt.Errorf("create db directory: %w", err)
	}

	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("init repository: %w", err)
	}

	embedder := embedding.NewOllamaClient(ollamaURL, ollamaModel)
	svc := book.NewService(repo, embedder)
	searchEngine := search.NewEngine(repo, embedder)

	cleanup := func() { repo.Close() }

	return svc, searchEngine, cleanup, nil
}

func addCmd() *cobra.Command {
	var title, author, genre, description, notes, status string
	var tags []string
	var rating int

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new book to your collection",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			if title == "" || author == "" {
				return fmt.Errorf("title and author are required")
			}

			bookStatus := book.StatusWantToRead
			if status != "" {
				bookStatus = book.Status(status)
				if !bookStatus.IsValid() {
					return fmt.Errorf("invalid status: %s (use: want_to_read, reading, read)", status)
				}
			}

			b := &book.Book{
				Title:       title,
				Author:      author,
				Genre:       genre,
				Description: description,
				Notes:       notes,
				Tags:        tags,
				Rating:      rating,
				Status:      bookStatus,
				DateAdded:   time.Now(),
			}

			if bookStatus == book.StatusRead {
				b.DateRead = time.Now()
			}

			fmt.Println("Adding book and generating embedding...")
			if err := svc.Add(context.Background(), b); err != nil {
				return err
			}

			fmt.Printf("Added: %s by %s (ID: %d)\n", b.Title, b.Author, b.ID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&title, "title", "t", "", "book title (required)")
	cmd.Flags().StringVarP(&author, "author", "a", "", "book author (required)")
	cmd.Flags().StringVarP(&genre, "genre", "g", "", "book genre")
	cmd.Flags().StringVarP(&description, "description", "d", "", "book description")
	cmd.Flags().StringVarP(&notes, "notes", "n", "", "your personal notes")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "comma-separated tags")
	cmd.Flags().IntVarP(&rating, "rating", "r", 0, "your rating (1-5)")
	cmd.Flags().StringVarP(&status, "status", "s", "want_to_read", "reading status (want_to_read, reading, read)")

	return cmd
}

func listCmd() *cobra.Command {
	var status string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List books in your collection",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			var books []book.Book
			ctx := context.Background()

			if status != "" {
				s := book.Status(status)
				if !s.IsValid() {
					return fmt.Errorf("invalid status: %s", status)
				}
				books, err = svc.ListByStatus(ctx, s)
			} else {
				books, err = svc.List(ctx)
			}

			if err != nil {
				return err
			}

			if len(books) == 0 {
				fmt.Println("No books found.")
				return nil
			}

			for _, b := range books {
				printBookShort(b)
			}

			fmt.Printf("\nTotal: %d books\n", len(books))
			return nil
		},
	}

	cmd.Flags().StringVarP(&status, "status", "s", "", "filter by status")
	return cmd
}

func searchCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Semantic search your book collection",
		Long:  `Search books by meaning, not just keywords. Examples:
  pka search "dark thriller with unexpected twist"
  pka search "cozy feel-good story"
  pka search "books about overcoming adversity"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, searchEngine, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			query := strings.Join(args, " ")
			fmt.Printf("Searching for: %s\n\n", query)

			results, err := searchEngine.Search(context.Background(), query, limit)
			if err != nil {
				return err
			}

			if len(results) == 0 {
				fmt.Println("No matching books found.")
				return nil
			}

			for _, r := range results {
				fmt.Printf("[%.2f] ", r.Similarity)
				printBookShort(r.Book)
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 5, "max results to show")
	return cmd
}

func similarCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "similar [book-id]",
		Short: "Find books similar to a specific book",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, searchEngine, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid book ID: %s", args[0])
			}

			results, err := searchEngine.FindSimilar(context.Background(), id, limit)
			if err != nil {
				return err
			}

			if len(results) == 0 {
				fmt.Println("No similar books found.")
				return nil
			}

			fmt.Println("Similar books:")
			for _, r := range results {
				fmt.Printf("[%.2f] ", r.Similarity)
				printBookShort(r.Book)
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 5, "max results to show")
	return cmd
}

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [book-id]",
		Short: "Show details of a specific book",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid book ID: %s", args[0])
			}

			b, err := svc.Get(context.Background(), id)
			if err != nil {
				return err
			}

			printBookFull(*b)
			return nil
		},
	}
}

func updateCmd() *cobra.Command {
	var status string
	var rating int
	var notes string

	cmd := &cobra.Command{
		Use:   "update [book-id]",
		Short: "Update a book's status, rating, or notes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid book ID: %s", args[0])
			}

			ctx := context.Background()
			b, err := svc.Get(ctx, id)
			if err != nil {
				return err
			}

			if status != "" {
				s := book.Status(status)
				if !s.IsValid() {
					return fmt.Errorf("invalid status: %s", status)
				}
				b.Status = s
				if s == book.StatusRead && b.DateRead.IsZero() {
					b.DateRead = time.Now()
				}
			}

			if cmd.Flags().Changed("rating") {
				b.Rating = rating
			}

			if cmd.Flags().Changed("notes") {
				b.Notes = notes
			}

			if err := svc.Update(ctx, b); err != nil {
				return err
			}

			fmt.Printf("Updated: %s by %s\n", b.Title, b.Author)
			return nil
		},
	}

	cmd.Flags().StringVarP(&status, "status", "s", "", "new status")
	cmd.Flags().IntVarP(&rating, "rating", "r", 0, "new rating (1-5)")
	cmd.Flags().StringVarP(&notes, "notes", "n", "", "new notes")

	return cmd
}

func deleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [book-id]",
		Short: "Delete a book from your collection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid book ID: %s", args[0])
			}

			ctx := context.Background()
			b, err := svc.Get(ctx, id)
			if err != nil {
				return err
			}

			if err := svc.Delete(ctx, id); err != nil {
				return err
			}

			fmt.Printf("Deleted: %s by %s\n", b.Title, b.Author)
			return nil
		},
	}
}

func printBookShort(b book.Book) {
	stars := strings.Repeat("*", b.Rating)
	fmt.Printf("[%d] %s by %s", b.ID, b.Title, b.Author)
	if stars != "" {
		fmt.Printf(" %s", stars)
	}
	fmt.Printf(" (%s)\n", b.Status)
}

func printBookFull(b book.Book) {
	fmt.Printf("ID:          %d\n", b.ID)
	fmt.Printf("Title:       %s\n", b.Title)
	fmt.Printf("Author:      %s\n", b.Author)
	if b.Genre != "" {
		fmt.Printf("Genre:       %s\n", b.Genre)
	}
	if b.Description != "" {
		fmt.Printf("Description: %s\n", b.Description)
	}
	if len(b.Tags) > 0 {
		fmt.Printf("Tags:        %s\n", strings.Join(b.Tags, ", "))
	}
	fmt.Printf("Status:      %s\n", b.Status)
	if b.Rating > 0 {
		fmt.Printf("Rating:      %s (%d/5)\n", strings.Repeat("*", b.Rating), b.Rating)
	}
	if b.Notes != "" {
		fmt.Printf("Notes:       %s\n", b.Notes)
	}
	fmt.Printf("Added:       %s\n", b.DateAdded.Format("2006-01-02"))
	if !b.DateRead.IsZero() {
		fmt.Printf("Read:        %s\n", b.DateRead.Format("2006-01-02"))
	}
}

func importCmd() *cobra.Command {
	var status string

	cmd := &cobra.Command{
		Use:   "import [isbn...]",
		Short: "Import books by ISBN from OpenLibrary",
		Long: `Fetch book metadata from OpenLibrary by ISBN and add to your collection.
Examples:
  pka import 9780593135204
  pka import 978-0-593-13520-4
  pka import 9780593135204 9780316769488 --status read`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			bookStatus := book.StatusWantToRead
			if status != "" {
				bookStatus = book.Status(status)
				if !bookStatus.IsValid() {
					return fmt.Errorf("invalid status: %s", status)
				}
			}

			client := scraper.NewOpenLibraryClient()
			ctx := context.Background()

			for _, isbn := range args {
				fmt.Printf("Fetching ISBN %s...\n", isbn)

				b, err := client.FetchByISBN(ctx, isbn)
				if err != nil {
					fmt.Printf("  Error: %v\n", err)
					continue
				}

				b.Status = bookStatus
				if bookStatus == book.StatusRead {
					b.DateRead = time.Now()
				}

				fmt.Printf("  Found: %s by %s\n", b.Title, b.Author)
				fmt.Println("  Generating embedding...")

				if err := svc.Add(ctx, b); err != nil {
					fmt.Printf("  Error saving: %v\n", err)
					continue
				}

				fmt.Printf("  Added with ID %d\n", b.ID)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&status, "status", "s", "want_to_read", "reading status for imported books")
	return cmd
}

func discoverCmd() *cobra.Command {
	var limit int
	var autoAdd bool

	cmd := &cobra.Command{
		Use:   "discover [query]",
		Short: "Search OpenLibrary and add books to your collection",
		Long: `Search OpenLibrary for books and interactively add them.
Examples:
  pka discover "Andy Weir"
  pka discover "dark fantasy"
  pka discover "Project Hail Mary" --auto`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			query := strings.Join(args, " ")
			client := scraper.NewOpenLibraryClient()
			ctx := context.Background()

			fmt.Printf("Searching OpenLibrary for: %s\n\n", query)

			books, err := client.Search(ctx, query, limit)
			if err != nil {
				return err
			}

			if len(books) == 0 {
				fmt.Println("No books found.")
				return nil
			}

			for i, b := range books {
				fmt.Printf("[%d] %s by %s", i+1, b.Title, b.Author)
				if b.ISBN != "" {
					fmt.Printf(" (ISBN: %s)", b.ISBN)
				}
				fmt.Println()
			}

			if autoAdd {
				// Add first result automatically
				b := &books[0]
				fmt.Printf("\nAuto-adding: %s by %s\n", b.Title, b.Author)
				fmt.Println("Generating embedding...")
				if err := svc.Add(ctx, b); err != nil {
					return err
				}
				fmt.Printf("Added with ID %d\n", b.ID)
				return nil
			}

			fmt.Print("\nEnter numbers to add (comma-separated), or 'q' to quit: ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			if input == "q" || input == "" {
				return nil
			}

			for _, numStr := range strings.Split(input, ",") {
				numStr = strings.TrimSpace(numStr)
				num, err := strconv.Atoi(numStr)
				if err != nil || num < 1 || num > len(books) {
					fmt.Printf("Invalid selection: %s\n", numStr)
					continue
				}

				b := &books[num-1]
				fmt.Printf("Adding: %s by %s\n", b.Title, b.Author)
				fmt.Println("Generating embedding...")

				if err := svc.Add(ctx, b); err != nil {
					fmt.Printf("Error: %v\n", err)
					continue
				}

				fmt.Printf("Added with ID %d\n", b.ID)
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 10, "max results to show")
	cmd.Flags().BoolVar(&autoAdd, "auto", false, "automatically add first result")
	return cmd
}

func bulkImportCmd() *cobra.Command {
	var status string
	var skipErrors bool

	cmd := &cobra.Command{
		Use:   "bulk-import [file]",
		Short: "Import multiple books from a file",
		Long: `Import books from a text file containing ISBNs (one per line).
Lines starting with # are treated as comments.

Example file (isbns.txt):
  # My reading list
  9780593135204
  978-0-316-76948-8
  9780553380163

Usage:
  pka bulk-import isbns.txt
  pka bulk-import isbns.txt --status read`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			bookStatus := book.StatusWantToRead
			if status != "" {
				bookStatus = book.Status(status)
				if !bookStatus.IsValid() {
					return fmt.Errorf("invalid status: %s", status)
				}
			}

			file, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("open file: %w", err)
			}
			defer file.Close()

			client := scraper.NewOpenLibraryClient()
			ctx := context.Background()

			scanner := bufio.NewScanner(file)
			var imported, failed int

			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())

				// Skip empty lines and comments
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}

				fmt.Printf("Fetching ISBN %s...\n", line)

				b, err := client.FetchByISBN(ctx, line)
				if err != nil {
					fmt.Printf("  Error: %v\n", err)
					failed++
					if !skipErrors {
						continue
					}
					continue
				}

				b.Status = bookStatus
				if bookStatus == book.StatusRead {
					b.DateRead = time.Now()
				}

				fmt.Printf("  Found: %s by %s\n", b.Title, b.Author)

				if err := svc.Add(ctx, b); err != nil {
					fmt.Printf("  Error saving: %v\n", err)
					failed++
					continue
				}

				fmt.Printf("  Added with ID %d\n", b.ID)
				imported++

				// Be nice to OpenLibrary API
				time.Sleep(500 * time.Millisecond)
			}

			fmt.Printf("\nDone! Imported: %d, Failed: %d\n", imported, failed)
			return nil
		},
	}

	cmd.Flags().StringVarP(&status, "status", "s", "want_to_read", "reading status for imported books")
	cmd.Flags().BoolVar(&skipErrors, "skip-errors", true, "continue on errors")
	return cmd
}

func exportCmd() *cobra.Command {
	var format string
	var output string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export your book collection to JSON or CSV",
		Long: `Export all books in your collection.

Examples:
  pka export                     # JSON to stdout
  pka export -f csv -o books.csv # CSV to file
  pka export -o library.json     # JSON to file`,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			books, err := svc.List(context.Background())
			if err != nil {
				return err
			}

			if len(books) == 0 {
				fmt.Println("No books to export.")
				return nil
			}

			var out *os.File
			if output == "" {
				out = os.Stdout
			} else {
				out, err = os.Create(output)
				if err != nil {
					return fmt.Errorf("create output file: %w", err)
				}
				defer out.Close()
			}

			switch format {
			case "json":
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(books); err != nil {
					return fmt.Errorf("encode JSON: %w", err)
				}

			case "csv":
				w := csv.NewWriter(out)
				defer w.Flush()

				// Header
				w.Write([]string{"ID", "Title", "Author", "ISBN", "Genre", "Description", "Tags", "Rating", "Status", "Notes", "DateAdded", "DateRead"})

				for _, b := range books {
					dateRead := ""
					if !b.DateRead.IsZero() {
						dateRead = b.DateRead.Format("2006-01-02")
					}
					w.Write([]string{
						strconv.FormatInt(b.ID, 10),
						b.Title,
						b.Author,
						b.ISBN,
						b.Genre,
						b.Description,
						strings.Join(b.Tags, ";"),
						strconv.Itoa(b.Rating),
						string(b.Status),
						b.Notes,
						b.DateAdded.Format("2006-01-02"),
						dateRead,
					})
				}

			default:
				return fmt.Errorf("unknown format: %s (use json or csv)", format)
			}

			if output != "" {
				fmt.Printf("Exported %d books to %s\n", len(books), output)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "json", "output format (json, csv)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file (default: stdout)")
	return cmd
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show reading statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			books, err := svc.List(context.Background())
			if err != nil {
				return err
			}

			if len(books) == 0 {
				fmt.Println("No books in your collection yet.")
				return nil
			}

			// Count by status
			var wantToRead, reading, read int
			var totalRating, ratedBooks int
			genreCount := make(map[string]int)
			authorCount := make(map[string]int)
			thisYear := time.Now().Year()
			var readThisYear int

			for _, b := range books {
				switch b.Status {
				case book.StatusWantToRead:
					wantToRead++
				case book.StatusReading:
					reading++
				case book.StatusRead:
					read++
					if !b.DateRead.IsZero() && b.DateRead.Year() == thisYear {
						readThisYear++
					}
				}

				if b.Rating > 0 {
					totalRating += b.Rating
					ratedBooks++
				}

				if b.Genre != "" {
					genreCount[b.Genre]++
				}

				authorCount[b.Author]++
			}

			fmt.Println("=== Library Stats ===")
			fmt.Printf("Total books: %d\n", len(books))
			fmt.Println()

			fmt.Println("By status:")
			fmt.Printf("  Want to read: %d\n", wantToRead)
			fmt.Printf("  Reading:      %d\n", reading)
			fmt.Printf("  Read:         %d\n", read)
			fmt.Println()

			fmt.Printf("Read this year (%d): %d\n", thisYear, readThisYear)

			if ratedBooks > 0 {
				avgRating := float64(totalRating) / float64(ratedBooks)
				fmt.Printf("Average rating: %.1f/5 (%d rated)\n", avgRating, ratedBooks)
			}
			fmt.Println()

			// Top genres
			if len(genreCount) > 0 {
				fmt.Println("Top genres:")
				type kv struct {
					k string
					v int
				}
				var genres []kv
				for k, v := range genreCount {
					genres = append(genres, kv{k, v})
				}
				// Simple sort (bubble sort for small data)
				for i := 0; i < len(genres)-1; i++ {
					for j := i + 1; j < len(genres); j++ {
						if genres[j].v > genres[i].v {
							genres[i], genres[j] = genres[j], genres[i]
						}
					}
				}
				for i, g := range genres {
					if i >= 5 {
						break
					}
					fmt.Printf("  %s: %d\n", g.k, g.v)
				}
				fmt.Println()
			}

			// Top authors
			fmt.Println("Top authors:")
			type kv struct {
				k string
				v int
			}
			var authors []kv
			for k, v := range authorCount {
				authors = append(authors, kv{k, v})
			}
			for i := 0; i < len(authors)-1; i++ {
				for j := i + 1; j < len(authors); j++ {
					if authors[j].v > authors[i].v {
						authors[i], authors[j] = authors[j], authors[i]
					}
				}
			}
			for i, a := range authors {
				if i >= 5 {
					break
				}
				fmt.Printf("  %s: %d book(s)\n", a.k, a.v)
			}

			return nil
		},
	}
}

func scrapeAuthorCmd() *cobra.Command {
	var limit int
	var addAll bool
	var source string

	cmd := &cobra.Command{
		Use:   "scrape-author [author name]",
		Short: "Scrape all books by an author",
		Long: `Fetch all books by an author from OpenLibrary or Google Books.

Examples:
  pka scrape-author "Brandon Sanderson"
  pka scrape-author "Andy Weir" --add-all
  pka scrape-author "Stephen King" --limit 100 --source google`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			authorName := strings.Join(args, " ")
			ctx := context.Background()

			var books []book.Book

			fmt.Printf("Searching for books by %s (source: %s)...\n\n", authorName, source)

			switch source {
			case "openlibrary", "ol":
				client := scraper.NewOpenLibraryClient()
				books, err = client.FetchAuthorBooks(ctx, authorName, limit)
			case "google", "gb":
				client := scraper.NewGoogleBooksClient("")
				books, err = client.SearchByAuthor(ctx, authorName, limit)
			default:
				return fmt.Errorf("unknown source: %s (use: openlibrary, google)", source)
			}

			if err != nil {
				return err
			}

			if len(books) == 0 {
				fmt.Println("No books found.")
				return nil
			}

			fmt.Printf("Found %d books:\n\n", len(books))
			for i, b := range books {
				fmt.Printf("[%d] %s", i+1, b.Title)
				if b.Author != "" {
					fmt.Printf(" by %s", b.Author)
				}
				fmt.Println()
			}

			if addAll {
				fmt.Printf("\nAdding all %d books...\n", len(books))
				return addBooksWithProgress(ctx, svc, books)
			}

			return promptAndAddBooks(ctx, svc, books)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 50, "max books to fetch")
	cmd.Flags().BoolVar(&addAll, "add-all", false, "add all books without prompting")
	cmd.Flags().StringVar(&source, "source", "openlibrary", "data source (openlibrary, google)")
	return cmd
}

func scrapeSubjectCmd() *cobra.Command {
	var limit int
	var addAll bool
	var source string

	cmd := &cobra.Command{
		Use:   "scrape-subject [subject/genre]",
		Short: "Scrape books by subject or genre",
		Long: `Fetch books by subject/genre from OpenLibrary or Google Books.

Popular subjects: science_fiction, fantasy, mystery, thriller, romance,
horror, biography, history, philosophy, poetry, art, music, cooking

Examples:
  pka scrape-subject "science fiction"
  pka scrape-subject fantasy --add-all
  pka scrape-subject "artificial intelligence" --source google
  pka scrape-subject thriller --limit 100`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			subject := strings.Join(args, " ")
			ctx := context.Background()

			var books []book.Book

			fmt.Printf("Searching for %s books (source: %s)...\n\n", subject, source)

			switch source {
			case "openlibrary", "ol":
				client := scraper.NewOpenLibraryClient()
				books, err = client.FetchBySubject(ctx, subject, limit)
			case "google", "gb":
				client := scraper.NewGoogleBooksClient("")
				books, err = client.SearchBySubject(ctx, subject, limit)
			default:
				return fmt.Errorf("unknown source: %s (use: openlibrary, google)", source)
			}

			if err != nil {
				return err
			}

			if len(books) == 0 {
				fmt.Println("No books found.")
				return nil
			}

			fmt.Printf("Found %d books:\n\n", len(books))
			for i, b := range books {
				fmt.Printf("[%d] %s", i+1, b.Title)
				if b.Author != "" {
					fmt.Printf(" by %s", b.Author)
				}
				fmt.Println()
			}

			if addAll {
				fmt.Printf("\nAdding all %d books...\n", len(books))
				return addBooksWithProgress(ctx, svc, books)
			}

			return promptAndAddBooks(ctx, svc, books)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 50, "max books to fetch")
	cmd.Flags().BoolVar(&addAll, "add-all", false, "add all books without prompting")
	cmd.Flags().StringVar(&source, "source", "openlibrary", "data source (openlibrary, google)")
	return cmd
}

func scrapeTrendingCmd() *cobra.Command {
	var limit int
	var addAll bool
	var period string

	cmd := &cobra.Command{
		Use:   "scrape-trending",
		Short: "Scrape trending/popular books",
		Long: `Fetch trending books from OpenLibrary.

Periods: now, daily, weekly, monthly, yearly, forever

Examples:
  pka scrape-trending
  pka scrape-trending --period monthly
  pka scrape-trending --period forever --add-all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, _, cleanup, err := initServices()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := context.Background()
			client := scraper.NewOpenLibraryClient()

			fmt.Printf("Fetching trending books (%s)...\n\n", period)

			books, err := client.FetchTrending(ctx, period, limit)
			if err != nil {
				return err
			}

			if len(books) == 0 {
				fmt.Println("No trending books found.")
				return nil
			}

			fmt.Printf("Found %d trending books:\n\n", len(books))
			for i, b := range books {
				fmt.Printf("[%d] %s", i+1, b.Title)
				if b.Author != "" {
					fmt.Printf(" by %s", b.Author)
				}
				fmt.Println()
			}

			if addAll {
				fmt.Printf("\nAdding all %d books...\n", len(books))
				return addBooksWithProgress(ctx, svc, books)
			}

			return promptAndAddBooks(ctx, svc, books)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "max books to fetch")
	cmd.Flags().BoolVar(&addAll, "add-all", false, "add all books without prompting")
	cmd.Flags().StringVar(&period, "period", "weekly", "trending period (now, daily, weekly, monthly, yearly, forever)")
	return cmd
}

// Helper function to prompt user and add selected books
func promptAndAddBooks(ctx context.Context, svc *book.Service, books []book.Book) error {
	fmt.Print("\nEnter numbers to add (comma-separated, 'all' for all, 'q' to quit): ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "q" || input == "" {
		return nil
	}

	if input == "all" {
		return addBooksWithProgress(ctx, svc, books)
	}

	var selected []book.Book
	for _, numStr := range strings.Split(input, ",") {
		numStr = strings.TrimSpace(numStr)

		// Handle ranges like "1-5"
		if strings.Contains(numStr, "-") {
			parts := strings.Split(numStr, "-")
			if len(parts) == 2 {
				start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
				end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err1 == nil && err2 == nil && start >= 1 && end <= len(books) && start <= end {
					for i := start; i <= end; i++ {
						selected = append(selected, books[i-1])
					}
					continue
				}
			}
		}

		num, err := strconv.Atoi(numStr)
		if err != nil || num < 1 || num > len(books) {
			fmt.Printf("Invalid selection: %s\n", numStr)
			continue
		}
		selected = append(selected, books[num-1])
	}

	if len(selected) == 0 {
		fmt.Println("No valid selections.")
		return nil
	}

	return addBooksWithProgress(ctx, svc, selected)
}

// Helper function to add multiple books with progress display
func addBooksWithProgress(ctx context.Context, svc *book.Service, books []book.Book) error {
	var added, failed int

	for i, b := range books {
		bookCopy := b // Create a copy to get pointer
		fmt.Printf("[%d/%d] Adding: %s...", i+1, len(books), b.Title)

		if err := svc.Add(ctx, &bookCopy); err != nil {
			fmt.Printf(" ERROR: %v\n", err)
			failed++
			continue
		}

		fmt.Printf(" OK (ID: %d)\n", bookCopy.ID)
		added++

		// Small delay to be nice to embedding service
		if i < len(books)-1 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	fmt.Printf("\nDone! Added: %d, Failed: %d\n", added, failed)
	return nil
}
