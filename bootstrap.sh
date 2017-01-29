#!/bin/bash
# downloads the initial tool and executes it; intended to be checked in to
# repositories, so users just need to "git clone ...; ./somecommand.sh"
# must use bash for $BASH_SOURCE

set -euf -o pipefail

BASE_URL=https://storage.googleapis.com/cliupdater/helloupdater
BIN_SCRIPT_RELATIVE=devenv/helloupdater

# get the dir this script is in
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
echo SCRIPT_DIR: ${SCRIPT_DIR}
BIN_PATH=${SCRIPT_DIR}/${BIN_SCRIPT_RELATIVE}
echo BIN_PATH: ${BIN_PATH}

# if the binary exists: execute it
if [[ -e ${BIN_PATH} ]]; then
  exec ${BIN_PATH} $@
fi

# It does not exist: download the appropriate one
OS_ARCH=`uname -s`-`uname -m`
echo OS_ARCH: ${OS_ARCH}

URL=${BASE_URL}-${OS_ARCH}
echo URL: ${URL}
curl --fail --compressed --create-dirs --output ${BIN_PATH} ${URL}
chmod a+x ${BIN_PATH}
exec ${BIN_PATH} $@
