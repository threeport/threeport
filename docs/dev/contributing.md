# Contributing to Threeport

We're excited that you're interested in contributing to Threeport! This document
provides guidelines for making contributions to this project.

## Code of Conduct

Before contributing, please read our [Code of Conduct](code-of-conduct.md). We
expect all contributors to abide by its principles to foster a welcoming and
respectful community.

## Getting Started

Check out the [Threeport project scope](project-scope.md) to ensure what you
want to work on belongs in Threeport.

Fork and Clone: Fork the project on GitHub and clone your fork locally.

Set Up Your Environment: Follow the instructions in our
[Quickstart](quickstart.md) to set up your development environment.

## Making Contributions

Pick an Issue: Start with an open issue. Feel free to ask questions in the issue
thread if you need clarification.

Create a Branch: Create a new branch in your fork for your contribution.  Create
the new branch from the latest feature branch.  The feature branches are named
according to the next release version.  For example, if the current latest release of Threeport
is `v0.5.*`, the feature branch will be called `0.6`.  All changes are made to
the feature branch and merged into `main` at release time.  If a bug fix needs
to be applied to both the feature branch and the latest release, the bug fix
commit must be cherry-picked onto main for a bug fix release as covered in the
[release docs](release.md#bug-fixes).

Commit Your Changes: Make your changes in your branch and commit them. Write
clear, concise commit messages that explain your changes.

Write Tests: If you are adding new functionality or fixing a bug, write tests
that cover your changes.

Follow the Style Guide: Ensure your code adheres to the project's [style
guide](style-guide.md).  Run any linters or formatting tools the project uses.

Update Documentation: If your changes require it, update the [User
Documentation](../) or the [Developer
Documentation](README.md).

Run Tests: Before submitting your changes, run the end-to-end tests to
ensure that your changes don't break anything.  See the [Testing
instructions](testing.md).

Create a Pull Request: Push your changes to your fork and open a pull request
to the main project. Include an explanation of your changes.

## Review Process

Once you submit a pull request, maintainers will review your changes. They might
request some changes or improvements. Keep an eye on your pull request and
respond promptly to feedback.

## Community

We use [Discord](https://discord.com/invite/Fwr2sc9Dfp) for announcements, help
and general discussion.  Join us there to stay up to date.

Thank you for contributing to Threeport!

