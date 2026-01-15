# PKA - Personal Knowledge Assistant

Semantic book library CLI. Search your books by vibes, not just keywords.

## Features

- **Semantic Search**: Find books by meaning ("dark thriller with plot twists", "cozy feel-good ending")
- **Similar Books**: Find books similar to ones you love
- **Reading Tracker**: Track status (want_to_read, reading, read), ratings, and personal notes
- **Local & Private**: SQLite database, Ollama embeddings - everything stays on your machine

## Prerequisites

1. **Go 1.21+**
2. **Ollama** with an embedding model:
   ```bash
   # Install Ollama (macOS)
   brew install ollama

   # Start Ollama service
   ollama serve

   # Pull the embedding model
   ollama pull nomic-embed-text
   ```

## Installation

```bash
go install github.com/erwar/pka/cmd/pka@latest
```

Or build from source:
```bash
git clone https://github.com/erwar/pka.git
cd pka
go build -o pka ./cmd/pka
```

## Usage

### Add a book
```bash
pka add -t "Project Hail Mary" -a "Andy Weir" \
  -g "Science Fiction" \
  -d "An astronaut wakes up alone on a spaceship with no memory" \
  --tags "space,survival,funny" \
  -r 5 -s read
```

### List books
```bash
pka list                    # all books
pka list -s reading         # currently reading
pka list -s want_to_read    # TBR pile
```

### Semantic search
```bash
pka search "dark thriller with unexpected twist"
pka search "cozy feel-good story"
pka search "books about overcoming adversity"
pka search "funny science fiction"
```

### Find similar books
```bash
pka similar 1    # find books similar to book ID 1
```

### Update a book
```bash
pka update 1 -s read -r 5 -n "Loved the ending!"
```

### Show book details
```bash
pka show 1
```

### Delete a book
```bash
pka delete 1
```

## Configuration

By default, PKA stores data in `~/.pka/books.db`. Override with flags:

```bash
pka --db ./my-books.db list
pka --ollama-url http://remote:11434 add ...
pka --ollama-model mxbai-embed-large add ...
```

## Data Model

Each book has:
- Title, Author (required)
- Genre, Description, Tags
- Status: `want_to_read` | `reading` | `read`
- Rating: 1-5 stars
- Personal notes
- Semantic embedding (auto-generated)

## How It Works

1. When you add a book, PKA combines title, author, description, genre, tags, and notes into text
2. Ollama generates a semantic embedding (768-dimensional vector)
3. Embeddings are stored in SQLite alongside book data
4. Search queries are embedded the same way
5. Cosine similarity finds the most semantically similar books

## License

MIT
