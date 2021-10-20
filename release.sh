#!/bin/bash

set -uex

CURRENT=$(cd "$(dirname "$0")" && pwd)
VERSION=${1#v}
MAJOR=$(echo "$VERSION" | cut -d. -f1)
MINOR=$(echo "$VERSION" | cut -d. -f2)
PATCH=$(echo "$VERSION" | cut -d. -f3)
git switch main

# update metadata
cat <<EOF > "$CURRENT/xray/version.go"
package xray

// Version records the current X-Ray Go SDK version.
const Version = "$MAJOR.$MINOR.$PATCH"
EOF
git add "$CURRENT/xray/version.go"
git commit -m "bump up v$MAJOR.$MINOR.$PATCH"
git tag "v$MAJOR.$MINOR.$PATCH"
git push origin "v$MAJOR.$MINOR.$PATCH"

# update xrayaws
cd "$CURRENT/xrayaws"
go get "github.com/shogo82148/aws-xray-yasdk-go@v$MAJOR.$MINOR.$PATCH"
go mod tidy

# update xrayaws-v2
cd "$CURRENT/xrayaws-v2"
go get "github.com/shogo82148/aws-xray-yasdk-go@v$MAJOR.$MINOR.$PATCH"
go mod tidy

git add .
git commit -m "bump aws-xray-yasdk-go v$MAJOR.$MINOR.$PATCH"
git push origin main
