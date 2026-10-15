#!/usr/bin/env bash
#
# Scan every component image in the local registry for known vulnerabilities.
#
# Reads the registry rather than taking a list of component names: the list
# changes when a component is added, and a scanner that silently stops
# covering a new image is worse than no scanner, because it still reports
# success.
#
# Vulnerability findings are advisory - see the comment at the top of
# .github/workflows/security.yml - but a scan that did not run is not a scan
# with no findings. Trivy failing to start, or to fetch its database, or the
# registry failing to answer, all end in a nonzero exit here so the step is
# visibly broken rather than quietly empty. --exit-code 0 keeps that separate
# from what Trivy actually found.
#
# The loop does not stop at the first failure: one run should report every
# image, not however many came before the first problem.

set -uo pipefail

registry="${REGISTRY:-localhost:5001}"
trivy_version="${TRIVY_VERSION:-0.76.0}"
ignore_file="${PWD}/.trivyignore"

# the ignore file lives in the checkout and trivy runs in a container, so it
# has to be mounted in. Passing a host path to --ignorefile is not a silent
# no-op: trivy exits FATAL with "ignore file not found".
ignore_args=()
if [ -f "$ignore_file" ]; then
  ignore_args=(-v "${ignore_file}:/.trivyignore:ro")
fi

# Trivy downloads a ~1GB vulnerability database on first use, and --rm throws
# it away with the container. Without somewhere to keep it, the database is
# fetched again for every image - twenty gigabytes of downloads per run,
# against a host that rate limits. A named volume keeps it across containers
# and across runs.
cache_volume="${TRIVY_CACHE_VOLUME:-trivy-db-cache}"

repositories=$(curl -fsS "http://${registry}/v2/_catalog" | jq -r '.repositories[]')
if [ -z "$repositories" ]; then
  echo "no images found in ${registry}; nothing was built, so nothing is scanned"
  exit 1
fi

scanned=0
failed=0
for repository in $repositories; do
  tags=$(curl -fsS "http://${registry}/v2/${repository}/tags/list" | jq -r '.tags[]')
  if [ -z "$tags" ]; then
    echo "could not list tags for ${repository}"
    failed=$((failed + 1))
    continue
  fi

  for tag in $tags; do
    echo "::group::${repository}:${tag}"
    if docker run --rm --network host \
      -v /var/run/docker.sock:/var/run/docker.sock \
      -v "${cache_volume}:/root/.cache/trivy" \
      "${ignore_args[@]}" \
      "aquasec/trivy:${trivy_version}" image \
      --severity HIGH,CRITICAL \
      --ignore-unfixed \
      ${ignore_args:+--ignorefile /.trivyignore} \
      --exit-code 0 \
      "${registry}/${repository}:${tag}"
    then
      scanned=$((scanned + 1))
    else
      echo "scan did not complete for ${repository}:${tag}"
      failed=$((failed + 1))
    fi
    echo "::endgroup::"
  done
done

echo "scanned ${scanned} image(s), ${failed} did not complete"
if [ "$failed" -gt 0 ]; then
  exit 1
fi
