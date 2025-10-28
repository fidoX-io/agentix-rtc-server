#!/bin/bash
# AgentIX RTC Server - Release Management Script
# Helps manage releases and upstream synchronization

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
UPSTREAM_REPO="https://github.com/livekit/livekit-server.git"
UPSTREAM_REMOTE="upstream"
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)

print_header() {
    echo -e "${BLUE}============================================${NC}"
    echo -e "${BLUE}  AgentIX RTC Server Release Manager${NC}"
    echo -e "${BLUE}============================================${NC}"
    echo
}

print_usage() {
    echo "Usage: $0 <command> [options]"
    echo
    echo "Commands:"
    echo "  release <version>     - Create a new AgentIX release"
    echo "  sync-upstream        - Sync with LiveKit upstream changes"
    echo "  check-status         - Check repository status"
    echo "  setup-upstream       - Setup upstream remote (run once)"
    echo
    echo "Examples:"
    echo "  $0 release v1.0.0"
    echo "  $0 sync-upstream"
    echo "  $0 check-status"
    echo
}

setup_upstream() {
    echo -e "${YELLOW}Setting up upstream remote...${NC}"
    
    if git remote get-url "$UPSTREAM_REMOTE" >/dev/null 2>&1; then
        echo -e "${GREEN}✓ Upstream remote already configured${NC}"
        git remote set-url "$UPSTREAM_REMOTE" "$UPSTREAM_REPO"
    else
        git remote add "$UPSTREAM_REMOTE" "$UPSTREAM_REPO"
        echo -e "${GREEN}✓ Added upstream remote${NC}"
    fi
    
    git fetch "$UPSTREAM_REMOTE"
    echo -e "${GREEN}✓ Fetched upstream changes${NC}"
}

check_status() {
    echo -e "${YELLOW}Repository Status:${NC}"
    echo
    
    # Current branch and commit
    echo -e "Current branch: ${GREEN}$CURRENT_BRANCH${NC}"
    echo -e "Current commit: ${GREEN}$(git rev-parse --short HEAD)${NC}"
    echo -e "Repository: ${GREEN}$(git remote get-url origin 2>/dev/null || echo 'No origin remote')${NC}"
    echo
    
    # Check for uncommitted changes
    if ! git diff-index --quiet HEAD --; then
        echo -e "${RED}⚠ You have uncommitted changes${NC}"
        git status --porcelain
        echo
    else
        echo -e "${GREEN}✓ Working directory clean${NC}"
    fi
    
    # Check upstream status
    if git remote get-url "$UPSTREAM_REMOTE" >/dev/null 2>&1; then
        echo -e "${GREEN}✓ Upstream remote configured${NC}"
        
        # Fetch latest upstream
        git fetch "$UPSTREAM_REMOTE" --quiet 2>/dev/null || true
        
        # Check for upstream changes
        UPSTREAM_COMMITS=$(git rev-list --count HEAD..upstream/master 2>/dev/null || echo "0")
        if [ "$UPSTREAM_COMMITS" -gt 0 ]; then
            echo -e "${YELLOW}⚠ $UPSTREAM_COMMITS commits behind upstream/master${NC}"
        else
            echo -e "${GREEN}✓ Up to date with upstream${NC}"
        fi
    else
        echo -e "${YELLOW}⚠ Upstream remote not configured (run: $0 setup-upstream)${NC}"
    fi
    
    # Check for existing releases
    echo
    echo -e "${YELLOW}Recent AgentIX releases:${NC}"
    git tag --sort=-version:refname | grep -E "^v[0-9]" | head -5 || echo "No version tags found"
}

