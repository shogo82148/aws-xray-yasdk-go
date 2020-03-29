#!/bin/bash

CURRENT=$(cd "$(dirname "$0")" && pwd)
curl -sSL https://raw.githubusercontent.com/aws/aws-xray-sdk-python/master/aws_xray_sdk/ext/resources/aws_para_whitelist.json \
    | jq --sort-keys > "$CURRENT/AWSWhitelist.json"
