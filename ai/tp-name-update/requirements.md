# Requirements for Updating Workload Names and Refrences in tptctl

## Update 1
In `pkg/config/v0/kubernetes_workload.go` on line 17, the key `Workload` needs to be updated to `KubernetesWorkload`. All refrences to this key need to be updated aswell.

## Update 2
In `cmd/tptctl/cmd/kubernetes_workload.go` on line 114, the command should use the new name `kubernetes-workload`. All instances of the old name should be updated in this file aswell. For example, the `Aliases` value use `workload` but should be updated to `kubernetes-workload`.