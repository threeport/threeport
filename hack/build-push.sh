#! /bin/bash

./bin/tptdev build -r $TEST_REPO -t $TEST_TAG --push --names aws-controller \
    && ./bin/tptdev build -r $TEST_REPO -t $TEST_TAG --push --names control-plane-controller \
    && ./bin/tptdev build -r $TEST_REPO -t $TEST_TAG --push --names gateway-controller \
    && ./bin/tptdev build -r $TEST_REPO -t $TEST_TAG --push --names kubernetes-runtime-controller \
    && ./bin/tptdev build -r $TEST_REPO -t $TEST_TAG --push --names rest-api \
    && ./bin/tptdev build -r $TEST_REPO -t $TEST_TAG --push --names workload-controller

