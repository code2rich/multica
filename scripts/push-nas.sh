#!/bin/bash
set -e

REGISTRY="192.168.31.199:5000"
VERSION="${1:-dev}"

echo "==> Building backend..."
docker build -t multica-backend:${VERSION} .

echo "==> Building web..."
docker build -f Dockerfile.web -t multica-web:${VERSION} .

echo "==> Tagging for NAS registry..."
docker tag multica-backend:${VERSION} ${REGISTRY}/multica-backend:${VERSION}
docker tag multica-web:${VERSION} ${REGISTRY}/multica-web:${VERSION}

echo "==> Pushing to ${REGISTRY}..."
docker push ${REGISTRY}/multica-backend:${VERSION}
docker push ${REGISTRY}/multica-web:${VERSION}

echo ""
echo "✓ Pushed ${VERSION} to ${REGISTRY}"
echo "  ${REGISTRY}/multica-backend:${VERSION}"
echo "  ${REGISTRY}/multica-web:${VERSION}"
