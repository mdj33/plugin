#!/usr/bin/env bash
# shellcheck disable=SC2128

MAIN_HTTP=""
PARA_HTTP=""
CASE_ERR=""
UNIT_HTTP=""

# $2=0 means true, other false
echo_rst() {
    if [ "$2" -eq 0 ]; then
        echo "$1 ok"
    else
        echo "$1 err"
        CASE_ERR="err"
    fi

}

paracross_GetBlock2MainInfo() {
    local height=$(curl -s --data-binary '{"jsonrpc":"2.0","id":2,"method":"paracross.GetBlock2MainInfo","params":[{"start":1,"end":3}]}' -H 'content-type:text/plain;' ${UNIT_HTTP} | jq -r ".result.items[1].height")
    [ "$height" -eq 2 ]
    local rst=$?
    echo_rst "$FUNCNAME" "$rst"
}


chain33_lock() {
    local ok=$(curl -s --data-binary '{"jsonrpc":"2.0","id":2,"method":"Chain33.Lock","params":[]}' -H 'content-type:text/plain;' ${PARA_HTTP} | jq -r ".result.isOK")
    [ "$ok" == true ]
    local rst=$?
    echo_rst "$FUNCNAME" "$rst"
}

chain33_unlock() {
    local ok=$(curl -s --data-binary '{"jsonrpc":"2.0","id":2,"method":"Chain33.UnLock","params":[{"passwd":"1314fuzamei","timeout":0}]}' -H 'content-type:text/plain;' ${PARA_HTTP} | jq -r ".result.isOK")
    [ "$ok" == true ]
    local rst=$?
    echo_rst "$FUNCNAME" "$rst"

}


function paracross_SignAndSend() {
    local signedTx=$(curl -ksd '{"method":"Chain33.SignRawTx","params":[{"expire":"120s","fee":'$1',"privkey":"'$2'","txHex":"'$3'"}]}' ${PARA_HTTP} | jq -r ".result")
    #echo "signedTx:$signedTx"
    local sendedTx=$(curl -ksd '{"method":"Chain33.SendTransaction","params":[{"data":"'"$signedTx"'"}]}' ${PARA_HTTP})
    #echo "sendedTx:$sendedTx"
}

function paracross_QueryBalance() {
    local req='{"method":"Chain33.GetBalance", "params":[{"addresses" : ["'$1'"], "execer" : "paracross","asset_exec":"paracross","asset_symbol":"coins.bty"}]}'
    local resp=$(curl -ksd "$req" "${PARA_HTTP}")
    local balance=$(jq -r '.result[0].balance' <<<"$resp")
    echo $balance
    return $?
}


function paracross_Transfer_Withdraw() {
    echo "=========== ## para cross transfer/withdraw (main to para) test start"

    #fromAddr  跨莲资产转移地址
    local fromAddr="12qyocayNF7Lv6C9qW4avxs2E7U41fKSfv"
    #privkey 地址签名
    local privkey="0x4257D8692EF7FE13C68B65D6A52F03933DB2FA5CE8FAF210B5B8B80C721CED01"
    #paracrossAddr 合约地址
    local paracrossAddr="1HPkPopVe3ERfvaAgedDtJQ792taZFEHCe"
    #amount_save 存钱到合约地址
    local amount_save=100000000
    #amount_should 应转移金额
    local amount_should=27000000
    #withdraw_should 应取款金额
    local withdraw_should=13000000
    #fee 交易费
    local fee=1000000

    #1. 查询资产转移前余额状态

    local para_balance_before=$(paracross_QueryBalance $fromAddr)
    echo "before transferring:$para_balance_before"

    #2  存钱到合约地址
    local tx=$(curl -ksd '{"method":"Chain33.CreateRawTransaction","params":[{"to":"'$paracrossAddr'","amount":'$amount_save'}]}' ${PARA_HTTP} | jq -r ".result")
    ##echo "tx:$tx"
    paracross_SignAndSend $fee $privkey $tx


    #3  资产从主链转移到平行链
    tx=$(curl -ksd '{"method":"Chain33.CreateTransaction","params":[{"execer":"paracross","actionName":"ParacrossAssetTransfer","payload":{"execer":"user.p.para.paracross","execName":"user.p.para.paracross","to":"'$fromAddr'","amount":'$amount_should'}}]}' ${PARA_HTTP} | jq -r ".result")
    #echo "rawTx:$rawTx"
    paracross_SignAndSend $fee $privkey $tx

    sleep 30

    #4 查询转移余额状态
    local para_balance_after=$(paracross_QueryBalance $fromAddr)
    echo "after transferring:$para_balance_after"

    #real_amount  实际转移金额
    local amount_real=$(($para_balance_after - $para_balance_before))

    #5 取钱
    tx=$(curl -ksd '{"method":"Chain33.CreateTransaction","params":[{"execer":"paracross","actionName":"ParacrossAssetWithdraw","payload":{"IsWithdraw":'true',"execer":"user.p.para.paracross","execName":"user.p.para.paracross","to":"'$fromAddr'","amount":'$withdraw_should'}}]}' ${PARA_HTTP} | jq -r ".result")
    #echo "rawTx:$rawTx"
    paracross_SignAndSend $fee $privkey $tx


    sleep 15
    #6 查询取钱后余额状态
    local para_balance_withdraw_after=$(paracross_QueryBalance $fromAddr)
    echo "after withdrawing :$para_balance_withdraw_after"

    #实际取钱金额
    local withdraw_real=$((para_balance_after - para_balance_withdraw_after))
    #echo $withdraw

     #7 验证转移是否正确
    [ "$amount_real" ==  "$amount_should" ] && [ "$withdraw_should" == "$withdraw_real"  ]
    rst=$?
    echo_rst "$FUNCNAME" "$rst"

    echo "=========== ## para cross transfer/withdraw (main to para) test start end"


}


