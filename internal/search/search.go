package search

import (
	"context"
	"math"
	"sort"

	"github.com/erwar/pka/internal/book"
)

type EmbeddingService interface {
	Generate(ctx context.Context, text string) ([]float32, error)
}

type Repository interface {
	GetAllWithEmbeddings(ctx context.Context) ([]book.Book, error)
}

type Engine struct {
	repo     Repository
	embedder EmbeddingService
}

func NewEngine(repo Repository, embedder EmbeddingService) *Engine {
	return &Engine{
		repo:     repo,
		embedder: embedder,
	}
}

func (e *Engine) Search(ctx context.Context, query string, limit int) ([]book.SearchResult, error) {
	queryEmbedding, err := e.embedder.Generate(ctx, query)
	if err != nil {
		return nil, err
	}

	books, err := e.repo.GetAllWithEmbeddings(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]book.SearchResult, 0, len(books))
	for _, b := range books {
		if len(b.Embedding) == 0 {
			continue
		}
		similarity := cosineSimilarity(queryEmbedding, b.Embedding)
		results = append(results, book.SearchResult{
			Book:       b,
			Similarity: similarity,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (e *Engine) FindSimilar(ctx context.Context, bookID int64, limit int) ([]book.SearchResult, error) {
	books, err := e.repo.GetAllWithEmbeddings(ctx)
	if err != nil {
		return nil, err
	}

	var targetBook *book.Book
	for i := range books {
		if books[i].ID == bookID {
			targetBook = &books[i]
			break
		}
	}

	if targetBook == nil || len(targetBook.Embedding) == 0 {
		return nil, nil
	}

	results := make([]book.SearchResult, 0, len(books)-1)
	for _, b := range books {
		if b.ID == bookID || len(b.Embedding) == 0 {
			continue
		}
		similarity := cosineSimilarity(targetBook.Embedding, b.Embedding)
		results = append(results, book.SearchResult{
			Book:       b,
			Similarity: similarity,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}
