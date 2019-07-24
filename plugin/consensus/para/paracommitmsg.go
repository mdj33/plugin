// Copyright Fuzamei Corp. 2018 All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package para

import (
	"context"
	"time"

	"strings"

	"sync/atomic"
	"unsafe"

	"github.com/33cn/chain33/common"
	"github.com/33cn/chain33/common/crypto"
	"github.com/33cn/chain33/types"
	paracross "github.com/33cn/plugin/plugin/dapp/paracross/types"
	pt "github.com/33cn/plugin/plugin/dapp/paracross/types"
	"github.com/pkg/errors"
)

var (
	consensusInterval = 10 //about 1 new block interval
	minerInterval     = 10 //5s的主块间隔后分叉概率增加，10s可以消除一些分叉回退
)

type commitMsgClient struct {
	paraClient           *client
	waitMainBlocks       int32  //等待平行链共识消息在主链上链并成功的块数，超出会重发共识消息，最小是2
	waitConsensStopTimes uint32 //共识高度低于完成高度， reset高度重发等待的次数
	commitCh             chan int64
	resetCh              chan int64
	sendMsgCh            chan *types.Transaction
	minerSwitch          int32
	currentTx            unsafe.Pointer
	chainHeight          int64
	sendingHeight        int64
	consensHeight        int64
	authAccountIn        int32
	isRollBack           int32
	checkTxCommitTimes   int32
	privateKey           crypto.PrivKey
	quit                 chan struct{}
}

// 1. 链高度回滚，低于当前发送高度，需要重新计算当前发送高度,不然不会重新发送回滚的高度
// 2. 定时轮询是在比如锁定解锁钱包这类外部条件变化时候，其他输入条件不会触发时候及时响应，不然任何一个外部条件变化都触发一下发送，可能条件比较多
func (client *commitMsgClient) handler() {
	var readTick <-chan time.Time
	var consensStopTimes uint32

	client.paraClient.wg.Add(1)
	go client.getConsensusHeight()

	if client.paraClient.authAccount != "" {
		client.paraClient.wg.Add(1)
		client.sendMsgCh = make(chan *types.Transaction, 1)
		go client.sendCommitMsg()

		ticker := time.NewTicker(time.Second * time.Duration(minerInterval))
		readTick = ticker.C
		defer ticker.Stop()
	}

out:
	for {
		select {
		//正常commit 入口
		case <-client.commitCh:
			//回滚场景
			if atomic.LoadInt64(&client.chainHeight) < client.sendingHeight {
				client.clearSendingTx()
			}
			client.procSendTx()
		//出错场景入口，需要reset 重发
		case <-client.resetCh:
			client.clearSendingTx()
			client.procSendTx()
		//例行检查发送入口
		case <-readTick:
			consensStopTimes = client.checkConsensusStop(consensStopTimes)
			client.procSendTx()

		case <-client.quit:
			break out
		}
	}

	client.paraClient.wg.Done()
}

func (client *commitMsgClient) commitNotify() {
	client.commitCh <- 1
}
func (client *commitMsgClient) resetNotify() {
	client.resetCh <- 1
}

func (client *commitMsgClient) clearSendingTx() {
	client.sendingHeight = -1
	client.setCurrentTx(nil)
}

func (client *commitMsgClient) procSendTx() {
	plog.Info("para commitMsg---status", "chainHeight", atomic.LoadInt64(&client.chainHeight), "sendingHeight", client.sendingHeight,
		"consensHeight", atomic.LoadInt64(&client.consensHeight), "isSendingTx", client.isSendingCommitMsg(), "sync", client.isSync())

	if client.isSendingCommitMsg() || !client.isSync() {
		return
	}

	consensHeight := atomic.LoadInt64(&client.consensHeight)
	chainHeight := atomic.LoadInt64(&client.chainHeight)

	if client.sendingHeight < consensHeight {
		client.sendingHeight = consensHeight
	}

	//1.如果是在主链共识场景，共识高度可能大于平行链的链高度
	//2.已发送，未共识场景
	if chainHeight < consensHeight || client.sendingHeight > consensHeight {
		return
	}

	if client.sendingHeight < chainHeight {
		signTx, count := client.getSendingTx(client.sendingHeight, chainHeight)
		if signTx == nil {
			return
		}
		client.sendingHeight = client.sendingHeight + count
		client.setCurrentTx(signTx)
		atomic.StoreInt32(&client.checkTxCommitTimes, 0)
		client.sendMsgCh <- signTx
	}

}

