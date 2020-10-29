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
	servers  = flag.Int("servers", 10, "the number of les servers to be created")
	clients  = flag.Int("clients", 10, "the number of les clients to be created")
	routes   = flag.String("routes", "", "the network topology to be created, separated by comma(e.g. c1->s2,c2->s1,c3->*,*->s4)")
)

// main() starts a simulation network which contains nodes running a simple
// ping-pong protocol
func main() {
	flag.Parse()

	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(*loglevel), log.StreamHandler(colorable.NewColorableStderr(), log.TerminalFormat(true))))

	if *servers == 0 && *clients == 0 {
		log.Crit("Invalid network topology setting")
	}
	var (
		serverConfigs []*simulator.ServerServiceConfig
		clientConfigs []*simulator.ClientServiceConfig
		conns         []*simulator.Conn
	)
	for i := 0; i < *servers; i++ {
		serverConfigs = append(serverConfigs, &simulator.ServerServiceConfig{
			LightServ:  100,
			LightPeers: 50,
		})
	}
	for i := 0; i < *clients; i++ {
		clientConfigs = append(clientConfigs, &simulator.ClientServiceConfig{TrustedServers: nil, TrustedFraction: 0})
	}
	if *routes != "" {
		conns = simulator.ParseTopology(*routes, *clients, *servers)
	}
	// Create LES cluster
	cluster, err := simulator.NewCluster(&simulator.ClusterConfig{
		Adapter:               "sim",
		ChainID:               1337,
		ClientConfig:          clientConfigs,
		ServerConfig:          serverConfigs,
		Blocks:                10,
		DeployPaymentContract: true,
		DeployOracleContract:  true,
		Conns:                 conns,
	})
	if err != nil {
		log.Crit("Failed to create les cluster", "error", err)
	}
	log.Info("starting cluster....")
	cluster.StartNodes()

	log.Info("Connecting nodes....")
	if err := cluster.Connect(); err != nil {
		log.Error("Connection failure", "error", err)
	}

	// start the HTTP API
	log.Info("starting simulation server on 0.0.0.0:9999...")
	if err := http.ListenAndServe(":9999", simulations.NewServer(cluster.Network())); err != nil {
		log.Crit("error starting simulation server", "err", err)
	}
}
