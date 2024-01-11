package explorer

import (
	"errors"
	"fmt"
	"github.com/alitto/pond"
	"math/big"
	"open-indexer/client/xycommon"
	"open-indexer/devents"
	"open-indexer/protocol"
	"open-indexer/protocol/common"
	"open-indexer/xyerrors"
	"open-indexer/xylog"
	"strings"
	"sync"
	"time"
)

func (e *Explorer) validReceiptTxs(items []*xycommon.RpcTransaction) ([]*xycommon.RpcTransaction, *xyerrors.InsError) {
	txHashList := make(map[string]struct{}, len(items))
	for _, item := range items {
		txHashList[item.Hash] = struct{}{}
	}

	workers := int(e.config.Server.ScanLimit)
	pool := pond.New(workers, 0, pond.MinWorkers(workers))

	receiptsMap := &sync.Map{}
	for txHash := range txHashList {
		hash := txHash
		pool.Submit(func() {
			r, err := e.node.TransactionReceipt(e.ctx, hash)
			if err != nil {
				xylog.Logger.Errorf("get tx receipt err:%v, tx:%s", err, hash)
				return
			}

			if r == nil {
				xylog.Logger.Errorf("get tx receipt nil, tx:%s", hash)
				return
			}
			receiptsMap.Store(hash, r)
		})
	}

	// Stop the pool and wait for all submitted tasks to complete
	pool.StopAndWait()

	results := make([]*xycommon.RpcTransaction, 0, len(items))
	for _, item := range items {
		rv, ok := receiptsMap.Load(item.Hash)
		if !ok {
			return nil, xyerrors.NewInsError(-100, fmt.Sprintf("get tx[%s] receipt nil", item.Hash))
		}

		r := rv.(*xycommon.RpcReceipt)

		// tx status check
		if r.Status.Int64() != 1 {
			xylog.Logger.Warnf("tx[%s] status <> 1 & filtered", item.Hash)
			continue
		}

		if r.EffectiveGasPrice.Cmp(big.NewInt(0)) > 0 {
			item.GasPrice = r.EffectiveGasPrice
		}

		if r.GasUsed.Cmp(big.NewInt(0)) > 0 {
			item.Gas = r.GasUsed
		}
		results = append(results, item)
	}
	return results, nil
}

func (e *Explorer) tryFilterTxs(txs []*xycommon.RpcTransaction) []*xycommon.RpcTransaction {
	validTxs := make([]*xycommon.RpcTransaction, 0, len(txs))
	for _, tx := range txs {
		pt, md := protocol.GetProtocol(e.config, tx)
		if pt == nil {
			continue
		}

		// Add protocol whitelist
		if !e.protocolEnabled(md.Protocol) {
			continue
		}

		// Add protocol whitelist
		if !e.tickEnabled(md.Tick) {
			continue
		}
		validTxs = append(validTxs, tx)
	}
	return validTxs
}

func (e *Explorer) handleTxs(block *xycommon.RpcBlock, txs []*xycommon.RpcTransaction) *xyerrors.InsError {
	blockTxResults := make([]*devents.DBModelEvent, 0, len(txs))
	for _, tx := range txs {
		pt, md := protocol.GetProtocol(e.config, tx)
		if pt == nil {
			continue
		}

		// Add protocol whitelist
		if !e.protocolEnabled(md.Protocol) {
			continue
		}

		// Add protocol whitelist
		if !e.tickEnabled(md.Tick) {
			continue
		}

		txResults, err := pt.Parse(block, tx, md)
		if err != nil && errors.Is(err, xyerrors.ErrInternal) {
			return err
		}
		if err != nil {
			xylog.Logger.Infof("tx data parsed failed. md[%v], tx[%s], err[%v]", md, tx.Hash, err)
			continue
		}
		xylog.Logger.Infof("tx data parsed success. md[%v], tx[%s]", md, tx.Hash)

		if len(txResults) < 1 {
			xylog.Logger.Warnf("tx data parsed result nil. md[%v], tx[%s]", md, tx.Hash)
			continue
		}

		// update cache
		for _, txResult := range txResults {
			e.txResultHandler.UpdateCache(txResult)
			blockTxResults = append(blockTxResults, e.txResultHandler.BuildModel(txResult))
		}
	}
	e.writeDBAsync(block, blockTxResults)
	return nil
}