func (client *commitMsgClient) isSync() bool {
	height := atomic.LoadInt64(&client.chainHeight)
	if height <= 0 {
		plog.Info("para is not Sync", "chainHeight", height)
		return false
	}

	height = atomic.LoadInt64(&client.consensHeight)
	if height == -2 {
		plog.Info("para is not Sync", "consensHeight", height)
		return false
	}

	if atomic.LoadInt32(&client.authAccountIn) != 1 {
		plog.Info("para is not Sync", "authAccountIn", atomic.LoadInt32(&client.authAccountIn))
		return false
	}

	if atomic.LoadInt32(&client.minerSwitch) != 1 {
		plog.Info("para is not Sync", "isMiner", atomic.LoadInt32(&client.minerSwitch))
		return false
	}

	if atomic.LoadInt32(&client.isRollBack) == 1 {
		plog.Info("para is not Sync", "isRollBack", atomic.LoadInt32(&client.isRollBack))
		return false
	}

	if atomic.LoadInt32(&client.paraClient.isCaughtUp) != 1 {
		plog.Info("para is not Sync", "isCaughtUp", atomic.LoadInt32(&client.paraClient.isCaughtUp))
		return false
	}

	if !client.paraClient.SyncHasCaughtUp() {
		plog.Info("para is not Sync", "syncCaughtUp", client.paraClient.SyncHasCaughtUp())
		return false
	}

	return true

}

func (client *commitMsgClient) getSendingTx(startHeight, endHeight int64) (*types.Transaction, int64) {
	count := endHeight - startHeight
	if count > types.TxGroupMaxCount {
		count = types.TxGroupMaxCount
	}
	status, err := client.getNodeStatus(startHeight+1, startHeight+count)
	if err != nil {
		plog.Error("para commit msg read tick", "err", err.Error())
		return nil, 0
	}
	if len(status) == 0 {
		return nil, 0
	}

	signTx, count, err := client.calcCommitMsgTxs(status)
	if err != nil || signTx == nil {
		return nil, 0
	}

	sendingMsgs := status[:count]
	plog.Info("paracommitmsg sending", "txhash", common.ToHex(signTx.Hash()), "exec", string(signTx.Execer))
	for i, msg := range sendingMsgs {
		plog.Info("paracommitmsg sending", "idx", i, "height", msg.Height, "mainheight", msg.MainBlockHeight,
			"blockhash", common.HashHex(msg.BlockHash), "mainHash", common.HashHex(msg.MainBlockHash),
			"from", client.paraClient.authAccount)
	}

	return signTx, count
}

func (client *commitMsgClient) setCurrentTx(tx *types.Transaction) {
	atomic.StorePointer(&client.currentTx, unsafe.Pointer(tx))
}

func (client *commitMsgClient) getCurrentTx() *types.Transaction {
	return (*types.Transaction)(atomic.LoadPointer(&client.currentTx))
}

func (client *commitMsgClient) isSendingCommitMsg() bool {
	return client.getCurrentTx() != nil
}

func (client *commitMsgClient) updateChainHeight(height int64, isDel bool) {
	if isDel {
		atomic.StoreInt32(&client.isRollBack, 1)
	} else {
		atomic.StoreInt32(&client.isRollBack, 0)
	}

	atomic.StoreInt64(&client.chainHeight, height)
	client.commitNotify()

}

