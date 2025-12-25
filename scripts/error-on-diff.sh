#!/usr/bin/env sh

set -xe

# Create a temporary directory and delete it once the script is done
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Create a copy of all *tracked* files in the temporary directory
git checkout-index --all --prefix "$TMP/"
cd "$TMP"

# Initialize as git repo to enable diff
git init . 2>/dev/null
git add . 2>/dev/null

# Run the given command, that may make changes to the source.
"$@"

# Print all changes and return with a non zero error if anything changed.
git diff --exit-code --ignore-space-at-eol
