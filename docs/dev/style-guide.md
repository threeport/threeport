# Developer Style Guide

The following style guide exists to promote consistency and readability in the
Threeport codebase.  As with most rules, there are exceptions.  And there are
legacy violations of this style guide that we hope to resolve over time.  Prioritize
code simplicity, readability and useful commenting over any style guidance.

## Imports

Group imports in the following order:

1. Standard library
1. 3rd party packages
1. Packages from this project

When using aliases, use snake case, e.g. `aws_builder` rather than `awsBuilder`
or `awsbuilder`.

## Commit Messages

Use the [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/)
standard for commit messages.  These are enforced in CI and the allowable
commit types can be found in the [commit message
check](.github/workflows/commit-messages.yml) CI config.

Keep in mind that `feat` and `fix` commit types will be automatically included
in the release notes so make them understandable for users.

Use imperative statements in your commit messages, e.g. "correct flag validation
for tptctl up command" instead of "fixed tptctl up command flag validation".

## Comments

See the [Go Doc Comments guide](https://tip.golang.org/doc/comment).

At a minimum, all types and funcitons should be commented, begin the
comment with the type/function name and use complete sentences with
punctuation.

Add comments using full setences and punctuation for all fields in a type
definition (excluding anonymous fields).  Use langauage that Threeport users
might use so that we can leverage LLM's to help users construct API calls and
use the system.

Use lowercase comments without punctuation for general-purpose explanations and
helper comments.  Keep wording as concise as is practical while still being
helpful.

## Naming

Use the Go convention of camel case or lower camel case for naming types,
functions, variables, etc.  Do not use all caps for acronyms, e.g. use
`inputJson` instead of `inputJSON`.

## Merges

When merging commits, squash all commits within the PR.

