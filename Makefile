REST_API_IMG ?= threeport-rest-api:latest
WORKLOAD_CONTROLLER_IMG ?= threeport-workload-controller:latest
KUBERNETES_RUNTIME_CONTROLLER_IMG ?= threeport-kubernetes-runtime-controller:latest
AWS_CONTROLLER_IMG ?= threeport-aws-controller:latest
GATEWAY_CONTROLLER_IMG ?= threeport-gateway-controller:latest
AGENT_IMG ?= threeport-agent:latest

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

#test-build-agent: @ Build threeport agent for container build
test-build-agent:
	CGO_ENABLED=0 GOOS=linux go build -a -o bin/threeport-agent cmd/agent/main.go

#remote-build-agent: @ Build threeport agent for container build
remote-build-agent:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/remote/threeport-agent cmd/agent/main.go

#test-build-aws-controller: @ Build threeport aws controller for container build
test-build-aws-controller:
	CGO_ENABLED=0 GOOS=linux go build -a -o bin/threeport-aws-controller cmd/aws-controller/main_gen.go

#test-build-gateway-controller: @ Build threeport gateway controller for container build
test-build-gateway-controller:
	CGO_ENABLED=0 GOOS=linux go build -a -o bin/threeport-gateway-controller cmd/gateway-controller/main_gen.go

#test-build-kubernetes-runtime-controller: @ Build threeport kubernetes runtime controller for container build
test-build-kubernetes-runtime-controller:
	CGO_ENABLED=0 GOOS=linux go build -a -o bin/threeport-kubernetes-runtime-controller cmd/kubernetes-runtime-controller/main_gen.go

#test-build-rest-api: @ Build threeport REST API for container build
test-build-rest-api:
	CGO_ENABLED=0 GOOS=linux go build -a -o bin/threeport-rest-api cmd/rest-api/main.go

#test-build-workload-controller: @ Build threeport workload controller for container build
test-build-workload-controller:
	CGO_ENABLED=0 GOOS=linux go build -a -o bin/threeport-workload-controller cmd/workload-controller/main_gen.go

#test-build-control-plane: @ Build all control plane images for container builds
test-build-control-plane: test-build-agent test-build-aws-controller test-build-gateway-controller test-build-kubernetes-runtime-controller test-build-rest-api test-build-workload-controller

## code generation

#generate: @ Run code generation
generate: generate-code generate-docs

#generate-code: @ Generate code
generate-code: build-codegen
	go generate ./...

#generate-docs: @ Generate swagger docs
generate-docs:
	swag init --dir cmd/rest-api,pkg/api,internal/api --parseDependency --generalInfo main.go --output internal/api/docs

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
	./bin/tptdev up --auth-enabled=false

#dev-down: @ Delete the local development environment
dev-down: build-tptdev
	./bin/tptdev down

#dev-logs-api: @ Follow log output from the local dev API
dev-logs-api:
	kubectl logs deploy/threeport-api-server -n threeport-control-plane -f

#dev-logs-wrk: @ Follow log output from the local dev workload controller
dev-logs-wrk:
	kubectl logs deploy/threeport-workload-controller -n threeport-control-plane -f

#dev-logs-gw: @ Follow log output from the local dev gateway controller
dev-logs-gw:
	kubectl logs deploy/threeport-gateway-controller -n threeport-control-plane -f

#dev-logs-wrk: @ Follow log output from the local dev kubernetes runtime controller
dev-logs-kr:
	kubectl logs deploy/threeport-kubernetes-runtime-controller -n threeport-control-plane -f

#dev-logs-aws: @ Follow log output from the local dev aws controller
dev-logs-aws:
	kubectl logs deploy/threeport-aws-controller -n threeport-control-plane -f

#dev-logs-agent: @ Follow log output from the local dev agent
dev-logs-agent:
	kubectl logs deploy/threeport-agent -n threeport-control-plane -f -c manager

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
	kubectl exec -it -n threeport-control-plane crdb-0 -- cockroach sql --host localhost --insecure --database threeport_api

#dev-reset-crdb: @ Reset the dev cockroach database
dev-reset-crdb:
	kubectl exec -it -n threeport-control-plane crdb-0 -- cockroach sql --host localhost --insecure --database threeport_api \
	--execute "TRUNCATE attached_object_references, \
		workload_events, \
		workload_definitions, \
		workload_resource_definitions, \
		workload_instances, \
		workload_resource_instances, \
		gateway_instances, \
		gateway_definitions, \
		domain_name_definitions, \
		domain_name_instances; \
		set sql_safe_updates = false; \
		update kubernetes_runtime_instances set gateway_controller_instance_id = NULL; \
		update kubernetes_runtime_instances set dns_controller_instance_id = NULL; \
		set sql_safe_updates = true;"

#TODO: move to kubectl exec command that uses `nats` binary in contianer
#dev-sub-nats: @ Subscribe to all messages from nats server locally (must first run `make dev-forward-nats` in another terminal)
dev-sub-nats:
	nats sub -s "nats://127.0.0.1:4222" ">"

