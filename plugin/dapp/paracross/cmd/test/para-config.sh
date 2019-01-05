#!/usr/bin/env bash
#just add the auth account to config, only one node need to do that once

PARANAME="para"
CLI="./chain33-cli"
PARA_CLI="./chain33-para-cli"

function para_transfer() {
    echo "=========== # para chain transfer ============="
    #para_transfer2account "1Q8hGLfoGe63efeWa8fJ4Pnukhkngt6poK"
    #para_transfer2account "1KSBd17H7ZK8iT37aJztFB22XGwsPTdwE4"
    #para_transfer2account "1JRNjdEqp4LJ5fqycUBm9ayCKSeeskgMKR"
    #para_transfer2account "1NLHPEcbTWWxxU3dGUZBhayjrCHD3psX7k"
    #para_transfer2account "1MCftFynyvG2F4ED5mdHYgziDxx6vDrScs"

    para_config "${CLI}" "paracross-nodes-user.p.${PARANAME}." "1KSBd17H7ZK8iT37aJztFB22XGwsPTdwE4"
    para_config "${CLI}" "paracross-nodes-user.p.${PARANAME}." "1JRNjdEqp4LJ5fqycUBm9ayCKSeeskgMKR"
    para_config "${CLI}" "paracross-nodes-user.p.${PARANAME}." "1NLHPEcbTWWxxU3dGUZBhayjrCHD3psX7k"
    para_config "${CLI}" "paracross-nodes-user.p.${PARANAME}." "1MCftFynyvG2F4ED5mdHYgziDxx6vDrScs"

    #para_config "${PARA_CLI}" "token-blacklist" "BTY"

}

function para_transfer2account() {
    echo "${1}"
    hash1=$(${CLI} send coins transfer -a 10000 -n test -t "${1}" -k 4257D8692EF7FE13C68B65D6A52F03933DB2FA5CE8FAF210B5B8B80C721CED01)
    echo "${hash1}"
}

function para_config() {
    echo "=========== # para chain send config ============="
    echo "${3}"
    tx=$(${1} config config_tx -o add -k "${2}" -v "${3}")
    echo "${tx}"
    sign=$(${CLI} wallet sign -k 0xc34b5d9d44ac7b754806f761d3d4d2c4fe5214f6b074c19f069c4f5c2a29c8cc -d "${tx}")
    echo "${sign}"
    send=$(${CLI} wallet send -d "${sign}")
    echo "${send}"
}

main(){
    para_transfer
}

main