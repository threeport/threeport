# tptdev

Manage development operations with ease.

Here you will find the main package for `tptdev` which is a developer tool to
help make developer's life easier.  Currently it supports spinning up and down
development environments of the threeport control plane.  The dev environment
differs from a regular instance of threeport in that the API and controller
components are created with your local code repo mounted and will live-reload
changes to the code in your dev environment.

If you find yourself writing scripts or complex make targets for common
development tasks, it may warrant a new command for `tptdev`.

