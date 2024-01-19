// Copyright (c) 2023-2024 The UXUY Developer Team
// License:
// MIT License

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
//SOFTWARE

package dcache

import (
	"github.com/shopspring/decimal"
	"sync"
)

// UTXO
/*****************************************************
 * Build cache for all utxo records
 * Mainly used for mint & transfer data checking
 ****************************************************/
type UTXO struct {
	hashes *sync.Map //record mint hash items
}

type UTXOItem struct {
	Protocol string
	Tick     string
	Amount   decimal.Decimal
	Owner    string
}

func NewUTXO() *UTXO {
	return &UTXO{
		hashes: &sync.Map{},
	}
}

/***************************************
 * idx define utxo unique id
 ***************************************/
func (d *UTXO) idx(sn string) string {
	return sn
}

// Add
/***************************************
 * Add new utxo record
 ***************************************/
func (d *UTXO) Add(protocol, tick, sn, address string, amount decimal.Decimal) {
	idx := d.idx(sn)
	d.hashes.Store(idx, &UTXOItem{
		Protocol: protocol,
		Tick:     tick,
		Amount:   amount,
		Owner:    address,
	})
}

// Transfer
/***************************************
 * transfer utxo owner
 ***************************************/
func (d *UTXO) Transfer(sn string, owner string) {
	ok, item := d.Get(sn)
	if !ok {
		return
	}
	item.Owner = owner
}

// Get
/***************************************
 * get utxo record by mint tx hash
 ***************************************/
func (d *UTXO) Get(sn string) (bool, *UTXOItem) {
	idx := d.idx(sn)
	item, ok := d.hashes.Load(idx)
	if !ok {
		return false, nil
	}
	return true, item.(*UTXOItem)
}
