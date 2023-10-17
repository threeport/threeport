# Testing

Tests are run in CI and must pass before merging PRs to main.

## Unit Tests

TODO

## End-To-End

As a distributed system, theeport relies heavily on automated e2e testing.  It
is essential to maintaining stability in the control plane.

In order to run tests, you'll need a dev environment with the default control plane
name which is used with:

```bash
make dev-up
```

With a dev environment up, run all tests with:
```
make tests
```
