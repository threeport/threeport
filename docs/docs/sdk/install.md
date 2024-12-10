# Install threeport-sdk

To install the Threeport SDK, visit the Threeport project's [releases
page](https://github.com/threeport/threeport/releases) on GitHub and download
the checksums and tarball for your OS and architecture.

Verify the downloaded file.

```bash
sha256sum -c --ignore-missing checksums.txt
```

Unpack and install.

```bash
tar xf [tarball filename]
sudo mv [binary filename] /usr/local/bin/
```

Confirm version and view usage info.

```bash
threeport-sdk version
threeport-sdk help
```

## Next Steps

Next, check out our [tutorial](tutorial.md) on using the SDK.

