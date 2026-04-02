#!/bin/bash
#
# Shrinks PNG files to a max height of 1200px
# Usage: ./shrink-png.sh file1.png file2.png ...
#

MAX_HEIGHT=1200

if [ $# -eq 0 ]; then
    echo "Usage: $0 file1.png file2.png ..."
    exit 1
fi

for file in "$@"; do
    if [ ! -f "$file" ]; then
        echo "File not found: $file"
        continue
    fi
    
    height=$(identify -format "%h" "$file")
    if [ "$height" -gt "$MAX_HEIGHT" ]; then
        echo "Shrinking $file (${height}px -> ${MAX_HEIGHT}px)"
        convert "$file" -resize "x${MAX_HEIGHT}" "$file"
    else
        echo "Skipping $file (${height}px <= ${MAX_HEIGHT}px)"
    fi
done
