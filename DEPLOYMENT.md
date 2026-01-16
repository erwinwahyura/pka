# PKA Deployment Guide

This guide covers deploying PKA (Personal Knowledge Assistant) web interface using Docker.

## Prerequisites

- Docker and Docker Compose installed
- At least **2GB RAM** (required for Ollama with embedding models)
- For Vultr or any VPS: Minimum 2GB RAM instance recommended

## Quick Start with Docker Compose

The easiest way to deploy PKA with all dependencies:

```bash
# Build and start all services
docker-compose up -d

# Check logs
docker-compose logs -f

# Pull the embedding model (first time only)
docker exec -it pka-ollama ollama pull nomic-embed-text

# Access the application
# Open http://your-server-ip:8080 in your browser
```

## Manual Docker Build

If you prefer to build and run manually:

```bash
# Build the image
docker build -t pka-web .

# Run the container
docker run -d \
  --name pka-web \
  -p 8080:8080 \
  -v pka-data:/data \
  -e OLLAMA_URL=http://your-ollama-host:11434 \
  pka-web
```

## Deployment Options

### Option 1: Single Server (Recommended for 2GB+ RAM)

Use the provided `docker-compose.yml` which includes both the web app and Ollama:

```bash
docker-compose up -d
docker exec -it pka-ollama ollama pull nomic-embed-text
```

### Option 2: Separate Ollama Instance

If your web server has less than 2GB RAM, run Ollama on a separate server:

1. **On the Ollama server (2GB+ RAM):**
```bash
docker run -d \
  --name ollama \
  -p 11434:11434 \
  -v ollama-data:/root/.ollama \
  ollama/ollama:latest

docker exec -it ollama ollama pull nomic-embed-text
```

2. **On the web server:**
Edit `docker-compose.yml` and remove the `ollama` service, then:
```bash
# Set the remote Ollama URL
export OLLAMA_URL=http://your-ollama-server-ip:11434

docker-compose up -d pka-web
```

### Option 3: Without Ollama (Basic Mode)

Deploy without semantic search features:

```bash
# Build image
docker build -t pka-web .

# Run without Ollama
docker run -d \
  --name pka-web \
  -p 8080:8080 \
  -v pka-data:/data \
  pka-web
```

The app will still work for basic book management, but semantic search will be unavailable.

## Environment Variables

Configure the application using these environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Web server port |
| `DB_PATH` | `/data/books.db` | SQLite database path |
| `OLLAMA_URL` | `http://ollama:11434` | Ollama API endpoint |
| `OLLAMA_MODEL` | `nomic-embed-text` | Embedding model name |

Example with custom settings:
```bash
docker run -d \
  --name pka-web \
  -p 3000:3000 \
  -e PORT=3000 \
  -e OLLAMA_URL=http://192.168.1.100:11434 \
  -v pka-data:/data \
  pka-web
```

## Deploying to VPS (Vultr, DigitalOcean, etc.)

### Step 1: Choose Your Instance

- **Minimum (without Ollama)**: 1GB RAM, 1 vCPU - $5-6/month
- **Recommended (with Ollama)**: 2GB RAM, 1 vCPU - $10-12/month
- **Optimal**: 4GB RAM, 2 vCPU - $20-24/month

### Step 2: Install Docker

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Install Docker Compose
sudo apt install docker-compose -y

# Add user to docker group (optional)
sudo usermod -aG docker $USER
```

### Step 3: Deploy the Application

```bash
# Clone or upload your code
git clone https://github.com/erwar/pka.git
cd pka

# Start services
docker-compose up -d

# Pull embedding model
docker exec -it pka-ollama ollama pull nomic-embed-text

# Check status
docker-compose ps
docker-compose logs -f
```

### Step 4: Configure Firewall

```bash
# Allow HTTP traffic
sudo ufw allow 8080/tcp
sudo ufw enable
```

### Step 5: Access Your Application

Visit `http://your-server-ip:8080` in your browser.

## Production Considerations

### 1. Use a Reverse Proxy (Recommended)

Set up Nginx or Caddy for HTTPS and domain support:

**Nginx example:**
```nginx
server {
    listen 80;
    server_name books.yourdomain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 2. Enable HTTPS with Let's Encrypt

```bash
sudo apt install certbot python3-certbot-nginx -y
sudo certbot --nginx -d books.yourdomain.com
```

### 3. Persistent Data Backups

Your data is stored in Docker volumes. Back them up regularly:

```bash
# Backup database
docker exec pka-web cat /data/books.db > backup-$(date +%Y%m%d).db

# Or backup the entire volume
docker run --rm -v pka-data:/data -v $(pwd):/backup alpine tar czf /backup/pka-backup.tar.gz /data
```

### 4. Resource Monitoring

```bash
# Check container resource usage
docker stats

# View logs
docker-compose logs -f pka-web
docker-compose logs -f ollama
```

## Troubleshooting

### Ollama Out of Memory

If Ollama crashes with 1GB RAM:
- Upgrade to 2GB+ RAM instance
- Or use a separate Ollama server with more resources
- Or deploy without Ollama (basic mode)

### Cannot Connect to Ollama

```bash
# Check if Ollama is running
docker ps | grep ollama

# Test Ollama connection
curl http://localhost:11434/api/tags

# Check network connectivity
docker exec pka-web ping ollama
```

### Database Permissions

If you see permission errors:
```bash
docker-compose down
docker volume rm pka-data
docker-compose up -d
```

## Updating

To update to a new version:

```bash
# Pull latest code
git pull

# Rebuild and restart
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

## Useful Commands

```bash
# View logs
docker-compose logs -f

# Restart services
docker-compose restart

# Stop services
docker-compose down

# Stop and remove volumes (⚠️ deletes data)
docker-compose down -v

# Access container shell
docker exec -it pka-web sh

# Check embedding model status
docker exec -it pka-ollama ollama list
```

## Cost Estimates

### Vultr Pricing Examples:
- **1GB RAM** (Basic, no Ollama): $5-6/month
- **2GB RAM** (Recommended): $10-12/month
- **4GB RAM** (Optimal): $20-24/month

### Alternative Providers:
- DigitalOcean: Similar pricing
- Linode: $10/month for 2GB
- AWS Lightsail: $10/month for 2GB
- Hetzner: €4.50/month for 2GB (Europe)

## Support

For issues or questions:
- GitHub Issues: https://github.com/erwar/pka/issues
- Check logs: `docker-compose logs -f`
