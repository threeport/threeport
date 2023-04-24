# tptctl

Manage workloads in Threeport from the command line.

Here you will find the main package for `tptctl` along with the commands
available in that tool.  It is the primary client tool for the threeport control
plane.  It provides configuration abstractions for users and makes calls to the
threeport API on the user's behalf to make changes in the system.

It makes use of the client library in `pkg/client` for interactions with the API
and uses the config packages in `pkg/config` to provide the config abstractions
for users that relieve the user from managing much of the detail needed to
create, update and delete API objects.

