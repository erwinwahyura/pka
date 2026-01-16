package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/erwar/pka/internal/book"
	"github.com/erwar/pka/internal/embedding"
	"github.com/erwar/pka/internal/scraper"
	"github.com/erwar/pka/internal/search"
	"github.com/erwar/pka/internal/storage"
	"github.com/erwar/pka/internal/web"
)

func main() {
	// Flags
	port := flag.String("port", "8080", "HTTP server port")
	dbPath := flag.String("db", "", "path to SQLite database")
	ollamaURL := flag.String("ollama-url", "http://localhost:11434", "Ollama API URL")
	ollamaModel := flag.String("ollama-model", "nomic-embed-text", "Ollama embedding model")
	flag.Parse()

	// Default database path
	if *dbPath == "" {
		homeDir, _ := os.UserHomeDir()
		*dbPath = filepath.Join(homeDir, ".pka", "books.db")
	}

	// Ensure database directory exists
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0755); err != nil {
		log.Fatalf("Failed to create database directory: %v", err)
	}

	// Initialize services
	repo, err := storage.NewSQLiteRepository(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer repo.Close()

	embedder := embedding.NewOllamaClient(*ollamaURL, *ollamaModel)
	bookService := book.NewService(repo, embedder)
	searchEngine := search.NewEngine(repo, embedder)

	// Initialize TMDB client (optional - requires API key)
	tmdbAPIKey := os.Getenv("TMDB_API_KEY")
	tmdbClient := scraper.NewTMDBClient(tmdbAPIKey)
	if tmdbAPIKey == "" {
		log.Println("Warning: TMDB_API_KEY not set - adaptation search will be disabled")
	}

	// Create web server
	server := web.NewServer(bookService, searchEngine, tmdbClient)

	// Start server
	addr := fmt.Sprintf(":%s", *port)
	log.Printf("Starting PKA web server on http://localhost%s", addr)
	log.Printf("Database: %s", *dbPath)

	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
