#!/bin/bash
set -o errexit

OPERATION=$1
CLUSTER_NAME=$2

REG_NAME='local-registry'
REG_PORT='5001'
REGISTRY_DIR="/etc/containerd/certs.d/localhost:${REG_PORT}"

create () {
    # create registry container unless it already exists
    if [ "$(docker inspect -f '{{.State.Running}}' "${REG_NAME}" 2>/dev/null || true)" != 'true' ]; then
        docker run \
            -d --restart=always -p "127.0.0.1:${REG_PORT}:5000" --network bridge --name "${REG_NAME}" \
            registry:2
    fi
}

connect () {
    # add the registry config to the nodes
    for node in $(kind get nodes -n "${CLUSTER_NAME}"); do
        docker exec "${node}" mkdir -p "${REGISTRY_DIR}"
        cat <<EOF | docker exec -i "${node}" cp /dev/stdin "${REGISTRY_DIR}/hosts.toml"
[host."http://${REG_NAME}:5000"]
EOF
    done

    # connect the registry to the cluster network if not already connected
    if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${REG_NAME}")" = 'null' ]; then
        docker network connect "kind" "${REG_NAME}"
    fi

    # document the local registry
    # https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${REG_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
}

delete () {
    docker stop $REG_NAME
    docker rm $REG_NAME
}

if [ "$OPERATION" == "create" ]; then
    create
elif [ "$OPERATION" == "connect" ]; then
    connect
elif [ "$OPERATION" == "delete" ]; then
    delete
else
    echo "Error: unrecognized operation $OPERATION"
    exit 1
fi

