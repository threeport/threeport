#! /bin/bash

docker buildx build \
    --build-arg BINARY=radius-workload-controller \
    --target radius \
    --load \
    --platform=linux/amd64 \
    -t richlander2k2/threeport-radius-workload-controller:test \
    -f cmd/tptdev/image/Dockerfile \
    /Users/lander2k2/Projects/src/github.com/threeport/threeport

docker push richlander2k2/threeport-radius-workload-controller:test

