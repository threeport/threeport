#! /bin/bash

docker buildx build \
    --build-arg BINARY=terraform-controller \
    --target terraform \
    --load \
    --platform=linux/amd64 \
    -t richlander2k2/threeport-terraform-controller:test \
    -f cmd/tptdev/image/Dockerfile \
    /Users/lander2k2/Projects/src/github.com/threeport/threeport

#docker push richlander2k2/threeport-terraform-controller:test
kind load docker-image richlander2k2/threeport-terraform-controller:test --name threeport-dev-0
