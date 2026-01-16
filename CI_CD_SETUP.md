# CI/CD Setup Guide for PKA

This guide will help you set up automated deployments using GitHub Actions. Every time you push to the `main` branch, your code will automatically deploy to your Hetzner server.

## Overview

The CI/CD pipeline will:
1. ✅ Run tests on every push
2. 🔨 Build the application
3. 🚀 Deploy to your Hetzner server (157.180.123.200)
4. 🏥 Verify the deployment is healthy
5. 📧 Notify you of success/failure

## Prerequisites

- GitHub repository for your code
- SSH access to your Hetzner server
- Your code already deployed once manually (initial setup)

## Step 1: Initial Server Setup

First, let's set up your Hetzner server for CI/CD deployments.

### 1.1 Connect to Your Server

```bash
ssh root@157.180.123.200
```

### 1.2 Install Git on Server

```bash
apt update
apt install git -y
```

### 1.3 Clone Your Repository

```bash
cd /opt
git clone https://github.com/erwar/pka.git
cd pka
```

If you haven't pushed to GitHub yet, we'll do that in Step 2.

### 1.4 Make Deploy Script Executable

```bash
chmod +x /opt/pka/deploy.sh
```

## Step 2: Push Your Code to GitHub

If you haven't already pushed your code to GitHub:

### 2.1 Create a GitHub Repository

1. Go to https://github.com/new
2. Create a repository named `pka`
3. Don't initialize with README (you already have one)

### 2.2 Push Your Local Code

On your Mac:

```bash
cd /Users/hiru/Documents/craft/pka

# Initialize git (if not already done)
git init

# Add GitHub as remote
git remote add origin https://github.com/YOUR_USERNAME/pka.git

# Add all files
git add .

# Commit
git commit -m "Initial commit with CI/CD setup"

# Push to GitHub
git branch -M main
git push -u origin main
```

## Step 3: Set Up SSH Key for GitHub Actions

GitHub Actions needs SSH access to your server to deploy.

### 3.1 Generate SSH Key Pair

On your Mac or server:

```bash
# Generate new SSH key (no passphrase)
ssh-keygen -t ed25519 -C "github-actions" -f ~/.ssh/github_actions_key -N ""

# This creates two files:
# - github_actions_key (private key - for GitHub)
# - github_actions_key.pub (public key - for server)
```

### 3.2 Copy Public Key to Server

```bash
# Copy the public key
cat ~/.ssh/github_actions_key.pub

# SSH to your server
ssh root@157.180.123.200

# Add the public key to authorized_keys
mkdir -p ~/.ssh
echo "YOUR_PUBLIC_KEY_HERE" >> ~/.ssh/authorized_keys
chmod 700 ~/.ssh
chmod 600 ~/.ssh/authorized_keys
```

### 3.3 Save Private Key for GitHub

```bash
# Display private key (you'll copy this to GitHub)
cat ~/.ssh/github_actions_key
```

Copy the entire output (including `-----BEGIN OPENSSH PRIVATE KEY-----` and `-----END OPENSSH PRIVATE KEY-----`)

## Step 4: Configure GitHub Secrets

GitHub Secrets store sensitive information like SSH keys.

### 4.1 Go to Repository Settings

1. Open your GitHub repository
2. Click **Settings** → **Secrets and variables** → **Actions**
3. Click **New repository secret**

### 4.2 Add Required Secrets

Add these secrets one by one:

| Secret Name | Value | Description |
|------------|-------|-------------|
| `SERVER_HOST` | `157.180.123.200` | Your Hetzner server IP |
| `SERVER_USER` | `root` | SSH username |
| `SSH_PRIVATE_KEY` | *paste private key* | The private key from step 3.3 |
| `SERVER_PORT` | `22` | SSH port (optional, defaults to 22) |

**For SSH_PRIVATE_KEY**: Paste the entire private key including the header and footer lines.

## Step 5: Test the CI/CD Pipeline

### 5.1 Make a Small Change

On your Mac:

```bash
cd /Users/hiru/Documents/craft/pka

# Make a small change (example)
echo "# CI/CD enabled" >> README.md

# Commit and push
git add .
git commit -m "Test CI/CD pipeline"
git push origin main
```

### 5.2 Watch the Deployment

1. Go to your GitHub repository
2. Click the **Actions** tab
3. You should see your workflow running
4. Click on it to see real-time logs

### 5.3 Verify Deployment

Once complete, visit:
```
http://157.180.123.200:8080
```

Your changes should be live!

## Manual Deployment Option

You can also deploy manually using the deployment script:

### On the Server:

```bash
ssh root@157.180.123.200
cd /opt/pka
./deploy.sh
```

### Remote Deployment from Your Mac:

```bash
ssh root@157.180.123.200 "cd /opt/pka && ./deploy.sh"
```

