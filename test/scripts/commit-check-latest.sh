#!/usr/bin/env bash

COMMIT_MSG=`git log -n 1 --pretty=format:"%s"`

# Subject line only (%s); same conventional header as .github/workflows/commit-messages.yml.
pattern='^((build|chore|ci|dev|docs|feat|fix|perf|refactor|release|revert|style|test)(\([^)]+\))?(!)?: .+)|(Merge .+)|(Initial commit)$'
if [[ ! "$COMMIT_MSG" =~ $pattern ]]; then
    echo "commit message check failed:"
    echo
    echo "${COMMIT_MSG}"
    echo
    echo "message is not conventional commits format"
    echo "please see https://www.conventionalcommits.org/en/v1.0.0/#specification"

    exit 1
fi