//TODO 非平行鏈的commit tx 去主鏈查詢
func (client *commitMsgClient) checkSendingTxDone(txs map[string]bool) {
	tx := client.getCurrentTx()
	if tx == nil {
		return
	}

	if txs[string(tx.Hash())] {
		client.setCurrentTx(nil)
		atomic.StoreInt32(&client.checkTxCommitTimes, 0)
		//继续处理
		client.commitNotify()
		return
	}

	atomic.AddInt32(&client.checkTxCommitTimes, 1)
	if atomic.LoadInt32(&client.checkTxCommitTimes) >= client.waitMainBlocks {
		atomic.StoreInt32(&client.checkTxCommitTimes, 0)
		//重新发送
		client.resetNotify()
	}

}

func (client *commitMsgClient) calcCommitMsgTxs(notifications []*pt.ParacrossNodeStatus) (*types.Transaction, int64, error) {
	txs, count, err := client.batchCalcTxGroup(notifications)
	if err != nil {
		txs, err = client.singleCalcTx((notifications)[0])
		if err != nil {
			plog.Error("single calc tx", "height", notifications[0].Height)

			return nil, 0, err
		}
		return txs, 1, nil
	}
	return txs, int64(count), nil
}

func (client *commitMsgClient) getTxsGroup(txsArr *types.Transactions) (*types.Transaction, error) {
	if len(txsArr.Txs) < 2 {
		tx := txsArr.Txs[0]
		tx.Sign(types.SECP256K1, client.privateKey)
		return tx, nil
	}

	group, err := types.CreateTxGroup(txsArr.Txs)
	if err != nil {
		plog.Error("para CreateTxGroup", "err", err.Error())
		return nil, err
	}
	err = group.Check(0, types.GInt("MinFee"), types.GInt("MaxFee"))
	if err != nil {
		plog.Error("para CheckTxGroup", "err", err.Error())
		return nil, err
	}
	for i := range group.Txs {
		group.SignN(i, int32(types.SECP256K1), client.privateKey)
	}

	newtx := group.Tx()
	return newtx, nil
}

func (client *commitMsgClient) batchCalcTxGroup(notifications []*pt.ParacrossNodeStatus) (*types.Transaction, int, error) {
	var rawTxs types.Transactions
	for _, status := range notifications {
		execName := pt.ParaX
		if isParaSelfConsensusForked(status.MainBlockHeight) {
			execName = paracross.GetExecName()
		}
		tx, err := paracross.CreateRawCommitTx4MainChain(status, execName, 0)
		if err != nil {
			plog.Error("para get commit tx", "block height", status.Height)
			return nil, 0, err
		}
		rawTxs.Txs = append(rawTxs.Txs, tx)
	}

	txs, err := client.getTxsGroup(&rawTxs)
	if err != nil {
		return nil, 0, err
	}
	return txs, len(notifications), nil
}

func (client *commitMsgClient) singleCalcTx(status *pt.ParacrossNodeStatus) (*types.Transaction, error) {
	execName := pt.ParaX
	if isParaSelfConsensusForked(status.MainBlockHeight) {
		execName = paracross.GetExecName()
	}
	tx, err := paracross.CreateRawCommitTx4MainChain(status, execName, 0)
	if err != nil {
		plog.Error("para get commit tx", "block height", status.Height)
		return nil, err
	}
	tx.Sign(types.SECP256K1, client.privateKey)
	return tx, nil

}

func (client *commitMsgClient) sendCommitMsg() {
	var err error
	var tx *types.Transaction
	var resendTimer <-chan time.Time

out:
	for {
		select {
		case tx = <-client.sendMsgCh:
			err = client.sendCommitMsgTx(tx)
			if err != nil && (err != types.ErrBalanceLessThanTenTimesFee && err != types.ErrNoBalance) {
				resendTimer = time.After(time.Second * 2)
			}
		case <-resendTimer:
			if err != nil && tx != nil {
				client.sendCommitMsgTx(tx)
			}
		case <-client.quit:
			break out
		}
	}

	client.paraClient.wg.Done()
}

func (client *commitMsgClient) sendCommitMsgTx(tx *types.Transaction) error {
	if tx == nil {
		return nil
	}
	resp, err := client.paraClient.grpcClient.SendTransaction(context.Background(), tx)
	if err != nil {
		plog.Error("sendCommitMsgTx send tx", "tx", common.ToHex(tx.Hash()), "err", err.Error())
		return err
	}

	if !resp.GetIsOk() {
		plog.Error("sendCommitMsgTx send tx Nok", "tx", common.ToHex(tx.Hash()), "err", string(resp.GetMsg()))
		return errors.New(string(resp.GetMsg()))
	}

	return nil

}

