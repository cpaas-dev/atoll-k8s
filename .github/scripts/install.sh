#!/usr/bin/env bash

set -e

timestamp() {
    date +"%Y-%m-%d %T"
}

error() {
    flag=$(timestamp)
    echo -e "\033[31m ERROR [$flag] >> $* \033[0m"
    exit 1
}

logger() {
    flag=$(timestamp)
    echo -e "\033[36m INFO [$flag] >> $* \033[0m"
}

warn() {
    flag=$(timestamp)
    echo -e "\033[33m WARN [$flag] >> $* \033[0m"
}

debug() {
    flag=$(timestamp)
    echo -e "\033[32m DEBUG [$flag] >> $* \033[0m"
}

check_file_exits() {
    for f; do
        if [[ -f $f ]]; then
            logger "The machine $f is installed"
            exit 0
        fi
    done
}

check_file_exits /usr/bin/atoll

case "$(uname -m)" in
x86_64) ATOLL_ARCH=amd64 ;;
aarch64 | arm64) ATOLL_ARCH=arm64 ;;
*) error "Unsupported architecture: $(uname -m)" ;;
esac

pushd "$(mktemp -d)" >/dev/null || exit
until curl -fsSLo "atoll.tar.gz" "https://github.com/cpaas-dev/atoll-k8s/releases/download/v5.1.2-rc5/atoll_5.1.2-rc5_linux_${ATOLL_ARCH}.tar.gz"; do sleep 3; done
tar -zxf atoll.tar.gz atoll && chmod +x atoll && mv atoll /usr/bin
rm -rf atoll.tar.gz
atoll version
popd >/dev/null
