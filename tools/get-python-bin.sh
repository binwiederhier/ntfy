#!/usr/bin/env bash

# Synopsis:
# - get-python-bin.sh python
# - get-python-bin.sh pip
#
# This script selects the most suitable `python` / `pip` binary available
# for building docs that's installed in this system.
#
# If no usable version of Python is available, this script will exit with a
# non-zero code.

set -e

case "$1" in
    "python" | "pip")
        BIN_PREFIX="$1"
        ;;
    *)
        echo "Incorrect usage" >&2
        exit 1
        ;;
esac

# if `python3` is >= 3.8 and `pip3` is available, use that
if echo -e "import sys\nif sys.version_info < (3,8):\n exit(1)" | python3 && \
which pip3 1>/dev/null 2>&1; then
    echo "${BIN_PREFIX}3"
    exit 0
fi

# check `python3.N` from newest to oldest
CANDIDATE_SUFFIXES=("3.11" "3.10" "3.9" "3.8")
for SUFFIX in ${CANDIDATE_SUFFIXES[@]}; do
    # if both `python3.N` and `pip3.N` are available, use that
    if which "python$SUFFIX" 1>/dev/null 2>&1 && \
    which "pip$SUFFIX" 1>/dev/null 2>&1; then
        echo "${BIN_PREFIX}${SUFFIX}"
        exit 0
    fi
done

# none found
echo "No suitable version of Python found" >&2
exit 2
