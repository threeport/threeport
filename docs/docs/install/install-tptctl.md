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
page](https://github.com/threeport/threeport/releases) and set it like so:

<pre><code><span>TPTCTL_VERSION=<span id="tptctl-latest">v0.6.1</span></span></code></pre>

<script>
(function () {
  var el = document.getElementById('tptctl-latest');
  if (!el) return;
  fetch('https://api.github.com/repos/threeport/threeport/releases/latest')
    .then(function (r) { return r.json(); })
    .then(function (d) { if (d.tag_name) el.textContent = d.tag_name; })
    .catch(function () {});
})();
</script>

## Set Platform Variables

Release artifacts use `arm64` for 64-bit ARM, but `uname -m` reports `aarch64`
on Linux. Set the package name once so the rest of the commands stay consistent:

```bash
TPTCTL_ARCH=$(uname -m)
[ "$TPTCTL_ARCH" = "aarch64" ] && TPTCTL_ARCH=arm64
TPTCTL_PKG="threeport_${TPTCTL_VERSION}_$(uname)_${TPTCTL_ARCH}"
```

## Download

Download the release and checksums:

```bash
curl -LO "https://github.com/threeport/threeport/releases/download/$TPTCTL_VERSION/${TPTCTL_PKG}.tar.gz"
curl -L "https://github.com/threeport/threeport/releases/download/$TPTCTL_VERSION/checksums.txt" > checksums.txt
```

## Verify

Optional but recommended.

On Linux:

```bash
sha256sum -c --ignore-missing checksums.txt
```

On MacOS, `sha256sum` is not installed by default. Use `shasum` instead:

```bash
shasum -a 256 -c --ignore-missing checksums.txt
```

## Install

```bash
tar xf "${TPTCTL_PKG}.tar.gz"
sudo mv "${TPTCTL_PKG}/tptctl" /usr/local/bin
```

The archive also contains `threeport-sdk` and a README. If you want the SDK on
your path as well:

```bash
sudo mv "${TPTCTL_PKG}/threeport-sdk" /usr/local/bin
```

## Cleanup

```bash
rm checksums.txt "${TPTCTL_PKG}.tar.gz"
rm -rf "${TPTCTL_PKG}"
```

## View Usage Info

```bash
tptctl help
```

## Note for MacOS Users

The binary is not notarized, so Gatekeeper will block the first run. The
quickest fix is to remove the quarantine attribute:

```bash
sudo xattr -d com.apple.quarantine /usr/local/bin/tptctl
```

Alternatively, run `tptctl` once and let MacOS block it. Then open System
Settings, go to Privacy & Security, scroll to Security and click "Open Anyway".
That button only appears after a blocked launch attempt and stays available for
about an hour. See Apple's
[documentation](https://support.apple.com/guide/mac-help/open-a-mac-app-from-an-unidentified-developer-mh40616/mac)
for details.

## Next Steps

Now that you have tptctl installed, we suggest you follow our [guide to install
Threeport locally](install-threeport-local.md).