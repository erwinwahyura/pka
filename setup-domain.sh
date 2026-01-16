#!/bin/bash

# Domain Setup Script for PKA
# Sets up Nginx reverse proxy and SSL for books.erwarx.com

set -e

DOMAIN="books.erwarx.com"
EMAIL="your-email@example.com"  # Change this to your email

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}🌐 Setting up domain: $DOMAIN${NC}"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Please run as root (use sudo)${NC}"
    exit 1
fi

# Install Nginx if not installed
if ! command -v nginx &> /dev/null; then
    echo -e "${YELLOW}📦 Installing Nginx...${NC}"
    apt update
    apt install -y nginx
else
    echo -e "${GREEN}✓ Nginx already installed${NC}"
fi

# Stop Nginx temporarily
systemctl stop nginx

# Create Nginx configuration
echo -e "${YELLOW}📝 Creating Nginx configuration...${NC}"
cat > /etc/nginx/sites-available/pka << 'EOF'
server {
    listen 80;
    server_name books.erwarx.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
EOF

# Enable site
ln -sf /etc/nginx/sites-available/pka /etc/nginx/sites-enabled/

# Remove default site if exists
rm -f /etc/nginx/sites-enabled/default

# Test Nginx configuration
echo -e "${YELLOW}🧪 Testing Nginx configuration...${NC}"
nginx -t

# Start Nginx
echo -e "${YELLOW}🚀 Starting Nginx...${NC}"
systemctl start nginx
systemctl enable nginx

# Configure firewall
echo -e "${YELLOW}🔒 Configuring firewall...${NC}"
ufw allow 'Nginx Full'
ufw allow 80/tcp
ufw allow 443/tcp

echo -e "${GREEN}✓ Nginx configured successfully${NC}"
echo ""
echo -e "${YELLOW}Testing domain access...${NC}"

# Wait a moment for Nginx to fully start
sleep 2

# Test if domain is accessible
if curl -f http://$DOMAIN > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Domain is accessible at http://$DOMAIN${NC}"
else
    echo -e "${YELLOW}⚠ Domain test inconclusive. Please check DNS propagation.${NC}"
    echo -e "${YELLOW}You can test manually: http://$DOMAIN${NC}"
fi

echo ""
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}Ready to set up HTTPS with Let's Encrypt?${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "Before proceeding, make sure:"
echo -e "  1. DNS has propagated (check: http://$DOMAIN works)"
echo -e "  2. You have a valid email for SSL certificates"
echo ""
read -p "Continue with SSL setup? (y/n) " -n 1 -r
echo

if [[ $REPLY =~ ^[Yy]$ ]]; then
    # Install Certbot
    echo -e "${YELLOW}📦 Installing Certbot...${NC}"
    apt update
    apt install -y certbot python3-certbot-nginx

    # Get SSL certificate
    echo -e "${YELLOW}🔐 Obtaining SSL certificate...${NC}"
    echo -e "${YELLOW}Please enter your email when prompted${NC}"

    certbot --nginx -d $DOMAIN --non-interactive --agree-tos --email $EMAIL || {
        echo -e "${RED}SSL setup failed. Common issues:${NC}"
        echo -e "  1. DNS not propagated yet (wait 5-10 minutes)"
        echo -e "  2. Domain not pointing to this server"
        echo -e "  3. Port 80/443 blocked by firewall"
        echo ""
        echo -e "${YELLOW}You can run SSL setup manually later with:${NC}"
        echo -e "  certbot --nginx -d $DOMAIN"
        exit 1
    }

    # Set up auto-renewal
    echo -e "${YELLOW}⚙️  Setting up auto-renewal...${NC}"
    systemctl enable certbot.timer
    systemctl start certbot.timer

    echo ""
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}🎉 Setup complete!${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "${GREEN}✓ Nginx configured${NC}"
    echo -e "${GREEN}✓ SSL certificate installed${NC}"
    echo -e "${GREEN}✓ Auto-renewal enabled${NC}"
    echo ""
    echo -e "${GREEN}Your application is now available at:${NC}"
    echo -e "${GREEN}🌐 https://$DOMAIN${NC}"
    echo ""
    echo -e "${YELLOW}Note: HTTP will automatically redirect to HTTPS${NC}"
else
    echo ""
    echo -e "${YELLOW}Skipped SSL setup. Your site is accessible at:${NC}"
    echo -e "${YELLOW}🌐 http://$DOMAIN${NC}"
    echo ""
    echo -e "${YELLOW}To add SSL later, run:${NC}"
    echo -e "  certbot --nginx -d $DOMAIN"
fi

echo ""
echo -e "${YELLOW}Useful commands:${NC}"
echo -e "  - Check Nginx status: systemctl status nginx"
echo -e "  - View Nginx logs: tail -f /var/log/nginx/error.log"
echo -e "  - Test SSL renewal: certbot renew --dry-run"
echo -e "  - Restart Nginx: systemctl restart nginx"
