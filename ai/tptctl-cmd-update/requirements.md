# Requirements For Updating Tptctl Commands

The workload-definition and workload-instance subcommands still persist in tptctl:

Example:
https://github.com/threeport/threeport/blob/0.7/cmd/tptctl/cmd/kubernetes_workload.go#L381

All instances of this need to say kubernetes-workload-definition and kubernetes-workload-instance.