func isParaSelfConsensusForked(height int64) bool {
	return height > mainParaSelfConsensusForkHeight
}

//当前未考虑获取key非常多失败的场景， 如果获取height非常多，block模块会比较大，但是使用完了就释放了
//如果有必要也可以考虑每次最多取20个一个txgroup，发送共识部分循环获取发送也没问题
func (client *commitMsgClient) getNodeStatus(start, end int64) ([]*pt.ParacrossNodeStatus, error) {
	var ret []*pt.ParacrossNodeStatus
	if start == 0 {
		geneStatus, err := client.getGenesisNodeStatus()
		if err != nil {
			return nil, err
		}
		ret = append(ret, geneStatus)
		start++
	}
	if end < start {
		return ret, nil
	}

	req := &types.ReqBlocks{Start: start, End: end}
	count := req.End - req.Start + 1
	nodeList := make(map[int64]*pt.ParacrossNodeStatus, count+1)
	keys := &types.LocalDBGet{}
	for i := 0; i < int(count); i++ {
		key := paracross.CalcMinerHeightKey(types.GetTitle(), req.Start+int64(i))
		keys.Keys = append(keys.Keys, key)
	}

	r, err := client.paraClient.GetAPI().LocalGet(keys)
	if err != nil {
		return nil, err
	}
	if count != int64(len(r.Values)) {
		plog.Error("paracommitmsg get node status key", "expect count", count, "actual count", len(r.Values))
		return nil, err
	}
	for _, val := range r.Values {
		status := &pt.ParacrossNodeStatus{}
		err = types.Decode(val, status)
		if err != nil {
			return nil, err
		}
		if !(status.Height >= req.Start && status.Height <= req.End) {
			plog.Error("paracommitmsg decode node status", "height", status.Height, "expect start", req.Start,
				"end", req.End, "status", status)
			return nil, errors.New("paracommitmsg wrong key result")
		}
		nodeList[status.Height] = status

	}
	for i := 0; i < int(count); i++ {
		if nodeList[req.Start+int64(i)] == nil {
			plog.Error("paracommitmsg get node status key nil", "height", req.Start+int64(i))
			return nil, errors.New("paracommitmsg wrong key status result")
		}
	}

	v, err := client.paraClient.GetAPI().GetBlocks(req)
	if err != nil {
		return nil, err
	}
	if count != int64(len(v.Items)) {
		plog.Error("paracommitmsg get node status block", "expect count", count, "actual count", len(v.Items))
		return nil, err
	}
	for _, block := range v.Items {
		if !(block.Block.Height >= req.Start && block.Block.Height <= req.End) {
			plog.Error("paracommitmsg get node status block", "height", block.Block.Height, "expect start", req.Start, "end", req.End)
			return nil, errors.New("paracommitmsg wrong block result")
		}
		nodeList[block.Block.Height].BlockHash = block.Block.Hash()
		if !paracross.IsParaForkHeight(nodeList[block.Block.Height].MainBlockHeight, paracross.ForkLoopCheckCommitTxDone) {
			nodeList[block.Block.Height].StateHash = block.Block.StateHash
		}
	}

	var needSentTxs uint32
	for i := 0; i < int(count); i++ {
		ret = append(ret, nodeList[req.Start+int64(i)])
		needSentTxs += nodeList[req.Start+int64(i)].NonCommitTxCounts
	}
	//1.如果是只有commit tx的空块，推迟发送，直到等到一个完全没有commit tx的空块或者其他tx的块
	//2,如果20个块都是 commit tx的空块，20个块打包一次发送，尽量减少commit tx造成的空块
	//3,如果形如xxoxx的块排列，x代表commit空块，o代表实际的块，即只要不全部是commit块，也要全部打包一起发出去
	//如果=0 意味着全部是paracross commit tx，延迟发送
	if needSentTxs == 0 && len(ret) < types.TxGroupMaxCount {
		plog.Debug("para commitmsg getNodeStatus all self consensus commit tx,send delay", "start", start, "end", end)
		return nil, nil
	}

	//clear flag
	for _, v := range ret {
		v.NonCommitTxCounts = 0
	}

	return ret, nil

}

