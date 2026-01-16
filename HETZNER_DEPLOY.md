# Deploy PKA to Hetzner Server (157.180.123.200)

## Quick Deployment Steps

### Step 1: Connect to Your Server

```bash
ssh root@157.180.123.200
```

### Step 2: Install Docker and Docker Compose

```bash
# Update system
apt update && apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh

# Install Docker Compose
apt install docker-compose -y

# Verify installation
docker --version
docker-compose --version
```

### Step 3: Get Your Code on the Server

**Option A: Clone from GitHub (if you've pushed to GitHub)**
```bash
cd /opt
git clone https://github.com/erwar/pka.git
cd pka
```

**Option B: Transfer files from your local machine**

On your local machine, run:
```bash
# From the pka directory on your Mac
cd /Users/hiru/Documents/craft/pka

# Transfer to server
scp -r . root@157.180.123.200:/opt/pka
```

Then on the server:
```bash
cd /opt/pka
```

### Step 4: Build and Start Services

```bash
# Build and start all services in background
docker-compose up -d --build

# Check if services are running
docker-compose ps

# View logs (Ctrl+C to exit)
docker-compose logs -f
```

### Step 5: Pull the Embedding Model

```bash
# Pull the nomic-embed-text model (this may take a few minutes)
docker exec -it pka-ollama ollama pull nomic-embed-text

# Verify the model is installed
docker exec -it pka-ollama ollama list
```

### Step 6: Configure Firewall

```bash
# Install UFW firewall
apt install ufw -y

# Allow SSH (important - don't lock yourself out!)
ufw allow 22/tcp

# Allow HTTP on port 8080
ufw allow 8080/tcp

# Enable firewall
ufw enable

# Check status
ufw status
```

### Step 7: Test Your Application

Open your browser and visit:
```
http://157.180.123.200:8080
```

You should see the PKA dashboard!

## Verification Commands

```bash
# Check if containers are running
docker ps

# Should show:
# - pka-web (your application)
# - pka-ollama (embedding service)

# View application logs
docker-compose logs pka-web

# View Ollama logs
docker-compose logs ollama

# Test Ollama API
curl http://localhost:11434/api/tags

# Check disk usage
df -h

# Check memory usage
free -h
```

## Troubleshooting

### Container won't start
```bash
# Check logs for errors
docker-compose logs

# Restart services
docker-compose restart

# Rebuild from scratch
docker-compose down
docker-compose up -d --build
```

### Cannot access from browser
```bash
# Check if port is listening
netstat -tlnp | grep 8080

# Check firewall
ufw status

# Check if containers are running
docker ps
```

### Out of memory errors
```bash
# Check memory usage
free -h
docker stats

# If Ollama is consuming too much, you may need to upgrade RAM
```

## Production Setup (Optional but Recommended)

### Add a Domain Name

1. Point your domain DNS A record to: `157.180.123.200`
2. Install Nginx as reverse proxy:

```bash
apt install nginx -y

# Create Nginx config
cat > /etc/nginx/sites-available/pka << 'EOF'
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
EOF

# Enable site
ln -s /etc/nginx/sites-available/pka /etc/nginx/sites-enabled/
nginx -t
systemctl restart nginx

# Allow HTTP/HTTPS in firewall
ufw allow 80/tcp
ufw allow 443/tcp
```

### Add HTTPS with Let's Encrypt

```bash
apt install certbot python3-certbot-nginx -y
certbot --nginx -d your-domain.com
```

## Backup Your Data

```bash
# Create backup directory
mkdir -p /root/backups

# Backup database
docker exec pka-web cat /data/books.db > /root/backups/books-$(date +%Y%m%d).db

# Or backup entire Docker volume
docker run --rm \
  -v pka-data:/data \
  -v /root/backups:/backup \
  alpine tar czf /backup/pka-data-$(date +%Y%m%d).tar.gz /data
```

## Useful Management Commands

```bash
# Restart all services
docker-compose restart

# Stop all services
docker-compose down

# View real-time logs
docker-compose logs -f

# Update to new version
cd /opt/pka
git pull  # or re-upload files
docker-compose down
docker-compose up -d --build

# Check resource usage
docker stats

# Clean up unused Docker resources
docker system prune -a
```

## Auto-start on Server Reboot

Docker Compose services are already configured with `restart: unless-stopped`, so they will automatically start when the server reboots.

## Your Server Details

- **IP Address**: 157.180.123.200
- **Application URL**: http://157.180.123.200:8080
- **Ollama API**: http://157.180.123.200:11434 (internal only)
- **Data Location**: Docker volume `pka-data`
- **Ollama Models**: Docker volume `ollama-data`

## Next Steps

1. Access your application at http://157.180.123.200:8080
2. Add some books to test it out
3. Try the semantic search feature
4. Set up a domain name and HTTPS (optional but recommended)
5. Set up automated backups

Enjoy your personal book library!
