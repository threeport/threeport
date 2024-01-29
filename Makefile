#help: @ List available make targets
help:
	@clear
	@echo "Usage: make <target>"
	@echo "Commands :"
	@grep -E '[a-zA-Z\.\-]+:.*?@ .*$$' $(MAKEFILE_LIST)| tr -d '#' | awk 'BEGIN {FS = ":.*?@ "}; {printf "\033[32m%-19s\033[0m - %s\n", $$1, $$2}'

## builds

#install-codegen: @ Build codegen binary and install in GOPATH
install-codegen:
	go build -o $(GOPATH)/bin/threeport-codegen cmd/codegen/main.go

#build-database-migrator: @ Build database migrator
build-database-migrator:
	go build -o bin/database-migrator cmd/database-migrator/main.go

#build-tptdev: @ Build tptdev binary
build-tptdev:
	go build -o bin/tptdev cmd/tptdev/main.go

#install-tptdev: @ Install tptctl binary
install-tptdev: build-tptdev
	sudo cp ./bin/tptdev /usr/local/bin/tptdev

#build-tptctl: @ Build tptctl binary
build-tptctl:
	go build -o bin/tptctl cmd/tptctl/main.go

#install-tptctl: @ Install tptctl binary
install-tptctl: build-tptctl
	sudo cp ./bin/tptctl /usr/local/bin/tptctl

## code generation

#generate: @ Run code generation
generate: generate-code generate-docs

#generate-code: @ Generate code
generate-code: install-codegen
	go generate ./...

#generate-docs: @ Generate swagger docs
generate-docs:
	swag init --dir cmd/rest-api,pkg/api,pkg/api-server/v0 --parseDependency --generalInfo main.go --output pkg/api-server/v0/docs

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
	@awk -v version="// @version ${RELEASE_VERSION}" '/\/\/ @version/ {print version; next} 1' cmd/rest-api/main.go > tmpfile && mv tmpfile cmd/rest-api/main.go
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

#dev-logs-kr: @ Follow log output from the local dev kubernetes runtime controller
dev-logs-kr:
	kubectl logs deploy/threeport-kubernetes-runtime-controller -n threeport-control-plane -f

#dev-logs-aws: @ Follow log output from the local dev aws controller
dev-logs-aws:
	kubectl logs deploy/threeport-aws-controller -n threeport-control-plane -f

#dev-logs-cp: @ Follow log output from the local dev control plane controller
dev-logs-cp:
	kubectl logs deploy/threeport-control-plane-controller -n threeport-control-plane -f

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
		gateway_http_ports, \
		gateway_tcp_ports, \
		gateway_instances, \
		gateway_definitions, \
		domain_name_definitions, \
		domain_name_instances; \
		set sql_safe_updates = false; \
		update kubernetes_runtime_instances set gateway_controller_instance_id = NULL; \
		update kubernetes_runtime_instances set dns_controller_instance_id = NULL; \
		set sql_safe_updates = true; \
		DELETE FROM control_plane_definitions WHERE name != 'dev-0'; \
		DELETE FROM control_plane_instances WHERE name != 'dev-0'; \
		DELETE FROM control_plane_components WHERE name != 'dev-0';" \

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
