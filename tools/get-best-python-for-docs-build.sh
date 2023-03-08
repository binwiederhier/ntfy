#!/usr/bin/env bash

# This script prints the version suffix of the most suitable version of Python
# for building docs that's installed in this system.
#
# Caller can safely append this suffix to `python` or `pip` and expect the
# binary to exist.
#
# If no usable version of Python is available, this script will exit with a
# non-zero code.

set -e

# if `python3` is >= 3.8 and `pip3` is available, use that
if echo -e "import sys\nif sys.version_info < (3,8):\n exit(1)" | python3 && \
which pip3 1>/dev/null 2>&1; then
    echo "3"
    exit 0
fi

# check `python3.N` from newest to oldest
CANDIDATE_SUFFIXES=("3.11" "3.10" "3.9" "3.8")
for SUFFIX in ${CANDIDATE_SUFFIXES[@]}; do
    # if both `python3.N` and `pip3.N` are available, use that
    if which "python$SUFFIX" 1>/dev/null 2>&1 && \
    which "pip$SUFFIX" 1>/dev/null 2>&1; then
        echo "$SUFFIX"
        exit 0
    fi
done

# none found
exit 1
