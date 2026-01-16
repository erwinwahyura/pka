#!/bin/bash

# Quick Server Setup Script for Hetzner
# Run this on your server: bash <(curl -s https://raw.githubusercontent.com/erwinwahyura/pka/main/SERVER_SETUP.sh)

set -e

echo "🚀 Setting up PKA on Hetzner server..."

# Install Docker if not already installed
if ! command -v docker &> /dev/null; then
    echo "📦 Installing Docker..."
    apt update
    apt install -y curl
    curl -fsSL https://get.docker.com -o get-docker.sh
    sh get-docker.sh
    rm get-docker.sh
fi

# Install Docker Compose if not already installed
if ! command -v docker-compose &> /dev/null; then
    echo "📦 Installing Docker Compose..."
    apt install -y docker-compose
fi

# Install Git if not already installed
if ! command -v git &> /dev/null; then
    echo "📦 Installing Git..."
    apt install -y git
fi

# Clone or update repository
if [ -d "/opt/pka" ]; then
    echo "📥 Updating existing repository..."
    cd /opt/pka
    git fetch origin
    git reset --hard origin/main
else
    echo "📥 Cloning repository..."
    cd /opt
    git clone https://github.com/erwinwahyura/pka.git
    cd pka
fi

# Make deploy script executable
chmod +x /opt/pka/deploy.sh

# Configure firewall
if command -v ufw &> /dev/null; then
    echo "🔒 Configuring firewall..."
    ufw allow 22/tcp
    ufw allow 8080/tcp
    ufw --force enable
fi

# Start services
echo "🚀 Starting services..."
docker-compose up -d --build

# Wait for services
echo "⏳ Waiting for services to start..."
sleep 15

# Pull embedding model
echo "📚 Pulling embedding model (this may take a few minutes)..."
docker exec -it pka-ollama ollama pull nomic-embed-text || echo "⚠️  Could not pull model automatically, will need to do manually"

echo ""
echo "✅ Setup complete!"
echo "🌐 Application should be available at: http://$(hostname -I | awk '{print $1}'):8080"
echo ""
echo "📋 Useful commands:"
echo "  - View logs: docker-compose logs -f"
echo "  - Restart: docker-compose restart"
echo "  - Deploy updates: ./deploy.sh"
