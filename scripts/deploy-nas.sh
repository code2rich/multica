#!/bin/bash
set -e

# Deploy multica to NAS private registry and restart remote services
# Usage: ./scripts/deploy-nas.sh [version]

REGISTRY="192.168.31.199:5000"
VERSION="${1:-dev}"
NAS_HOST="root@192.168.31.199"
NAS_COMPOSE_DIR="/data_n002/data/udata/real/18379178030/home/docker/multica"

echo "==> Building and pushing ${VERSION} to NAS registry..."
./scripts/push-nas.sh "${VERSION}"

echo ""
echo "==> Updating remote .env to use private registry..."
ssh "${NAS_HOST}" "sed -i \"s|^MULTICA_BACKEND_IMAGE=.*|MULTICA_BACKEND_IMAGE=${REGISTRY}/multica-backend|\" ${NAS_COMPOSE_DIR}/.env"
ssh "${NAS_HOST}" "sed -i \"s|^MULTICA_WEB_IMAGE=.*|MULTICA_WEB_IMAGE=${REGISTRY}/multica-web|\" ${NAS_COMPOSE_DIR}/.env"
ssh "${NAS_HOST}" "sed -i \"s|^MULTICA_IMAGE_TAG=.*|MULTICA_IMAGE_TAG=${VERSION}|\" ${NAS_COMPOSE_DIR}/.env"

echo ""
echo "==> Pulling new images on NAS and restarting services..."
ssh "${NAS_HOST}" "cd ${NAS_COMPOSE_DIR} && docker compose pull && docker compose up -d"

echo ""
echo "✓ Deployed ${VERSION} to NAS"
echo "  Registry: ${REGISTRY}"
echo "  Compose:  ${NAS_COMPOSE_DIR}"
