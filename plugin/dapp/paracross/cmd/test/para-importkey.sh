#!/usr/bin/env bash

BUILD_PATH="/root/go/src/github.com/33cn/plugin/build/ci/paracross"
PARA_CLI="${BUILD_PATH}/chain33-para-cli"


function para_import_key() {
    echo "=========== # save seed to wallet ============="
    ${1} seed save -p 1314 -s "tortoise main civil member grace happy century convince father cage beach hip maid merry rib"


    echo "=========== # unlock wallet ============="
    ${1} wallet unlock -p 1314 -t 0


    echo "=========== # import private key ============="

    ${1} account import_key -k "0x6da92a632ab7deb67d38c0f6560bcfed28167998f6496db64c258d5e8393a81b" -l returnAddr1
    ${1} account import_key -k "0x19c069234f9d3e61135fefbeb7791b149cdf6af536f26bebb310d4cd22c3fee4" -l returnAddr2
    ${1} account import_key -k "0x7a80a1f75d7360c6123c32a78ecf978c1ac55636f87892df38d8b85a9aeff115" -l returnAddr3
    ${1} account import_key -k "0xcacb1f5d51700aea07fca2246ab43b0917d70405c65edea9b5063d72eb5c6b71" -l returnAddr4


    echo "=========== # close auto mining ============="
    ${1} wallet auto_mine -f 0

    echo "=========== # wallet status ============="
    ${1} wallet status
}

function main() {
    echo "==========================================main begin========================================================"

    sleep 15
    para_import_key "${PARA_CLI}"

    echo "==========================================main end========================================================="
}

# run script
main