function paracross_IsSync() {
    local ok=$(curl -s --data-binary '{"jsonrpc":"2.0","id":2,"method":"paracross.IsSync","params":[]}' -H 'content-type:text/plain;' ${PARA_HTTP} | jq -r ".result")
    [ "$ok" == true ]
    local rst=$?
    echo_rst "$FUNCNAME" "$rst"
}

function paracross_ListTitles() {
    local resp=$(curl -s --data-binary '{"jsonrpc":"2.0","id":2,"method":"paracross.ListTitles","params":[]}' -H 'content-type:text/plain;' ${PARA_HTTP} )
    #echo $resp
    local ok=$(jq '(.error|not) and (.result| [has("titles"),true])' <<<"$resp")
    [ "$ok" == true ]
    local rst=$?
    echo_rst "$FUNCNAME" "$rst"
}



function paracross_GetHeight() {
    local resp=$(curl -s --data-binary '{"jsonrpc":"2.0","id":2,"method":"paracross.GetHeight","params":[]}' -H 'content-type:text/plain;' ${PARA_HTTP} )
    #echo $resp
    local ok=$(jq '(.error|not) and (.result| [has("consensHeight"),true])' <<<"$resp")
    [ "$ok" == true ]
    local rst=$?
    echo_rst "$FUNCNAME" "$rst"
}

function paracross_GetNodeGroupAddrs() {
    local resp=$(curl -s --data-binary '{"jsonrpc":"2.0","id":2,"method":"paracross.GetNodeGroupAddrs","params":[{"title":"user.p.para."}]}' -H 'content-type:text/plain;' ${PARA_HTTP} )
    #echo $resp
    local ok=$(jq '(.error|not) and (.result| [has("key","value"),true])' <<<"$resp")
    [ "$ok" == true ]
    local rst=$?
    echo_rst "$FUNCNAME" "$rst"
}

function paracross_GetNodeGroupStatus() {
    local resp=$(curl -s --data-binary '{"jsonrpc":"2.0","id":2,"method":"paracross.GetNodeGroupStatus","params":[{"title":"user.p.para."}]}' -H 'content-type:text/plain;' ${PARA_HTTP} )
    #echo $resp
    local ok=$(jq '(.error|not) and (.result| [has("status"),true])' <<<"$resp")
    [ "$ok" == true ]
    local rst=$?
    echo_rst "$FUNCNAME" "$rst"
}

function paracross_ListNodeGroupStatus() {
    local resp=$(curl -s --data-binary '{"jsonrpc":"2.0","id":2,"method":"paracross.ListNodeGroupStatus","params":[{"title":"user.p.para.","status":'2'}]}' -H 'content-type:text/plain;' ${PARA_HTTP} )
    #echo $resp
    local ok=$(jq '(.error|not) and (.result| [has("status"),true])' <<<"$resp")
    [ "$ok" == true ]
    local rst=$?
    echo_rst "$FUNCNAME" "$rst"
}


function paracross_ListNodeStatus() {
    local resp=$(curl -s --data-binary '{"jsonrpc":"2.0","id":2,"method":"paracross.ListNodeStatus","params":[{"title":"user.p.para.","status":'4'}]}' -H 'content-type:text/plain;' ${PARA_HTTP} )
    #echo $resp
    local ok=$(jq '(.error|not) and (.result| [has("status"),true])' <<<"$resp")
    [ "$ok" == true ]
    local rst=$?
    echo_rst "$FUNCNAME" "$rst"
}


function run_main_testcases() {
    chain33_lock
    chain33_unlock
    paracross_GetBlock2MainInfo

}

function run_para_testcases() {
    chain33_lock
    chain33_unlock
    paracross_GetBlock2MainInfo
    paracross_IsSync
    paracross_ListTitles
    paracross_GetHeight
    paracross_GetNodeGroupAddrs
    paracross_GetNodeGroupStatus
    paracross_ListNodeGroupStatus
    paracross_ListNodeStatus
    paracross_Transfer_Withdraw
}

function dapp_rpc_test() {
    local ip=$1
    MAIN_HTTP="http://$ip:8801"
    PARA_HTTP="http://$ip:8901"
    echo "=========== # paracross rpc test ============="
    echo "main_ip=$MAIN_HTTP,para_ip=$PARA_HTTP"

    UNIT_HTTP=$MAIN_HTTP
    run_main_testcases

    UNIT_HTTP=$PARA_HTTP
    run_para_testcases

    if [ -n "$CASE_ERR" ]; then
        echo "paracross there some case error"
        exit 1
    fi
}

#dapp_rpc_test $1
