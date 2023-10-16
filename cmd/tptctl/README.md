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
between using different threeport control planes.



# threeport config

Your threeport config file that is used with tptctl is considered a logical container
for a group of control planes.

Since control planes can be deployed and managed by other control planes via the control plane
api, a tree like relationship exists between those managed by one another.
The root of this tree is considered the genesis control plane.

When a new control plane is spun up using `tptctl up`, you are orchestrating the creation
of a control plane that is considered a genesis control plane within a new group. Hence if the 
provided config file contains control planes within them, it will be considered
already populated with a previous control plane group, throwing back an error prompting you to choose 
a different empty config file.

Similiarly, `tptctl down` can only be used to spin down a control plane that is a genesis control
plane within the group. The recommended practice to properly clean up resources are to delete control
plane from the leaves to the root i.e. every parent control plane that is responsible other control planes
should reconcile there deletion before being deleted themselves by their own parent.
The genesis control plane can thus be brought down with `tptctl down` once the process reaches the root.