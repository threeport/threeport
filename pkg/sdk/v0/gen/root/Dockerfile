# syntax=docker/dockerfile:1.7
#
# Minimal Dockerfile for module binary images.
#
# Modules ship reconciliation controllers and tptctl plugins; they
# don't embed infrastructure provisioning runtimes or in-container
# debug tooling, so the terraform, pulumi, and delve stages from
# threeport's canonical Dockerfile are omitted.
#
# Compile binaries outside Docker first (mage build:allBinsRelease or
# the equivalent for your module), then pass them in via the build
# context. The context root must contain per-arch subdirectories with
# the named binary:
#   ./<context>/amd64/<binary-name>
#   ./<context>/arm64/<binary-name>
#
# Build args:
#   BINARY        Binary file name within the per-arch directory.
#   GIT_REVISION  Commit sha stamped into org.opencontainers.image.revision.
#   GIT_TAG       Version stamped into org.opencontainers.image.version.
#   BUILD_CREATED ISO-8601 timestamp stamped into org.opencontainers.image.created.
#
# Targets:
#   release  Distroless image with the compiled binary.
#
# Multi-arch builds use `--platform=linux/amd64,linux/arm64`;
# ${TARGETARCH} below resolves per platform so one buildx invocation
# pulls the right binary for each arch.

# ----- release: minimal distroless image with the compiled binary -----
#
# Binary lands at /${BINARY} so the deployment manifest's
# /<component-name> command resolves.
FROM gcr.io/distroless/static:nonroot AS release
ARG TARGETARCH
ARG BINARY
ARG GIT_REVISION=
ARG GIT_TAG=
ARG BUILD_CREATED=
LABEL org.opencontainers.image.revision="${GIT_REVISION}" \
      org.opencontainers.image.version="${GIT_TAG}" \
      org.opencontainers.image.created="${BUILD_CREATED}"
COPY ${TARGETARCH}/${BINARY} /${BINARY}
USER 65532:65532
