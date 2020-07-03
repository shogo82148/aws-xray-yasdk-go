#!/bin/bash

set -uex

VERSION=$1
MAJOR=$(echo "$VERSION" | cut -d. -f1)
MINOR=$(echo "$VERSION" | cut -d. -f2)
PATCH=$(echo "$VERSION" | cut -d. -f3)

git tag "xrayaws/v$MAJOR.$MINOR.$PATCH"
git push --tags
