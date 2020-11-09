// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"flag"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/simulations"
	"github.com/ethereum/go-ethereum/params"
	"github.com/mattn/go-colorable"
	"github.com/rjl493456442/les-simulator/simulator"
)

var (
	loglevel = flag.Int("loglevel", 3, "verbosity of logs")
)

// main() starts a simulation network which contains nodes running a simple
// ping-pong protocol
func main() {
	flag.Parse()

	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(*loglevel), log.StreamHandler(colorable.NewColorableStderr(), log.TerminalFormat(true))))

	// Create accounts for simulations
	keystorePath, err := ioutil.TempDir("", "simulation-keystore")
	if err != nil {
		log.Crit("Failed to create temp dir", "err", err)
	}
	defer os.RemoveAll(keystorePath)
	var prefunds = make(map[common.Address]*big.Int)
	var accounts []common.Address
	for i := 0; i < 2; i++ {
		account, err := keystore.StoreKey(keystorePath, "foobar", keystore.LightScryptN, keystore.LightScryptP)
		if err != nil {
			log.Crit("Failed to create test account file", "err", err)
		}
		prefunds[account.Address] = big.NewInt(params.Ether)
		accounts = append(accounts, account.Address)
	}
	// Create LES cluster
	cluster, err := simulator.NewCluster(&simulator.ClusterConfig{
		Adapter: "sim",
		ChainID: 1337,
		ClientConfig: []*simulator.ClientServiceConfig{
			{
				PaymentAddress:  accounts[0],
				TrustedServers:  nil,
				TrustedFraction: 0,
			},
		},
		ServerConfig: []*simulator.ServerServiceConfig{
			{
				PaymentAddress: accounts[1],
				LightServ:      100,
				LightPeers:     30,
			},
		},
		Blocks:                10,
		DeployPaymentContract: true,
		DeployOracleContract:  true,
		Prefunds:              prefunds,
		Conns:                 nil,
		KeystorePath:          keystorePath,
		ClefEnabled:           true,
		SigningRule:           signingRules,
	})
	if err != nil {
		log.Crit("Failed to create les cluster", "error", err)
	}
	log.Info("starting cluster....")
	cluster.StartNodes()

	log.Info("Connecting nodes....")
	cluster.Connect()

	// start the HTTP API
	log.Info("starting simulation server on 0.0.0.0:9999...")
	if err := http.ListenAndServe(":9999", simulations.NewServer(cluster.Network())); err != nil {
		log.Crit("error starting simulation server", "err", err)
	}
}

var signingRules = []byte(`
// The rules for listing accounts
function ApproveListing(req) {
    return "Approve"
}

function big(str) {
    if (str.slice(0, 2) == "0x") {
        return new BigNumber(str.slice(2), 16)
    }
    return new BigNumber(str)
}

// Only 5e-2 ethers are allowed to spend in 1 week time.
var window = 1000*3600*7;
var limit = new BigNumber("5e16");

function isLimitOk(transaction) {
    var value = big(transaction.value)
    var windowstart = new Date().getTime() - window;

    var txs = [];
    var stored = storage.get('txs');

    if (stored != "") {
        txs = JSON.parse(stored)
    }
    // First, remove all that have passed out of the time-window
    var newtxs = txs.filter(function(tx){return tx.tstamp > windowstart});
    // Secondly, aggregate the current sum
    sum = new BigNumber(0)
    sum = newtxs.reduce(function(agg, tx){ return big(tx.value).plus(agg)}, sum);
    // Would we exceed weekly limit ?
    return sum.plus(value).lt(limit)
}

// The rules for sigining transactions
function ApproveTx(r) {
    if (isLimitOk(r.transaction)) {
        return "Approve"
    }
    return "Reject"
}

// OnApprovedTx(str) is called when a transaction has been approved and signed.
function OnApprovedTx(resp) {
    var value = big(resp.tx.value)
    var txs = []
    // Load stored transactions
    var stored = storage.get('txs');
    if (stored != "") {
        txs = JSON.parse(stored)
    }
    // Add this to the storage
    txs.push({tstamp: new Date().getTime(), value: value});
    storage.put("txs", JSON.stringify(txs));
}

// The rules for sigining cheques
function ApproveSignData(r) {
    return "Approve"
}

// The rules for printing banner.
function OnSignerStartup(i) {
    return "Approve"
}`)
