package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/erwar/pka/internal/book"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	repo := &SQLiteRepository{db: db}
	if err := repo.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return repo, nil
}

func (r *SQLiteRepository) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS books (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		author TEXT NOT NULL,
		isbn TEXT,
		description TEXT,
		genre TEXT,
		tags TEXT,
		cover_url TEXT,
		rating INTEGER,
		status TEXT NOT NULL DEFAULT 'want_to_read',
		notes TEXT,
		date_added DATETIME NOT NULL,
		date_read DATETIME,
		embedding BLOB
	);

	CREATE INDEX IF NOT EXISTS idx_books_status ON books(status);
	CREATE INDEX IF NOT EXISTS idx_books_author ON books(author);
	`
	_, err := r.db.Exec(schema)
	if err != nil {
		return err
	}

	// Add columns if they don't exist (for existing databases)
	r.db.Exec("ALTER TABLE books ADD COLUMN cover_url TEXT")
	r.db.Exec("ALTER TABLE books ADD COLUMN page_count INTEGER")
	r.db.Exec("ALTER TABLE books ADD COLUMN current_page INTEGER")
	r.db.Exec("ALTER TABLE books ADD COLUMN adaptations TEXT")
	return nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func (r *SQLiteRepository) Create(ctx context.Context, b *book.Book) error {
	tags, _ := json.Marshal(b.Tags)
	adaptations, _ := json.Marshal(b.Adaptations)

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO books (title, author, isbn, description, genre, tags, cover_url, page_count, current_page, rating, status, notes, date_added, date_read, adaptations)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, b.Title, b.Author, b.ISBN, b.Description, b.Genre, string(tags), b.CoverURL, b.PageCount, b.CurrentPage, b.Rating, b.Status, b.Notes, b.DateAdded, nullTime(b.DateRead), string(adaptations))

	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	b.ID = id

	return nil
}

func (r *SQLiteRepository) GetByID(ctx context.Context, id int64) (*book.Book, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, title, author, isbn, description, genre, tags, cover_url, page_count, current_page, rating, status, notes, date_added, date_read, embedding, adaptations
		FROM books WHERE id = ?
	`, id)

	return r.scanBook(row)
}

func (r *SQLiteRepository) GetAll(ctx context.Context) ([]book.Book, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, author, isbn, description, genre, tags, cover_url, page_count, current_page, rating, status, notes, date_added, date_read, embedding, adaptations
		FROM books ORDER BY date_added DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	return r.scanBooks(rows)
}

