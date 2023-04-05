REST_API_IMG ?= threeport-rest-api:latest
WORKLOAD_CONTROLLER_IMG ?= threeport-workload-controller:latest

#help: @ List available make targets
help:
	@clear
	@echo "Usage: make COMMAND"
	@echo "Commands :"
	@grep -E '[a-zA-Z\.\-]+:.*?@ .*$$' $(MAKEFILE_LIST)| tr -d '#' | awk 'BEGIN {FS = ":.*?@ "}; {printf "\033[32m%-19s\033[0m - %s\n", $$1, $$2}'

#build-codegen: @ Build codegen binary
build-codegen:
	go build -o bin/threeport-codegen cmd/codegen/main.go

#generate: @ Run code generation
generate: build-codegen
	go generate ./...

#build-tptdev: @ Build tptdev binary
build-tptdev:
	go build -a -o bin/tptdev cmd/tptdev/main.go

#build-tptctl: @ Build tptctl binary
build-tptctl:
	go build -a -o bin/tptctl cmd/tptctl/main.go

release:
ifndef RELEASE_VERSION
	@echo "RELEASE_VERSION environment variable not set"
	exit 1
endif
	@echo -n "Are you sure you want to release version ${RELEASE_VERSION} of threeport? [y/n] " && read ans && [ $${ans:-n} = y ]
	@echo ${RELEASE_VERSION} > internal/version/version.txt
	@echo sed -i "/\/\/ @version/c\\/\/ @version ${RELEASE_VERSION}" cmd/rest-api/main.go
	@git add internal/version/version.txt
	@git add cmd/rest-api/main.go
	@git commit -s -m "release: cut version ${RELEASE_VERSION}"
	@git tag ${RELEASE_VERSION}
	@git push origin main --tag
	@echo "version ${RELEASE_VERSION} released"

## dev environment targets

#dev-up: @ Run a local development environment
dev-up: build-tptdev
	./bin/tptdev up

#dev-down: @ Delete the local development environment
dev-down: build-tptdev
	./bin/tptdev down

#dev-forward-api: @ Forward local port 1323 to the local dev API
dev-forward-api:
	kubectl port-forward -n threeport-control-plane service/threeport-api-server 1323:80

##dev-query-cr: @ Open a terminal connection to the dev cockroach database
#dev-query-cr:
#	cockroach sql --host $$(kubectl get svc threeport-api-db -n threeport-control-plane -ojson | jq '.status.loadBalancer.ingress[0].ip' -r) --insecure --database threeport_api

## container image builds

#rest-api-image: @ Build REST API container image
rest-api-image:
	docker build -t $(REST_API_IMG) -f cmd/rest-api/image/Dockerfile .

#workload-controller-image: @ Build workload controller container image
workload-controller-image:
	docker build -t $(WORKLOAD_CONTROLLER_IMG) -f cmd/workload-controller/image/Dockerfile .

