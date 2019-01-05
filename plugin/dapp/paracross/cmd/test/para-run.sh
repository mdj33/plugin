#!/usr/bin/env bash
#wait 300s for random of 5
sleep 300
PARA_PATH="/root/go/src/github.com/33cn/plugin/plugin/dapp/paracross/cmd/test"
BUILD_PATH="/root/go/src/github.com/33cn/plugin/build"
bash $PARA_PATH/para-importkey.sh &
$BUILD_PATH/chain33 -f "$PARA_PATH/$1"