sync_upstream() {
    echo -e "${YELLOW}Syncing with LiveKit upstream...${NC}"
    
    # Ensure upstream is configured
    if ! git remote get-url "$UPSTREAM_REMOTE" >/dev/null 2>&1; then
        echo -e "${RED}❌ Upstream remote not configured. Run: $0 setup-upstream${NC}"
        exit 1
    fi
    
    # Check for uncommitted changes
    if ! git diff-index --quiet HEAD --; then
        echo -e "${RED}❌ You have uncommitted changes. Please commit or stash them first.${NC}"
        exit 1
    fi
    
    # Fetch upstream
    echo "Fetching upstream changes..."
    git fetch "$UPSTREAM_REMOTE"
    
    # Create a merge commit
    echo "Creating merge from upstream/master..."
    if git merge --no-ff upstream/master -m "merge: sync with LiveKit upstream $(date '+%Y-%m-%d')"; then
        echo -e "${GREEN}✓ Successfully merged upstream changes${NC}"
        echo
        echo -e "${YELLOW}Next steps:${NC}"
        echo "1. Test the merged changes"
        echo "2. Resolve any conflicts in AgentIX-specific features"
        echo "3. Run tests: go test ./..."
        echo "4. Create a new release if needed"
    else
        echo -e "${RED}❌ Merge conflicts detected${NC}"
        echo
        echo -e "${YELLOW}To resolve:${NC}"
        echo "1. Fix conflicts in the affected files"
        echo "2. git add <resolved-files>"
        echo "3. git commit"
        echo "4. Test thoroughly"
    fi
}

create_release() {
    VERSION="$1"
    
    if [ -z "$VERSION" ]; then
        echo -e "${RED}❌ Version required. Example: $0 release v1.0.0${NC}"
        exit 1
    fi
    
    # Validate version format
    if ! echo "$VERSION" | grep -qE "^v[0-9]+\.[0-9]+\.[0-9]+"; then
        echo -e "${RED}❌ Version must be in format v1.2.3${NC}"
        exit 1
    fi
    
    # Check if tag already exists
    if git tag | grep -q "^$VERSION$"; then
        echo -e "${RED}❌ Tag $VERSION already exists${NC}"
        exit 1
    fi
    
    echo -e "${YELLOW}Creating AgentIX release $VERSION...${NC}"
    
    # Check for uncommitted changes
    if ! git diff-index --quiet HEAD --; then
        echo -e "${RED}❌ You have uncommitted changes. Please commit them first.${NC}"
        exit 1
    fi
    
    # Update version in code if needed
    if [ -f "version/version.go" ]; then
        echo "Updating version in code..."
        sed -i.bak "s/Version = \".*\"/Version = \"${VERSION#v}\"/" version/version.go
        rm -f version/version.go.bak
        
        if ! git diff-index --quiet HEAD --; then
            git add version/version.go
            git commit -m "version: bump to $VERSION"
        fi
    fi
    
    # Create and push tag
    git tag -a "$VERSION" -m "AgentIX RTC Server $VERSION

Features:
- AI-Enhanced noise filtering using RNNoise
- Based on LiveKit Server with AgentIX extensions
- Real-time audio processing and enhancement

Built from commit: $(git rev-parse --short HEAD)"
    
    echo -e "${GREEN}✓ Created tag $VERSION${NC}"
    
    # Push tag to trigger release workflow
    echo "Pushing tag to trigger release workflow..."
    git push origin "$VERSION"
    
    echo -e "${GREEN}✓ Release $VERSION initiated${NC}"
    echo
    echo -e "${YELLOW}Next steps:${NC}"
    echo "1. Monitor GitHub Actions: https://github.com/fidoX-io/agentix-rtc-server/actions"
    echo "2. Docker image will be available at: ghcr.io/fidox-io/agentix-rtc-server:$VERSION"
    echo "3. Update documentation if needed"
}

# Main script
print_header

case "${1:-}" in
    "release")
        create_release "$2"
        ;;
    "sync-upstream")
        sync_upstream
        ;;
    "check-status")
        check_status
        ;;
    "setup-upstream")
        setup_upstream
        ;;
    *)
        print_usage
        exit 1
        ;;
esac