#!/bin/bash
set -e
name=${1:-"kubernetes"}
version=${2:-"v1.28.15"}
TOKEN=${3:-"changeme"}
ARCH=${4:-"amd64"}
AUTHOR=${5:-"cpaas-dev"}
COMMIT_ID=${6:-"latest"}
IMAGE_NAME="ghcr.io/${AUTHOR}/atoll/${name}:${version}"

commitDATE=$(date +%Y%m%d%H%M%S)
atoll login -u "${AUTHOR}" -p "${TOKEN}" ghcr.io
atoll build --pull=always --platform=linux/"${ARCH}" -t "$IMAGE_NAME"-"${ARCH}" \
  --label org.opencontainers.image.description="kubernetes and app cluster image" \
  --label org.opencontainers.image.licenses="Atoll Sustainable Use License" \
  --label org.opencontainers.image.source="https://github.com/${AUTHOR}/atoll" \
  --label org.opencontainers.image.title="atoll-image" \
  --label org.opencontainers.image.time="${commitDATE}" \
  --label org.opencontainers.image.url="https://github.com/${AUTHOR}/atoll" \
  --label org.opencontainers.image.version="${version}" \
  --label org.opencontainers.image.revision="${COMMIT_ID}" .

atoll push "$IMAGE_NAME"-"${ARCH}"
atoll rmi  -p --force || true

