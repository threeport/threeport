REST_API_IMG ?= threeport-rest-api:latest
WORKLOAD_CONTROLLER_IMG ?= threeport-workload-controller:latest

#help: @ List available make targets
help:
	@clear
	@echo "Usage: make <target>"
	@echo "Commands :"
	@grep -E '[a-zA-Z\.\-]+:.*?@ .*$$' $(MAKEFILE_LIST)| tr -d '#' | awk 'BEGIN {FS = ":.*?@ "}; {printf "\033[32m%-19s\033[0m - %s\n", $$1, $$2}'

## builds

#build-codegen: @ Build codegen binary
build-codegen:
	go build -o bin/threeport-codegen cmd/codegen/main.go

#build-tptdev: @ Build tptdev binary
build-tptdev:
	go build -o bin/tptdev cmd/tptdev/main.go

#build-tptctl: @ Build tptctl binary
build-tptctl:
	go build -o bin/tptctl cmd/tptctl/main.go

## code generation

#generate: @ Run code generation
generate: build-codegen
	go generate ./...

## testing

#test: @ Run automated tests
tests:
	go test -v ./... -count=1

#test-commit: @ Check to make sure commit messages follow conventional commits format
test-commit:
	test/scripts/commit-check-latest.sh

## release

#release: @ Set a new version of threeport, commit, tag and push to origin.  Will trigger CI for new release of threeport.
release:
ifndef RELEASE_VERSION
	@echo "RELEASE_VERSION environment variable not set"
	exit 1
endif
	@echo -n "Are you sure you want to release version ${RELEASE_VERSION} of threeport? [y/n] " && read ans && [ $${ans:-n} = y ]
	@echo ${RELEASE_VERSION} > internal/version/version.txt
	@sed -i "/\/\/ @version/c\\/\/ @version ${RELEASE_VERSION}" cmd/rest-api/main.go
	@git add internal/version/version.txt
	@git add cmd/rest-api/main.go
	@git commit -s -m "release: cut version ${RELEASE_VERSION}"
	@git tag ${RELEASE_VERSION}
	@git push origin main --tag
	@echo "version ${RELEASE_VERSION} released"

## dev environment

#dev-up: @ Run a local development environment
dev-up: build-tptdev
	./bin/tptdev up --auth-enabled=true

#dev-down: @ Delete the local development environment
dev-down: build-tptdev
	./bin/tptdev down

#dev-logs-api: @ Follow log output from the local dev API
dev-logs-api:
	kubectl logs deploy/threeport-api-server -n threeport-control-plane -f

#dev-logs-wrk: @ Follow log output from the local dev workload controller
dev-logs-wrk:
	kubectl logs deploy/threeport-workload-controller -n threeport-control-plane -f

#dev-logs-agent: @ Follow log output from the local dev agent
dev-logs-agent:
	kubectl logs deploy/threeport-agent -n threeport-control-plane -f

#dev-forward-api: @ Forward local port 1323 to the local dev API
dev-forward-api:
	kubectl port-forward -n threeport-control-plane service/threeport-api-server 1323:80

#dev-forward-crdb: @ Forward local port 26257 to local dev cockroach database
dev-forward-crdb:
	kubectl port-forward -n threeport-control-plane service/crdb 26257

#dev-forward-nats: @ Forward local port 33993 to the local dev API nats server
dev-forward-nats:
	kubectl port-forward -n threeport-control-plane service/nats-js 4222:4222

#TODO: move to kubectl exec command that uses `cockroach` binary in contianer
#dev-query-crdb: @ Open a terminal connection to the dev cockroach database (must first run `make dev-forward-crdb` in another terminal)
dev-query-crdb:
	cockroach sql --host localhost --insecure --database threeport_api

#TODO: move to kubectl exec command that uses `nats` binary in contianer
#dev-sub-nats: @ Subscribe to all messages from nats server locally (must first run `make dev-forward-nats` in another terminal)
dev-sub-nats:
	nats sub -s "nats://127.0.0.1:4222" ">"

#dev-debug-api: @ Start debugging session for API (must first run `make dev-forward-nats` in another terminal)
dev-debug-api:
	dlv debug cmd/rest-api/main.go -- -env-file hack/env -auto-migrate -verbose

#dev-debug-wrk: @ Start debugging session for workload-controller (must first run `make dev-forward-nats` in another terminal)
dev-debug-wrk:
	dlv debug cmd/workload-controller/main_gen.go -- -api-server http://localhost:1323 -msg-broker-host localhost -msg-broker-port 4222

## container image builds

#rest-api-image-build: @ Build REST API container image
rest-api-image-build:
	docker build -t $(REST_API_IMG) -f cmd/rest-api/image/Dockerfile .

#workload-controller-image-build: @ Build workload controller container image
workload-controller-image-build:
	docker build -t $(WORKLOAD_CONTROLLER_IMG) -f cmd/workload-controller/image/Dockerfile .

#agent-image-build: @ Build agent container image
agent-image-build:
	docker build -t $(AGENT_IMG) -f cmd/agent/image/Dockerfile .

#rest-api-image: @ Build and push REST API container image
rest-api-image: rest-api-image-build
	docker push $(REST_API_IMG)

#workload-controller-image: @ Build and push workload controller container image
workload-controller-image: workload-controller-image-build
	docker push $(WORKLOAD_CONTROLLER_IMG)

#agent-image: @ Build and push agent container image
agent-image: agent-image-build
	docker push $(AGENT_IMG)