func (e *Explorer) extractTxsFromBlock(block *xycommon.RpcBlock) []*xycommon.RpcTransaction {
	if block == nil || len(block.Transactions) == 0 {
		return nil
	}

	txs := make([]*xycommon.RpcTransaction, 0, len(block.Transactions))
	for _, tx := range block.Transactions {
		// fast check & filter invalid txs
		if !e.fastChecking(tx) {
			continue
		}
		txs = append(txs, tx)
	}
	return txs
}

func (e *Explorer) FlushDB() {
	defer func() {
		e.cancel()
		if err := recover(); err != nil {
			xylog.Logger.Panicf("flush db error & quit, err[%v]", err)
		}
		xylog.Logger.Infof("flush db quit")
	}()
	e.dEvent.Flush()
}

func (e *Explorer) Index() {
	defer func() {
		e.cancel()
		if err := recover(); err != nil {
			xylog.Logger.Panicf("index error & quit, err[%v]", err)
		}
		xylog.Logger.Infof("index quit")
	}()
	xylog.Logger.Infof("start indexing...")

	for {
		select {
		case block := <-e.blocks:
			e.handleBlock(block)
		case <-e.ctx.Done():
			return
		}
	}
}

func (e *Explorer) handleBlock(block *xycommon.RpcBlock) {
	xylog.Logger.Infof("start handle block:%d", block.Number.Uint64())
	st := time.Now()
	defer func() {
		xylog.Logger.Infof("handle block finished, cost:%v", time.Since(st))
	}()

	retry := 0
	for {
		if block == nil || block.Number.Uint64() <= 0 {
			xylog.Logger.Infof("block nil or number[%d] <= 0", block.Number.Uint64())
			return
		}

		// extract txs from block & fast checking invalid tx
		txs := e.extractTxsFromBlock(block)

		// try filter invalid txs
		txs = e.tryFilterTxs(txs)

		// Add receipt data & filter invalid status
		txs, err := e.validReceiptTxs(txs)
		if err != nil {
			xylog.Logger.Errorf("fetch receipt data internal err:%v & retry later[%d]", err, retry)
			retry++
			<-time.After(time.Millisecond * 100)
			continue
		}

		// Handle: parse txs & sync cache / db
		err = e.handleTxs(block, txs)
		if err != nil {
			xylog.Logger.Errorf("parse internal err:%v & retry later[%d]", err, retry)
			retry++
			<-time.After(time.Millisecond * 100)
			continue
		}
		return
	}
}

func (e *Explorer) writeDBAsync(block *xycommon.RpcBlock, txResults []*devents.DBModelEvent) {
	if block == nil || len(txResults) <= 0 {
		return
	}

	start := time.Now()

	//write db async
	event := &devents.Event{
		Chain:     e.config.Chain.ChainName,
		BlockNum:  block.Number.Uint64(),
		BlockTime: block.Time,
		BlockHash: block.Hash,
		Items:     txResults,
	}
	e.dEvent.WriteDBAsync(event)

	//dBytes, _ := json.Marshal(event)
	dBytes := ""
	xylog.Logger.Infof("push block data to events, cost:%v, block:%s, data:%s", time.Since(start), block.Number.String(), dBytes)
}

func (e *Explorer) fastChecking(tx *xycommon.RpcTransaction) bool {
	// events log checking
	if len(tx.Events) > 0 {
		return true
	}

	// input dmt format checking
	trxContent := tx.Input

	// 0x prefix checking
	if !strings.HasPrefix(trxContent, "0x") {
		return false
	}

	// data prefix checking
	if strings.HasPrefix(trxContent, common.DataPrefix) {
		return true
	}
	return false
}

func (e *Explorer) protocolEnabled(protocol string) bool {
	if e.config.Filters == nil || e.config.Filters.Whitelist == nil {
		return true
	}

	if len(e.config.Filters.Whitelist.Protocols) <= 0 {
		return true
	}

	for _, v := range e.config.Filters.Whitelist.Protocols {
		if strings.EqualFold(v, protocol) {
			return true
		}
	}
	return false
}

func (e *Explorer) tickEnabled(tick string) bool {
	if e.config.Filters == nil || e.config.Filters.Whitelist == nil {
		return true
	}

	if len(e.config.Filters.Whitelist.Ticks) <= 0 {
		return true
	}

	for _, v := range e.config.Filters.Whitelist.Ticks {
		if strings.EqualFold(v, tick) {
			return true
		}
	}
	return false
}