func (r *SQLiteRepository) GetByStatus(ctx context.Context, status book.Status) ([]book.Book, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, author, isbn, description, genre, tags, cover_url, page_count, current_page, rating, status, notes, date_added, date_read, embedding, adaptations
		FROM books WHERE status = ? ORDER BY date_added DESC
	`, status)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	return r.scanBooks(rows)
}

func (r *SQLiteRepository) Update(ctx context.Context, b *book.Book) error {
	tags, _ := json.Marshal(b.Tags)
	adaptations, _ := json.Marshal(b.Adaptations)

	_, err := r.db.ExecContext(ctx, `
		UPDATE books SET
			title = ?, author = ?, isbn = ?, description = ?, genre = ?,
			tags = ?, cover_url = ?, page_count = ?, current_page = ?, rating = ?, status = ?, notes = ?, date_read = ?, adaptations = ?
		WHERE id = ?
	`, b.Title, b.Author, b.ISBN, b.Description, b.Genre, string(tags), b.CoverURL, b.PageCount, b.CurrentPage, b.Rating, b.Status, b.Notes, nullTime(b.DateRead), string(adaptations), b.ID)

	return err
}

func (r *SQLiteRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM books WHERE id = ?", id)
	return err
}

func (r *SQLiteRepository) UpdateEmbedding(ctx context.Context, id int64, embedding []float32) error {
	blob, err := encodeEmbedding(embedding)
	if err != nil {
		return fmt.Errorf("encode embedding: %w", err)
	}

	_, err = r.db.ExecContext(ctx, "UPDATE books SET embedding = ? WHERE id = ?", blob, id)
	return err
}

func (r *SQLiteRepository) GetAllWithEmbeddings(ctx context.Context) ([]book.Book, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, author, isbn, description, genre, tags, cover_url, page_count, current_page, rating, status, notes, date_added, date_read, embedding, adaptations
		FROM books WHERE embedding IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	return r.scanBooks(rows)
}

func (r *SQLiteRepository) FindByISBN(ctx context.Context, isbn string) (*book.Book, error) {
	if isbn == "" {
		return nil, nil
	}

	row := r.db.QueryRowContext(ctx, `
		SELECT id, title, author, isbn, description, genre, tags, cover_url, page_count, current_page, rating, status, notes, date_added, date_read, embedding, adaptations
		FROM books WHERE isbn = ? LIMIT 1
	`, isbn)

	b, err := r.scanBook(row)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

func (r *SQLiteRepository) FindByTitleAuthor(ctx context.Context, title, author string) (*book.Book, error) {
	if title == "" || author == "" {
		return nil, nil
	}

	// Case-insensitive search using LOWER()
	row := r.db.QueryRowContext(ctx, `
		SELECT id, title, author, isbn, description, genre, tags, cover_url, page_count, current_page, rating, status, notes, date_added, date_read, embedding, adaptations
		FROM books WHERE LOWER(title) = LOWER(?) AND LOWER(author) = LOWER(?) LIMIT 1
	`, title, author)

	b, err := r.scanBook(row)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func (r *SQLiteRepository) scanBook(s scanner) (*book.Book, error) {
	var b book.Book
	var tagsJSON string
	var adaptationsJSON string
	var coverURL sql.NullString
	var pageCount, currentPage sql.NullInt64
	var dateRead sql.NullTime
	var embeddingBlob []byte

	err := s.Scan(
		&b.ID, &b.Title, &b.Author, &b.ISBN, &b.Description, &b.Genre,
		&tagsJSON, &coverURL, &pageCount, &currentPage, &b.Rating, &b.Status, &b.Notes, &b.DateAdded, &dateRead, &embeddingBlob, &adaptationsJSON,
	)
	if err != nil {
		return nil, err
	}

	if tagsJSON != "" {
		json.Unmarshal([]byte(tagsJSON), &b.Tags)
	}
	if adaptationsJSON != "" {
		json.Unmarshal([]byte(adaptationsJSON), &b.Adaptations)
	}
	if coverURL.Valid {
		b.CoverURL = coverURL.String
	}
	if pageCount.Valid {
		b.PageCount = int(pageCount.Int64)
	}
	if currentPage.Valid {
		b.CurrentPage = int(currentPage.Int64)
	}
	if dateRead.Valid {
		b.DateRead = dateRead.Time
	}
	if len(embeddingBlob) > 0 {
		b.Embedding, _ = decodeEmbedding(embeddingBlob)
	}

	return &b, nil
}

func (r *SQLiteRepository) scanBooks(rows *sql.Rows) ([]book.Book, error) {
	var books []book.Book
	for rows.Next() {
		b, err := r.scanBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, *b)
	}
	return books, rows.Err()
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

func encodeEmbedding(embedding []float32) ([]byte, error) {
	parts := make([]string, len(embedding))
	for i, v := range embedding {
		parts[i] = fmt.Sprintf("%f", v)
	}
	return []byte(strings.Join(parts, ",")), nil
}

func decodeEmbedding(blob []byte) ([]float32, error) {
	parts := strings.Split(string(blob), ",")
	embedding := make([]float32, len(parts))
	for i, p := range parts {
		var v float32
		fmt.Sscanf(p, "%f", &v)
		embedding[i] = v
	}
	return embedding, nil
}
