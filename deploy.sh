#!/bin/bash

# PKA Deployment Script
# This script can be run manually on the server or via CI/CD

set -e  # Exit on error

echo "🚀 Starting PKA deployment..."

# Configuration
APP_DIR="/opt/pka"
BACKUP_DIR="/root/backups"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Create backup directory if it doesn't exist
mkdir -p "$BACKUP_DIR"

# Backup database before deployment
if [ -d "$APP_DIR" ]; then
    echo -e "${YELLOW}📦 Creating database backup...${NC}"
    BACKUP_FILE="$BACKUP_DIR/books-$(date +%Y%m%d-%H%M%S).db"

    if docker exec pka-web cat /data/books.db > "$BACKUP_FILE" 2>/dev/null; then
        echo -e "${GREEN}✓ Backup created: $BACKUP_FILE${NC}"
    else
        echo -e "${YELLOW}⚠ No existing database to backup (first deployment?)${NC}"
    fi
fi

# Navigate to app directory
cd "$APP_DIR" || {
    echo -e "${RED}❌ App directory not found: $APP_DIR${NC}"
    exit 1
}

# Check existing data before deployment
echo -e "${YELLOW}📊 Checking existing data...${NC}"
docker volume ls | grep pka || echo "No pka volumes found"
if docker exec pka-web ls -lh /data/books.db 2>/dev/null; then
    echo -e "${GREEN}✅ Database file exists${NC}"
    docker exec pka-web ls -lh /data/books.db
else
    echo -e "${YELLOW}⚠️  No existing database found (first deployment?)${NC}"
fi

# Pull latest changes if using git
if [ -d .git ]; then
    echo -e "${YELLOW}📥 Pulling latest changes...${NC}"
    git fetch origin
    git reset --hard origin/main
    echo -e "${GREEN}✓ Code updated${NC}"
fi

# Stop running containers (preserves volumes)
echo -e "${YELLOW}🛑 Stopping running containers...${NC}"
docker-compose down

# Remove old images to free space
echo -e "${YELLOW}🧹 Cleaning up old images...${NC}"
docker image prune -f

# Build and start services
echo -e "${YELLOW}🔨 Building and starting services...${NC}"
docker-compose up -d --build

# Wait for services to start
echo -e "${YELLOW}⏳ Waiting for services to start...${NC}"
sleep 10

# Check if containers are running
if docker-compose ps | grep -q "Up"; then
    echo -e "${GREEN}✓ Containers are running${NC}"
    docker-compose ps
else
    echo -e "${RED}❌ Containers failed to start${NC}"
    docker-compose logs
    exit 1
fi

# Verify data persisted after deployment
echo -e "${YELLOW}📊 Verifying data after deployment...${NC}"
sleep 2
if docker exec pka-web ls -lh /data/books.db 2>/dev/null; then
    echo -e "${GREEN}✅ Database file exists and persisted${NC}"
    docker exec pka-web ls -lh /data/books.db
else
    echo -e "${YELLOW}⚠️  Database file not found!${NC}"
fi

# Health check
echo -e "${YELLOW}🏥 Running health check...${NC}"
sleep 5

if curl -f http://localhost:8080 > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Application is healthy and responding${NC}"
else
    echo -e "${RED}❌ Application health check failed${NC}"
    docker-compose logs pka-web
    exit 1
fi

# Display logs
echo -e "${YELLOW}📋 Recent logs:${NC}"
docker-compose logs --tail=20

echo ""
echo -e "${GREEN}🎉 Deployment completed successfully!${NC}"
echo -e "${GREEN}🌐 Application is running at http://$(hostname -I | awk '{print $1}'):8080${NC}"
