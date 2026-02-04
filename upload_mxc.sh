#!/bin/bash

SERVER="https://matrix.beeper.com"
TOKEN="syt_YmF0dWhhbg_MCBCEuxwFYIABHwYpIVw_1Flopr"

upload_file() {
    local file="$1"
    local content_type="$2"
    local filename=$(basename "$file")
    
    echo "Uploading $filename..."
    
    response=$(curl -s -X POST \
        "${SERVER}/_matrix/media/v3/upload?filename=${filename}" \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: ${content_type}" \
        --data-binary "@${file}")
    
    echo "Response: $response"
    echo ""
}

# Upload avatar.png
upload_file "avatar.png" "image/png"

# Upload avatar.svg
upload_file "avatar.svg" "image/svg+xml"