func (client *commitMsgClient) getGenesisNodeStatus() (*pt.ParacrossNodeStatus, error) {
	var status pt.ParacrossNodeStatus
	req := &types.ReqBlocks{Start: 0, End: 0}
	v, err := client.paraClient.GetAPI().GetBlocks(req)
	if err != nil {
		return nil, err
	}
	block := v.Items[0].Block
	if block.Height != 0 {
		return nil, errors.New("block chain not return 0 height block")
	}
	status.Title = types.GetTitle()
	status.Height = block.Height
	status.BlockHash = block.Hash()

	return &status, nil
}

//only sync once, as main usually sync, here just need the first sync status after start up
func (client *commitMsgClient) mainSync() error {
	req := &types.ReqNil{}
	reply, err := client.paraClient.grpcClient.IsSync(context.Background(), req)
	if err != nil {
		plog.Error("Paracross main is syncing", "err", err.Error())
		return err
	}
	if !reply.IsOk {
		plog.Error("Paracross main reply not ok")
		return err
	}

	plog.Info("Paracross main sync succ")
	return nil

}

func (client *commitMsgClient) checkConsensusStop(consensStopTimes uint32) uint32 {
	if client.sendingHeight > atomic.LoadInt64(&client.consensHeight) && !client.isSendingCommitMsg() {
		if consensStopTimes > client.waitConsensStopTimes {
			client.clearSendingTx()
			return 0
		}
		return consensStopTimes + 1
	}

	return 0
}

func (client *commitMsgClient) getConsensusHeight() {
	ticker := time.NewTicker(time.Second * time.Duration(consensusInterval))
	isSync := false
	defer ticker.Stop()

out:
	for {
		select {
		case <-client.quit:
			break out
		case <-ticker.C:
			if !isSync {
				err := client.mainSync()
				if err != nil {
					continue
				}
				isSync = true
			}

			block, err := client.paraClient.getLastBlockInfo()
			if err != nil {
				continue
			}

			status, err := client.getConsensusStatus(block)
			if err != nil {
				continue
			}
			atomic.StoreInt64(&client.consensHeight, status.Height)

			authExist := false
			if client.paraClient.authAccount != "" {
				nodes, err := client.getNodeGroupAddrs()
				if err != nil {
					continue
				}
				authExist = strings.Contains(nodes, client.paraClient.authAccount)
			}
			if authExist {
				atomic.StoreInt32(&client.authAccountIn, 1)
			} else {
				atomic.StoreInt32(&client.authAccountIn, 0)
			}

			plog.Debug("para getConsensusHeight", "height", status.Height, "AccoutIn", authExist)

		}
	}

	client.paraClient.wg.Done()
}

