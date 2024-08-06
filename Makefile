#help: @ List available make targets
help:
	@clear
	@echo "Usage: make <target>"
	@echo "Commands :"
	@grep -E '[a-zA-Z\.\-]+:.*?@ .*$$' $(MAKEFILE_LIST)| tr -d '#' | awk 'BEGIN {FS = ":.*?@ "}; {printf "\033[32m%-19s\033[0m - %s\n", $$1, $$2}'

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

#TODO: move to kubectl exec command that uses `cockroach` binary in contianer
#dev-query-crdb: @ Open a terminal connection to the dev cockroach database (must first run `make dev-forward-crdb` in another terminal)
dev-query-crdb:
	kubectl exec -it -n threeport-control-plane crdb-0 -- cockroach sql --host localhost --insecure --database threeport_api

#dev-reset-crdb: @ Reset the dev cockroach database
dev-reset-crdb:
	kubectl exec -it -n threeport-control-plane crdb-0 -- cockroach sql --host localhost --insecure --database threeport_api \
	--execute "TRUNCATE attached_object_references, \
		workload_events, \
		helm_workload_definitions, \
		helm_workload_instances, \
		workload_definitions, \
		workload_resource_definitions, \
		workload_instances, \
		workload_resource_instances, \
		gateway_http_ports, \
		gateway_tcp_ports, \
		gateway_instances, \
		gateway_definitions, \
		domain_name_definitions, \
		domain_name_instances, \
		metrics_instances, \
		metrics_definitions, \
		logging_instances, \
		logging_definitions, \
		observability_dashboard_instances, \
		observability_dashboard_definitions, \
		observability_stack_instances, \
		observability_stack_definitions, \
		secret_instances, \
		secret_definitions; \
		set sql_safe_updates = false; \
		update kubernetes_runtime_instances set gateway_controller_instance_id = NULL; \
		update kubernetes_runtime_instances set dns_controller_instance_id = NULL; \
		update kubernetes_runtime_instances set secrets_controller_instance_id = NULL; \
		set sql_safe_updates = true; \
		DELETE FROM control_plane_definitions WHERE name != 'dev-0'; \
		DELETE FROM control_plane_instances WHERE name != 'dev-0'; \
		DELETE FROM control_plane_components WHERE name != 'dev-0';" \

#dev-purge-streams: @ Purge all nats streams
dev-purge-streams:
	nats stream ls --names | xargs -I {} nats stream purge {} --force

#dev-uninstall-helm: @ Uninstall all helm releases
dev-uninstall-helm:
	helm ls -A --short | xargs -I {} helm uninstall --namespace threeport-control-plane {}

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
