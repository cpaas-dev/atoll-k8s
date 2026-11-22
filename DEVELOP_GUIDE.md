# Environment settings

atoll only support Linux now, you need a Linux server to test it.

Some tools can be very handy to help you start a virtual machine such as [multipass](https://multipass.run/)

## Install golang

```shell
wget https://go.dev/dl/go1.27.3.linux-amd64.tar.gz
tar -C /usr/local -zxvf go1.27.3.linux-amd64.tar.gz
cat >> /etc/profile <<EOF
# set go path
export PATH=\$PATH:/usr/local/go/bin
EOF
source /etc/profile  && go version
```

## Build the project

```shell script
git clone https://github.com/cpaas-dev/atoll-k8s.git
cd atoll-k8s
make build BINS=atoll
```

You can scp the bin file to your Linux host.

If you use multipass, you can mount the bin dir to the vm:

```shell script
multipass mount /your-bin-dir <name>[:<path>]
```

Then test it locally.

## Notes about cross-platform building

All binaries are built with `CGO_ENABLED=0`: no C compiler or system development libraries are needed, and `make build.multiarch PLATFORMS="linux_arm64 linux_amd64"` cross-compiles from any host. The binaries only run on Linux.

> Both Makefile and GoReleaser in this project have this setting.

## Using atoll image ci

### How to build container image
1. fix makefile build step
    ```makefile
   .PHONY: build
    build:## Build manager binary.
    CGO_ENABLED=0 go build -o bin/manager main.go
    ```
2. build amd64 bin and arm64 bin
    - amd64 bin
   ```
   GOARCH=amd64 make build
   mv bin/manager bin/controller-${{ matrix.module }}-amd64
   chmod +x bin/controller-${{ matrix.module }}-amd64
   ```
    - arm64 bin
   ```
   GOARCH=arm64 make build
   mv bin/manager bin/controller-${{ matrix.module }}-arm64
   chmod +x bin/controller-${{ matrix.module }}-arm64
   ```
3. fix dockerfile
   ```dockerfile
   FROM gcr.io/distroless/static:nonroot
   ARG TARGETARCH
   
   WORKDIR /
   USER 65532:65532
   
   COPY bin/controller-${{ matrix.module }}-$TARGETARCH /manager
   ENTRYPOINT ["/manager"]
   ```
   tips: .dockerignore not add bin dir
4. docker buildx
   ```shell
   docker buildx build \
      --platform linux/amd64,linux/arm64 \
      --push \
      -t ${DOCKER_REPO}:${{ steps.prepare.outputs.tag_name }} \
      -f Dockerfile \
      .
   ```

## Example: how to build atoll on macOS(ARM64) using multipass

1. launch vm and mount atoll source code:
```shell
# edit the ATOLL_CODE_DIR to your own
export ATOLL_CODE_DIR=$HOME/work/src/github.com/cpaas-dev/atoll-k8s
# copy, paste and run to launch vm
multipass launch \
   --mount ${ATOLL_CODE_DIR}:/go/src/github.com/cpaas-dev/atoll-k8s \
   --name atoll-dev --cpus 2 --mem 4G --disk 40G
```

2. exec into the vm
```shell
multipass exec atoll-dev bash
sudo su
```
3. install golang
```shell
apt-get install build-essential
apt install make
wget https://go.dev/dl/go1.27.3.linux-arm64.tar.gz
tar -C /usr/local -zxvf go1.27.3.linux-arm64.tar.gz
cat >> /etc/profile <<EOF
# set go path
export PATH=\$PATH:/usr/local/go/bin
EOF
source /etc/profile  && go version
```
4. Build the source code
```shell
go env -w GOPROXY=https://goproxy.cn,direct # optional
make build
```

## FAQ

1. clone code slow, your can use ghproxy: `git clone https://ghproxy.com/https://github.com/cpaas-dev/atoll-k8s`
2. build download package slow, you can use goproxy: `go env -w GOPROXY=https://goproxy.cn,direct && make build`