func (client *commitMsgClient) getConsensusStatus(block *types.Block) (*pt.ParacrossStatus, error) {
	if isParaSelfConsensusForked(block.MainHeight) {
		//从本地查询共识高度
		ret, err := client.paraClient.GetAPI().QueryChain(&types.ChainExecutor{
			Driver:   "paracross",
			FuncName: "GetTitle",
			Param:    types.Encode(&types.ReqString{Data: types.GetTitle()}),
		})
		if err != nil {
			plog.Error("getConsensusHeight ", "err", err.Error())
			return nil, err
		}
		resp, ok := ret.(*pt.ParacrossStatus)
		if !ok {
			plog.Error("getConsensusHeight ParacrossStatus nok")
			return nil, err
		}
		//开启自共识后也要等到自共识真正切换之后再使用，如果本地区块已经过了自共识高度，但自共识的高度还没达成，就会导致共识机制出错
		if resp.Height > -1 {
			req := &types.ReqBlocks{Start: resp.Height, End: resp.Height}
			v, err := client.paraClient.GetAPI().GetBlocks(req)
			if err != nil {
				plog.Error("getConsensusHeight GetBlocks", "err", err.Error())
				return nil, err
			}
			//本地共识高度对应主链高度一定要高于自共识高度，为了适配平行链共识高度不连续场景
			if isParaSelfConsensusForked(v.Items[0].Block.MainHeight) {
				return resp, nil
			}
		}
	}

	//去主链获取共识高度
	reply, err := client.paraClient.grpcClient.QueryChain(context.Background(), &types.ChainExecutor{
		Driver:   "paracross",
		FuncName: "GetTitleByHash",
		Param:    types.Encode(&pt.ReqParacrossTitleHash{Title: types.GetTitle(), BlockHash: block.MainHash}),
	})
	if err != nil {
		plog.Error("getMainConsensusHeight", "err", err.Error())
		return nil, err
	}
	if !reply.GetIsOk() {
		plog.Info("getMainConsensusHeight nok", "error", reply.GetMsg())
		return nil, err
	}
	var result pt.ParacrossStatus
	err = types.Decode(reply.Msg, &result)
	if err != nil {
		plog.Error("getMainConsensusHeight decode", "err", err.Error())
		return nil, err
	}
	return &result, nil

}

//node group会在主链和平行链都同时配置,只本地查询就可以
func (client *commitMsgClient) getNodeGroupAddrs() (string, error) {
	ret, err := client.paraClient.GetAPI().QueryChain(&types.ChainExecutor{
		Driver:   "paracross",
		FuncName: "GetNodeGroupAddrs",
		Param:    types.Encode(&pt.ReqParacrossNodeInfo{Title: types.GetTitle()}),
	})
	if err != nil {
		plog.Error("commitmsg.getNodeGroupAddrs ", "err", err.Error())
		return "", err
	}
	resp, ok := ret.(*types.ReplyConfig)
	if !ok {
		plog.Error("commitmsg.getNodeGroupAddrs rsp nok")
		return "", err
	}

	return resp.Value, nil
}

func (client *commitMsgClient) onWalletStatus(status *types.WalletStatus) {
	if status == nil || client.paraClient.authAccount == "" {
		return
	}
	if !status.IsWalletLock && client.privateKey == nil {
		client.fetchPriKey()
		plog.Info("para commit fetchPriKey")
	}

	if client.privateKey == nil {
		plog.Info("para commit wallet status prikey null", "status", status.IsWalletLock)
		return
	}

	if status.IsWalletLock {
		atomic.StoreInt32(&client.minerSwitch, 0)
	} else {
		atomic.StoreInt32(&client.minerSwitch, 1)
	}

}

func (client *commitMsgClient) onWalletAccount(acc *types.Account) {
	if acc == nil || client.paraClient.authAccount == "" || client.paraClient.authAccount != acc.Addr || client.privateKey != nil {
		return
	}
	err := client.fetchPriKey()
	if err != nil {
		plog.Error("para commit fetchPriKey", "err", err.Error())
		return
	}

	atomic.StoreInt32(&client.minerSwitch, 1)

}

func (client *commitMsgClient) fetchPriKey() error {
	req := &types.ReqString{Data: client.paraClient.authAccount}

	msg := client.paraClient.GetQueueClient().NewMessage("wallet", types.EventDumpPrivkey, req)
	err := client.paraClient.GetQueueClient().Send(msg, true)
	if err != nil {
		plog.Error("para commit send msg", "err", err.Error())
		return err
	}
	resp, err := client.paraClient.GetQueueClient().Wait(msg)
	if err != nil {
		plog.Error("para commit msg sign to wallet", "err", err.Error())
		return err
	}
	str := resp.GetData().(*types.ReplyString).Data
	pk, err := common.FromHex(str)
	if err != nil && pk == nil {
		return err
	}

	secp, err := crypto.New(types.GetSignName("", types.SECP256K1))
	if err != nil {
		return err
	}

	priKey, err := secp.PrivKeyFromBytes(pk)
	if err != nil {
		plog.Error("para commit msg get priKey", "err", err.Error())
		return err
	}

	client.privateKey = priKey
	plog.Info("para commit fetchPriKey success")
	return nil
}
