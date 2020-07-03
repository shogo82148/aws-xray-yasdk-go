#!/bin/bash

set -uex

CURRENT=$(cd "$(dirname "$0")" && pwd)
VERSION=$1
MAJOR=$(echo "$VERSION" | cut -d. -f1)
MINOR=$(echo "$VERSION" | cut -d. -f2)
PATCH=$(echo "$VERSION" | cut -d. -f3)

# update metadata
cat <<EOF > "$CURRENT/xray/version.go"
package xray

// Version records the current X-Ray Go SDK version.
const Version = "$MAJOR.$MINOR.$PATCH"
EOF
git add "$CURRENT/xray/version.go"
git commit -m "bump up v$MAJOR.$MINOR.$PATCH"
git tag "v$MAJOR.$MINOR.$PATCH"
git push --tags
