# tptctl

Manage workloads in Threeport from the command line.

Here you will find the main package for `tptctl` along with the commands
available in that tool.  It is the primary client tool for the threeport control
plane.

It makes use of the client library in `pkg/client` for interactions with the API
and uses the config packages in `pkg/config` to provide the config abstractions
for users that relieve the user from managing much of the detail needed to
create, update and delete API objects.

It references a user's threeport config and provides the ability to switch
between using different instances of the threeport control plane.

