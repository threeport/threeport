# Install tptctl

This guide has instructions for installing the Threeport command line tool.

Note: while we're building releases for Windows, they are not tested and not
expected to work at this time.

## Get Latest Version

If you have [jq](https://jqlang.github.io/jq/) installed, run the following command:

```bash
TPTCTL_VERSION=$(curl -s "https://api.github.com/repos/threeport/threeport/releases/latest" | jq '.tag_name' -r)
```

Otherwise, look up the version at the [releases
page](https://github.com/threeport/releases/releases) and set it like so:

```bash
TPTCTL_VERSION=v0.5.1  # substitute latest version
```

## Download

Download the release and checksums:
```bash
curl -LO "https://github.com/threeport/threeport/releases/download/$TPTCTL_VERSION/tptctl_${TPTCTL_VERSION}_$(uname)_$(uname -m).tar.gz"
curl -L "https://github.com/threeport/threeport/releases/download/$TPTCTL_VERSION/checksums.txt" > checksums.txt
```

## Verify

Optional but recommended.

Run the following command on Linux to verify the integrity of the package:

```bash
sha256sum -c --ignore-missing checksums.txt
```

## Install

```bash
tar xf tptctl_${TPTCTL_VERSION}_$(uname)_$(uname -m).tar.gz
sudo mv tptctl_${TPTCTL_VERSION}_$(uname)_$(uname -m)/tptctl /usr/local/bin
```

## Cleanup

```bash
rm checksums.txt tptctl_${TPTCTL_VERSION}_$(uname)_$(uname -m).tar.gz
rm -rf tptctl_${TPTCTL_VERSION}_$(uname)_$(uname -m)
```

## View Usage Info

```bash
tptctl help
```

## Note for MacOS Users

If you have issues running `tptctl` on your machine, follow the steps outlined by Apple
[here](https://support.apple.com/guide/mac-help/open-a-mac-app-from-an-unidentified-developer-mh40616/mac).

## Next Steps

Now that you have tptctl installed, we suggest you follow our [guide to install
Threeport locally](install-threeport-local.md).

