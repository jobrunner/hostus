#!/usr/bin/env bash
# Hook: Security scan Docker images with trivy
# Runs when Dockerfile* files are modified via Edit/Write
# Note: This builds the image locally first, then scans it

set -euo pipefail

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(pwd)}"

# Read hook input from stdin (JSON format)
INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null)

# Exit silently if no file path
if [[ -z "$FILE_PATH" ]]; then
    exit 0
fi

# Only run for Dockerfile changes
if [[ ! "$FILE_PATH" =~ Dockerfile ]]; then
    exit 0
fi

# Check if trivy is available
if ! command -v trivy &> /dev/null; then
    echo "⚠️  trivy not found. Install with: brew install trivy (macOS) or nix-shell -p trivy" >&2
    echo "   Skipping security scan." >&2
    exit 0
fi

# Check if docker is available
if ! command -v docker &> /dev/null; then
    echo "⚠️  docker not found. Skipping image build and security scan." >&2
    exit 0
fi

# Check if file exists
if [[ ! -f "$FILE_PATH" ]]; then
    exit 0
fi

echo "🔒 Security scanning Dockerfile: $FILE_PATH" >&2

# Determine image tag based on Dockerfile name
DOCKERFILE_NAME=$(basename "$FILE_PATH")
if [[ "$DOCKERFILE_NAME" == "Dockerfile" ]]; then
    IMAGE_TAG="hostus:local-test"
elif [[ "$DOCKERFILE_NAME" == "Dockerfile.alpine" ]]; then
    IMAGE_TAG="hostus:local-test-alpine"
elif [[ "$DOCKERFILE_NAME" == "Dockerfile.ubuntu" ]]; then
    IMAGE_TAG="hostus:local-test-ubuntu"
else
    IMAGE_TAG="hostus:local-test-${DOCKERFILE_NAME}"
fi

echo "   Building image for security scan..." >&2
cd "$PROJECT_DIR"

# Build the image (timeout after 5 minutes)
if ! timeout 300 docker build -f "$FILE_PATH" -t "$IMAGE_TAG" . > /dev/null 2>&1; then
    echo "⚠️  Docker build failed or timed out. Skipping security scan." >&2
    echo "   You can build manually with: docker build -f $FILE_PATH -t $IMAGE_TAG ." >&2
    exit 0
fi

echo "   Running trivy security scan..." >&2
# Run trivy with severity filter (only HIGH and CRITICAL)
if ! trivy image --severity HIGH,CRITICAL --exit-code 1 "$IMAGE_TAG" >&2; then
    echo "❌ Security vulnerabilities found in $IMAGE_TAG" >&2
    echo "   Review the issues above and fix before pushing." >&2
    # Clean up
    docker rmi "$IMAGE_TAG" > /dev/null 2>&1 || true
    exit 1
fi

echo "✅ Security scan passed" >&2

# Clean up test image
docker rmi "$IMAGE_TAG" > /dev/null 2>&1 || true

exit 0