## Workflow Triggers

The deployment workflow runs when:
- ✅ You push to the `main` branch
- ✅ You manually trigger it from GitHub Actions tab

To manually trigger:
1. Go to **Actions** tab in GitHub
2. Click **Deploy to Hetzner**
3. Click **Run workflow**

## Understanding the Pipeline

Here's what happens on each push:

1. **Test Job** (runs first)
   - Checks out code
   - Sets up Go environment
   - Downloads dependencies
   - Runs tests
   - Builds CLI and Web binaries

2. **Deploy Job** (only if tests pass)
   - SSH into your server
   - Navigate to `/opt/pka`
   - Pull latest code from GitHub
   - Backup database
   - Stop containers
   - Rebuild and restart containers
   - Verify containers are running

3. **Health Check**
   - Waits 5 seconds
   - Checks if app responds on port 8080
   - Fails deployment if unhealthy

## Troubleshooting

### Deployment Fails with "Permission Denied"

Check SSH key setup:
```bash
# On server, verify authorized_keys
cat ~/.ssh/authorized_keys

# Test SSH connection locally
ssh -i ~/.ssh/github_actions_key root@157.180.123.200
```

### Tests Failing

```bash
# Run tests locally first
cd /Users/hiru/Documents/craft/pka
go test ./...
```

### Containers Not Starting

Check the logs in GitHub Actions, or SSH to server:
```bash
ssh root@157.180.123.200
cd /opt/pka
docker-compose logs
```

### Database Lost After Deployment

The deployment script creates automatic backups in `/root/backups/`. To restore:
```bash
# List backups
ls -lh /root/backups/

# Restore backup
docker exec -i pka-web sh -c 'cat > /data/books.db' < /root/backups/books-YYYYMMDD-HHMMSS.db
docker-compose restart
```

## Security Best Practices

1. ✅ **Use SSH Keys**: Never use passwords in CI/CD
2. ✅ **Protect Secrets**: Never commit secrets to git
3. ✅ **Limit Access**: Only `main` branch triggers deployment
4. ✅ **Regular Backups**: Deployment script auto-backs up database
5. ✅ **Monitor Logs**: Check GitHub Actions logs regularly

## Advanced: Deployment Notifications

### Add Slack Notifications

Add this to your workflow after the deploy job:

```yaml
- name: Slack Notification
  uses: 8398a7/action-slack@v3
  with:
    status: ${{ job.status }}
    text: 'Deployment to production'
    webhook_url: ${{ secrets.SLACK_WEBHOOK }}
  if: always()
```

### Add Discord Notifications

```yaml
- name: Discord Notification
  uses: sarisia/actions-status-discord@v1
  with:
    webhook: ${{ secrets.DISCORD_WEBHOOK }}
    status: ${{ job.status }}
    title: "PKA Deployment"
    description: "Deployment to http://157.180.123.200:8080"
```

## Rollback Procedure

If a deployment goes wrong:

### Option 1: Rollback via Git

```bash
# On your Mac
git log --oneline  # Find the good commit hash
git revert HEAD    # Or specific commit
git push origin main  # This triggers re-deployment
```

### Option 2: Manual Rollback on Server

```bash
ssh root@157.180.123.200
cd /opt/pka

# Restore database from backup
docker exec -i pka-web sh -c 'cat > /data/books.db' < /root/backups/books-LATEST.db

# Rollback code
git log --oneline
git reset --hard GOOD_COMMIT_HASH

# Rebuild
docker-compose down
docker-compose up -d --build
```

## Development Workflow

Recommended git workflow:

```bash
# Create feature branch
git checkout -b feature/new-feature

# Make changes and test locally
# ... code changes ...

# Commit
git add .
git commit -m "Add new feature"

# Push to GitHub (doesn't trigger deployment)
git push origin feature/new-feature

# Create Pull Request on GitHub
# After review, merge to main
# This triggers automatic deployment!
```

## Cost Considerations

GitHub Actions free tier includes:
- 2,000 minutes/month for private repos
- Unlimited for public repos

Each deployment takes ~2-5 minutes, so you can deploy 400+ times/month on free tier.

## Next Steps

1. ✅ Set up GitHub repository
2. ✅ Configure secrets
3. ✅ Test deployment
4. Consider adding:
   - More comprehensive tests
   - Staging environment
   - Database migrations
   - Monitoring and alerts

## Quick Reference

```bash
# Push changes (triggers deployment)
git push origin main

# View deployment status
# Go to: https://github.com/YOUR_USERNAME/pka/actions

# Manual deploy on server
ssh root@157.180.123.200 "cd /opt/pka && ./deploy.sh"

# Check app status
curl http://157.180.123.200:8080

# View server logs
ssh root@157.180.123.200 "cd /opt/pka && docker-compose logs -f"
```

Happy deploying! 🚀