#dev-debug-api: @ Start debugging session for API (must first run `make dev-forward-nats` in another terminal)
dev-debug-api:
	dlv debug cmd/rest-api/main.go -- -env-file hack/env -auto-migrate -verbose

#dev-debug-wrk: @ Start debugging session for workload-controller (must first run `make dev-forward-nats` in another terminal)
dev-debug-wrk:
	dlv debug cmd/workload-controller/main_gen.go -- -auth-enabled=false -api-server=localhost:1323 -msg-broker-host=localhost -msg-broker-port=4222

#dev-debug-gateway: @ Start debugging session for workload-controller (must first run `make dev-forward-nats` in another terminal)
dev-debug-gateway:
	dlv debug --build-flags cmd/gateway-controller/main_gen.go -- -auth-enabled=false -api-server=localhost:1323 -msg-broker-host=localhost -msg-broker-port=4222

## container image builds and pushes

#rest-api-image-build: @ Build REST API container image
rest-api-image-build:
	docker buildx build --platform linux/amd64 -t $(REST_API_IMG) -f cmd/rest-api/image/Dockerfile-test .

#workload-controller-image-build: @ Build workload controller container image
workload-controller-image-build:
	docker buildx build --platform linux/amd64 -t $(WORKLOAD_CONTROLLER_IMG) -f cmd/workload-controller/image/Dockerfile-test .

#kubernetes-runtime-controller-image-build: @ Build kubernetes runtime controller container image
kubernetes-runtime-controller-image-build:
	docker buildx build --platform linux/amd64 -t $(KUBERNETES_RUNTIME_CONTROLLER_IMG) -f cmd/kubernetes-runtime-controller/image/Dockerfile-test .

#aws-controller-image-build: @ Build aws controller container image
aws-controller-image-build:
	docker buildx build --platform linux/amd64 -t $(AWS_CONTROLLER_IMG) -f cmd/aws-controller/image/Dockerfile-test .

#gateway-image-build: @ Build gateway controller container image
gateway-controller-image-build:
	docker buildx build --platform linux/amd64 -t $(GATEWAY_CONTROLLER_IMG) -f cmd/gateway-controller/image/Dockerfile-test .

#agent-image-build: @ Build agent container image
agent-image-build:
	docker buildx build --platform linux/amd64 -t $(AGENT_IMG) -f cmd/agent/image/Dockerfile-test .

#agent-image-build-remote: @ Build agent container image
agent-image-build-remote:
	docker buildx build --build-arg BIN=bin/remote --platform linux/amd64 -t $(AGENT_IMG)-remote -f cmd/agent/image/Dockerfile-test .

#control-plane-images-build: @ Build all control plane images
control-plane-images-build: rest-api-image-build workload-controller-image-build kubernetes-runtime-controller-image-build aws-controller-image-build gateway-controller-image-build agent-image-build

#rest-api-image-push: @ Push REST API container image
rest-api-image-push:
	docker push $(REST_API_IMG)

#workload-controller-image-push: @ Push workload controller container image
workload-controller-image-push:
	docker push $(WORKLOAD_CONTROLLER_IMG)

#kubernetes-runtime-controller-image-push: @ Push kubernetes runtime controller container image
kubernetes-runtime-controller-image-push:
	docker push $(KUBERNETES_RUNTIME_CONTROLLER_IMG)

#aws-controller-image-push: @ Push aws controller container image
aws-controller-image-push:
	docker push $(AWS_CONTROLLER_IMG)

#gateway-controller-image-push: @ Push gateway controller container image
gateway-controller-image-push:
	docker push $(GATEWAY_CONTROLLER_IMG)

#agent-image-push: @ Push agent container image
agent-image-push:
	docker push $(AGENT_IMG)

#agent-image-push: @ Push agent container image
agent-image-push-remote:
	docker push $(AGENT_IMG)-remote

#control-plane-images-push: @ Push all control plane images
control-plane-images-push: rest-api-image-push workload-controller-image-push kubernetes-runtime-controller-image-push aws-controller-image-push gateway-controller-image-push agent-image-push

#rest-api-image: @ Build and push REST API container image
rest-api-image: rest-api-image-build rest-api-image-push

#workload-controller-image: @ Build and push workload controller container image
workload-controller-image: workload-controller-image-build workload-controller-image-push

#kubernetes-runtime-controller-image: @ Build and push kubernetes runtime controller container image
kubernetes-runtime-controller-image: kubernetes-runtime-controller-image-build kubernetes-runtime-controller-image-push

#aws-controller-image: @ Build and push aws controller container image
aws-controller-image: aws-controller-image-build aws-controller-image-push

#gateway-controller-image: @ Build and push gateway controller container image
gateway-controller-image: gateway-controller-image-build gateway-controller-image-push

#agent-image: @ Build and push agent container image
agent-image: agent-image-build agent-image-push

