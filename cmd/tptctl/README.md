tptctl

Manage workloads on Threeport.

## Requirements

* [curl](https://help.ubidots.com/en/articles/2165289-learn-how-to-install-run-curl-on-windows-macosx-linux)
* [wget](https://www.gnu.org/software/wget/)
* [jq](https://github.com/stedolan/jq/wiki/Installation)
* [docker](https://docs.docker.com/engine/install/)
* [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
* [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
* [homebrew](https://brew.sh/) - Optional
* [gvm](https://github.com/moovweb/gvm) Go 1.19 - Optional
    ```bash
    gvm install go1.19 --prefer-binary --with-build-tools --with-protobuf
    gvm use go1.19 --default

## Install

### Prebuilt binaries

Prebuilt binaries are available for a variety of operating systems and architectures.</br>
Set `VERSION` environment variable to latest
```bash
VERSION=$(curl --silent "https://api.github.com/repos/threeport/tptctl/releases/latest" | jq '.tag_name' -r)
```
or a specific version
```bash
VERSION=v0.0.5
```
download and install prebuilt binaries
```bash
wget https://github.com/threeport/tptctl/releases/download/${VERSION}/tptctl_${VERSION}_$(echo $(uname))_$(uname -m).tar.gz -O - | tar -xz && sudo mv tptctl /usr/local/bin/tptctl
```

### Package managers
#### Homebrew
Homebrew is a free and open source package manager for macOS and Linux.

```bash
brew tap threeport/tap
brew install threeport/tap/tptctl
```

### Linux

#### Debian
Derivatives of the Debian distribution of Linux include elementary OS, KDE neon, Linux Lite, Linux Mint, MX Linux, Pop!_OS, Ubuntu, Zorin OS, and others.</br></br>
Set `VERSION` to 
* latest
    ```bash
    VERSION=$(curl --silent "https://api.github.com/repos/threeport/tptctl/releases/latest" | jq '.tag_name' -r)
    ```
* or a specific version
    ```bash
    VERSION=v0.0.5
    ```
download and install package
```bash
TEMP_PACKAGE="$(mktemp)" && wget -O "$TEMP_PACKAGE" "https://github.com/threeport/tptctl/releases/download/${VERSION}/tptctl_${VERSION}_$(uname -m | sed -E 's/^(aarch64|aarch64_be|armv6l|armv7l|armv8b|armv8l)$$/arm64/g' | sed -E 's/^x86_64$$/amd64/g').deb" && sudo dpkg -i $TEMP_PACKAGE
rm -f "$TEMP_PACKAGE"
```

#### Fedora
Derivatives of the Fedora distribution of Linux include CentOS, Red Hat Enterprise Linux, and others.</br></br>
Set `VERSION` to
* latest
    ```bash
    VERSION=$(curl --silent "https://api.github.com/repos/threeport/tptctl/releases/latest" | jq '.tag_name' -r)
    ```
* or a specific version
    ```bash
    VERSION=v0.0.5
    ```
download and install package
```bash
TEMP_PACKAGE="$(mktemp)" && wget -O "$TEMP_PACKAGE" "https://github.com/threeport/tptctl/releases/download/${VERSION}/tptctl_${VERSION}_$(uname -m | sed -E 's/^(aarch64|aarch64_be|armv6l|armv7l|armv8b|armv8l)$$/arm64/g' | sed -E 's/^x86_64$$/amd64/g').rpm" && sudo dnf -y $TEMP_PACKAGE
rm -f "$TEMP_PACKAGE"
```

#### Alpine

Set `VERSION` to
* latest
    ```bash
    VERSION=$(curl --silent "https://api.github.com/repos/threeport/tptctl/releases/latest" | jq '.tag_name' -r)
    ```
* or a specific version
    ```bash
    VERSION=v0.0.5
    ```
download and install package
```bash
TEMP_PACKAGE="$(mktemp)" && wget -O "$TEMP_PACKAGE" "https://github.com/threeport/tptctl/releases/download/${VERSION}/tptctl_${VERSION}_$(uname -m | sed -E 's/^(aarch64|aarch64_be|armv6l|armv7l|armv8b|armv8l)$$/arm64/g' | sed -E 's/^x86_64$$/amd64/g').apk" && sudo apk add --allow-untrusted $TEMP_PACKAGE
rm -f "$TEMP_PACKAGE"
```

## Quickstart

Install the Threeport control plane:

```bash
tptctl create control-plane --name quickstart
```

Remove the Threeport control plane:

```bash
tptctl delete control-plane --name quickstart
```

## Release
Run `release` target
```bash
make release
```

### Help

```text
$ make
Usage: make COMMAND
Commands :
help                - List available tasks
clean               - Cleanup
get                 - Download and install dependency packages
update              - Update dependencies to latest versions
test                - Run tests
build               - Build tptctl binary
install             - Install the tptctl CLI
release             - Create and push a new tag
version             - Print current version(tag)
codegen-subcommand  - Build subcommand - a tool for generating subcommand source code
```
