# Install threeport-sdk

To install the Threeport SDK, visit the Threeport project's [releases
page](https://github.com/threeport/threeport/releases) on GitHub and download
the checksums and tarball for your OS and architecture. The SDK ships in the
same `threeport_<version>_<os>_<arch>` archive as `tptctl`.

Verify the downloaded file.

On Linux:

```bash
sha256sum -c --ignore-missing checksums.txt
```

On MacOS, `sha256sum` is not installed by default. Use `shasum` instead:

```bash
shasum -a 256 -c --ignore-missing checksums.txt
```

Unpack and install. The archive unpacks into a directory named after the
release, so the binary is one level down.

```bash
tar xf [tarball filename]
sudo mv [unpacked directory]/threeport-sdk /usr/local/bin/
```

Confirm version and view usage info.

```bash
threeport-sdk version
threeport-sdk help
```

## Note for MacOS Users

The binary is not notarized, so Gatekeeper will block the first run. The
quickest fix is to remove the quarantine attribute:

```bash
sudo xattr -d com.apple.quarantine /usr/local/bin/threeport-sdk
```

Alternatively, run `threeport-sdk` once and let MacOS block it. Then open System
Settings, go to Privacy & Security, scroll to Security and click "Open Anyway".
That button only appears after a blocked launch attempt and stays available for
about an hour. See Apple's
[documentation](https://support.apple.com/guide/mac-help/open-a-mac-app-from-an-unidentified-developer-mh40616/mac)
for details.

## Next Steps

Next, check out our [tutorial](tutorial.md) on using the SDK.