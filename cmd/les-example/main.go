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
	"net/http"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/simulations"
	"github.com/mattn/go-colorable"
	"github.com/rjl493456442/les-simulator/simulator"
)

var (
	loglevel = flag.Int("loglevel", 5, "verbosity of logs")
)

// main() starts a simulation network which contains nodes running a simple
// ping-pong protocol
func main() {
	flag.Parse()

	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(*loglevel), log.StreamHandler(colorable.NewColorableStderr(), log.TerminalFormat(true))))

	// Create LES cluster
	cluster, err := simulator.NewCluster(&simulator.ClusterConfig{
		Adapter:      "sim",
		ChainID:      1337,
		ClientConfig: []*simulator.ClientServiceConfig{{TrustedServers: nil, TrustedFraction: 0}},
		ServerConfig: []*simulator.ServerServiceConfig{
			{
				LightServ:  100,
				LightPeers: 50,
			},
		},
		Blocks:                10,
		DeployPaymentContract: true,
		DeployOracleContract:  true,
		Conns:                 nil,
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
