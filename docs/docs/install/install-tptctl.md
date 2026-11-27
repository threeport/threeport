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

## Check the Version

Confirm the install worked:

```bash
tptctl version --client
```

```text
Client Version: v0.7.0
```

`--client` prints only the version of `tptctl` itself.  It does not read your
Threeport config or contact a control plane, so it works before you have
installed one.

Without `--client`, `tptctl version` also asks the control plane in your
Threeport config for its version, the way `kubectl version` does:

```text
Client Version: v0.7.0
Server Version: v0.7.0 (control plane genesis-00)
```

With no Threeport config, or no current control plane, the server line is
`Server Version: none (no control plane in the Threeport config)`.

| Flag | Behavior |
| --- | --- |
| `--client` | Print only the client version.  Does not read the config or make a network call. |
| `--control-plane-name`, `-i` | Report the server version of the named control plane instead of the current one. |
| `--output`, `-o` | `text` (default), `json` or `yaml`.  The output has `clientVersion`, `server` and `match`; `server` is `null` when there is no control plane or with `--client`, and `match` is `false` when the versions differ or the server version could not be read. |

| Situation | Exit code |
| --- | --- |
| Server version read, whether or not it matches | 0 |
| `--client` | 0 |
| No config file, or no current control plane | 0 |
| A control plane is configured but its version can't be read (unreachable, TLS or auth failure, bad response) | 1 |

In the last case `tptctl version` still prints the client version, writes the
error to stderr, and gives up after 10 seconds.  Use `--client` when you are
offline.

### If the versions don't match

When the two versions differ, `tptctl version` still prints both and exits 0,
and writes a warning to stderr:

```text
Warning: tptctl version v0.7.0-rc.6.a1b2c3d does not match control plane genesis-00 version v0.7.0-rc.6.e4f5a6b.
Install a matching tptctl before installing, upgrading or changing this control plane.
```

The versions are compared as exact strings.  They are not ordered, so a newer
`tptctl` is a mismatch too.

This matters because `tptctl` uses its own version as the default image tag for
`tptctl up`, `tptctl upgrade control-plane image`, `tptctl upgrade control-plane
blue-green` and control plane creation.  With a mismatched `tptctl`, those
commands quietly move a control plane to different images.  Install a matching
`tptctl` before you run any of them.

To get a matching `tptctl`:

* For a release candidate or a release, such as `v0.7.0-rc.6` or `v0.7.0`,
  install the released `tptctl` for the control plane's version by following
  the steps above with `TPTCTL_VERSION` set to that version.
* For a Head build, an untagged build of a commit on a release branch whose
  version ends in a commit, such as `v0.7.0-rc.6.a1b2c3d`, check out that commit
  in the Threeport repository and run `mage install:tptctlHead`.  See
  `docs/dev/head-builds.md` in the [Threeport
  repository](https://github.com/threeport/threeport).

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