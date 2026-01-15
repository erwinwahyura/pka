package book

import (
	"context"
	"fmt"
)

type Repository interface {
	Create(ctx context.Context, b *Book) error
	GetByID(ctx context.Context, id int64) (*Book, error)
	GetAll(ctx context.Context) ([]Book, error)
	GetByStatus(ctx context.Context, status Status) ([]Book, error)
	Update(ctx context.Context, b *Book) error
	Delete(ctx context.Context, id int64) error
	UpdateEmbedding(ctx context.Context, id int64, embedding []float32) error
	GetAllWithEmbeddings(ctx context.Context) ([]Book, error)
}

type EmbeddingService interface {
	Generate(ctx context.Context, text string) ([]float32, error)
}

type Service struct {
	repo      Repository
	embedder  EmbeddingService
}

func NewService(repo Repository, embedder EmbeddingService) *Service {
	return &Service{
		repo:     repo,
		embedder: embedder,
	}
}

func (s *Service) Add(ctx context.Context, b *Book) error {
	if err := s.repo.Create(ctx, b); err != nil {
		return fmt.Errorf("create book: %w", err)
	}

	// Generate embedding from combined text
	text := s.buildEmbeddingText(b)
	embedding, err := s.embedder.Generate(ctx, text)
	if err != nil {
		return fmt.Errorf("generate embedding: %w", err)
	}

	if err := s.repo.UpdateEmbedding(ctx, b.ID, embedding); err != nil {
		return fmt.Errorf("update embedding: %w", err)
	}

	return nil
}

func (s *Service) Get(ctx context.Context, id int64) (*Book, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]Book, error) {
	return s.repo.GetAll(ctx)
}

func (s *Service) ListByStatus(ctx context.Context, status Status) ([]Book, error) {
	return s.repo.GetByStatus(ctx, status)
}

func (s *Service) Update(ctx context.Context, b *Book) error {
	if err := s.repo.Update(ctx, b); err != nil {
		return fmt.Errorf("update book: %w", err)
	}

	// Regenerate embedding
	text := s.buildEmbeddingText(b)
	embedding, err := s.embedder.Generate(ctx, text)
	if err != nil {
		return fmt.Errorf("generate embedding: %w", err)
	}

	return s.repo.UpdateEmbedding(ctx, b.ID, embedding)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) buildEmbeddingText(b *Book) string {
	text := b.Title + " by " + b.Author
	if b.Description != "" {
		text += ". " + b.Description
	}
	if b.Genre != "" {
		text += ". Genre: " + b.Genre
	}
	if len(b.Tags) > 0 {
		text += ". Tags: "
		for i, tag := range b.Tags {
			if i > 0 {
				text += ", "
			}
			text += tag
		}
	}
	if b.Notes != "" {
		text += ". Notes: " + b.Notes
	}
	return text
}
