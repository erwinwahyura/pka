package book

import "time"

type Book struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	ISBN        string    `json:"isbn,omitempty"`
	Description string    `json:"description,omitempty"`
	Genre       string    `json:"genre,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Rating      int       `json:"rating,omitempty"`      // 1-5 stars
	Status      Status    `json:"status"`                // want_to_read, reading, read
	Notes       string    `json:"notes,omitempty"`       // personal notes
	DateAdded   time.Time `json:"date_added"`
	DateRead    time.Time `json:"date_read,omitempty"`
	Embedding   []float32 `json:"-"`                     // semantic embedding vector
}

type Status string

const (
	StatusWantToRead Status = "want_to_read"
	StatusReading    Status = "reading"
	StatusRead       Status = "read"
)

func (s Status) String() string {
	return string(s)
}

func (s Status) IsValid() bool {
	switch s {
	case StatusWantToRead, StatusReading, StatusRead:
		return true
	}
	return false
}

type SearchResult struct {
	Book       Book    `json:"book"`
	Similarity float32 `json:"similarity"` // cosine similarity score
}
