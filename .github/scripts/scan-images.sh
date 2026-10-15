#!/usr/bin/env bash
#
# Scan every component image in the local registry for known vulnerabilities.
#
# Reads the registry rather than taking a list of component names: the list
# changes when a component is added, and a scanner that silently stops
# covering a new image is worse than no scanner, because it still reports
# success.
#
# Advisory for now - see the comment at the top of .github/workflows/
# security.yml. The loop does not stop at the first image with a finding
# either: one run should report every image, not the first one that has
# something in it.

set -uo pipefail

registry="${REGISTRY:-localhost:5001}"
trivy_version="${TRIVY_VERSION:-0.76.0}"

repositories=$(curl -fsS "http://${registry}/v2/_catalog" | jq -r '.repositories[]')
if [ -z "$repositories" ]; then
  echo "no images found in ${registry}; nothing was built, so nothing is scanned"
  exit 1
fi

scanned=0
for repository in $repositories; do
  for tag in $(curl -fsS "http://${registry}/v2/${repository}/tags/list" | jq -r '.tags[]'); do
    echo "::group::${repository}:${tag}"
    docker run --rm --network host \
      -v /var/run/docker.sock:/var/run/docker.sock \
      "aquasec/trivy:${trivy_version}" image \
      --severity HIGH,CRITICAL \
      --ignore-unfixed \
      --ignorefile .trivyignore \
      --exit-code 0 \
      "${registry}/${repository}:${tag}"
    echo "::endgroup::"
    scanned=$((scanned + 1))
  done
done

echo "scanned ${scanned} image(s)"
