#!/bin/bash

IMAGE=localhost:5000/${OWNER}/lvscare:$GIT_COMMIT_SHORT_SHA-$ARCH
PATCH=ghcr.io/${OWNER}/atoll-patch:$GIT_COMMIT_SHORT_SHA-$ARCH
PATCH_DIR=lifecycle/docker/patch
ATOLL=sudo atoll
ATOLL_VERSION=4.1.7

if [ -z "${ARCH}" ]; then
  ARCH=$(uname -m)
fi

case "${ARCH}" in
  amd64|x86_64)
    ARCH=amd64
    ATOLL_ARCH=amd64
    ;;
  arm64|aarch64)
    ARCH=arm64
    ATOLL_ARCH=arm64
    ;;
  *)
    echo "Unsupported ARCH: ${ARCH}" >&2
    exit 1
    ;;
esac

# download atoll
wget https://github.com/cpaas-dev/atoll-k8s/releases/download/v${ATOLL_VERSION}/atoll_${ATOLL_VERSION}_linux_${ATOLL_ARCH}.tar.gz
tar -zxf atoll_${ATOLL_VERSION}_linux_${ATOLL_ARCH}.tar.gz atoll
chmod +x atoll && sudo mv atoll /usr/bin/atoll

# resolve buildah conflicts
sudo apt remove buildah
sudo apt autoremove

# use correct image name
# shellcheck disable=SC2164
cd ${PATCH_DIR}
mkdir -p images/shim
echo "${IMAGE}" > images/shim/lvscareImage
sed -i "s#__lvscare__#${IMAGE}#g" Dockerfile

sudo atoll build -t "${PATCH}" --label=atoll.io.type=patch --label=image="${IMAGE}" --platform linux/"${ARCH}" -f Dockerfile .

# save patch image
cd - && sudo atoll save -o patch-"${ARCH}".tar "${PATCH}"
