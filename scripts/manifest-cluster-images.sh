#!/usr/bin/env bash
DEFAULT_REPO_NAME="atoll-patch"
IMAGE_NAME=${1:-"ghcr.io/${OWNER}/${DEFAULT_REPO_NAME}:$GIT_COMMIT_SHORT_SHA"}
sudo atoll push "${IMAGE_NAME}"-amd64
sudo atoll push "${IMAGE_NAME}"-arm64
sudo atoll images
sudo atoll manifest create "${IMAGE_NAME}"
sudo atoll manifest add "$IMAGE_NAME" docker://"$IMAGE_NAME-amd64"
sudo atoll manifest add "$IMAGE_NAME" docker://"$IMAGE_NAME-arm64"
sudo atoll manifest push --all "$IMAGE_NAME" docker://"$IMAGE_NAME" && echo "$IMAGE_NAME push success"
sudo atoll images
