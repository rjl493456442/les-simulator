package simulator

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	oracle "github.com/ethereum/go-ethereum/contracts/checkpointoracle/contract"
	lottery "github.com/ethereum/go-ethereum/contracts/lotterybook/contract"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/simulations"
	"github.com/ethereum/go-ethereum/p2p/simulations/adapters"
	"github.com/ethereum/go-ethereum/params"
)

type LesServer struct {
	node   *simulations.Node
	signer *ClefDaemon
}

type LesClient struct {
	node   *simulations.Node
	signer *ClefDaemon
}

type Conn struct {
	From int // Client index
	To   int // Server index
}

type ClusterConfig struct {
	Adapter      string
	ClientConfig []*ClientServiceConfig
	ServerConfig []*ServerServiceConfig
	Conns        []*Conn // Nil mean each client will connect to all servers.

	// Initial blockchain state.
	ChainID               int64
	Blocks                int  // Initial blockchain length, 0 means genesis only.
	DeployPaymentContract bool // Whether deploy payment contract in blockchain
	DeployOracleContract  bool // Whether deploy checkpoint oracle contract in blockchain
	Prefunds              map[common.Address]*big.Int

	// Additional services
	KeystorePath string
	SigningRule  []byte
}

type Cluster struct {
	lock    sync.RWMutex
	config  *ClusterConfig
	servers []*LesServer
	clients []*LesClient
	network *simulations.Network

	// Blockchain state
	oracleAddress  common.Address
	lotteryAddress common.Address

	// Signing state
	keystore keystore.KeyStore
}

func NewCluster(config *ClusterConfig) (*Cluster, error) {
	var (
		gspec = core.Genesis{
			Config:     params.AllEthashProtocolChanges,
			GasLimit:   4700000,
			Difficulty: big.NewInt(5242880),
		}
		masterKey, _ = crypto.GenerateKey()
		masterAddr   = crypto.PubkeyToAddress(masterKey.PublicKey)
	)
	gspec.Alloc = make(core.GenesisAlloc)
	gspec.Alloc[masterAddr] = core.GenesisAccount{Balance: big.NewInt(params.Ether)}
	if config.Prefunds != nil {
		for address, fund := range config.Prefunds {
			gspec.Alloc[address] = core.GenesisAccount{Balance: fund}
		}
	}
	db := rawdb.NewMemoryDatabase()
	genesis := gspec.MustCommit(db)

	// Pre-generate blockchain as the initial state
	var (
		oracleAddr  common.Address
		lotteryAddr common.Address

		blocks []*types.Block
	)
	if config.Blocks > 0 {
		sim := backends.NewSimulatedBackendWithDatabase(db, gspec.Alloc, 100000000)
		blocks, _ = core.GenerateChain(gspec.Config, genesis, ethash.NewFaker(), db, config.Blocks, func(i int, gen *core.BlockGen) {
			var tx *types.Transaction
			switch {
			case i == 1 && config.DeployOracleContract:
				// deploy checkpoint contract
				opt := bind.NewKeyedTransactor(masterKey)
				opt.GasPrice = big.NewInt(2 * params.GWei)
				oracleAddr, tx, _, _ = oracle.DeployCheckpointOracle(opt, sim, []common.Address{masterAddr}, big.NewInt(128), big.NewInt(1), big.NewInt(1))
				gen.AddTx(tx)
			case i == 2 && config.DeployPaymentContract:
				opt := bind.NewKeyedTransactor(masterKey)
				opt.GasPrice = big.NewInt(2 * params.GWei)
				lotteryAddr, tx, _, _ = lottery.DeployLotteryBook(opt, sim)
				gen.AddTx(tx)
			default:
			}
			sim.Commit()
		})
	}
	bcfg := &BlockchainConfig{
		Genesis: &gspec,
		Chain:   blocks,
	}
	// Register all services
	var (
		services = make(map[string]adapters.LifecycleConstructor)

		serverDaemons []*ClefDaemon
		clientDaemons []*ClefDaemon
	)
	for index, server := range config.ServerConfig {
		// Initialize signing daemon if required
		clefPath, err := ioutil.TempDir("", fmt.Sprintf("server-clef-%d", index))
		if err != nil {
			return nil, err
		}
		d, err := NewClefDaemon(&ClefConfig{
			Dir:      clefPath,
			Keystore: config.KeystorePath,
			ChainID:  config.ChainID,
			Rules:    config.SigningRule,
			Accounts: map[common.Address]string{server.PaymentAddress: ""},
		})
		if err != nil {
			return nil, err
		}
		serverDaemons = append(serverDaemons, d)
		services[fmt.Sprintf("les-server-%d", index)] = NewLesServerService(server, bcfg, index == 0)
	}
	for index, client := range config.ClientConfig {
		// Initialize signing daemon if required
		clefPath, err := ioutil.TempDir("", fmt.Sprintf("client-clef-%d", index))
		if err != nil {
			return nil, err
		}
		d, err := NewClefDaemon(&ClefConfig{
			Dir:      clefPath,
			Keystore: config.KeystorePath,
			ChainID:  config.ChainID,
			Rules:    config.SigningRule,
			Accounts: map[common.Address]string{client.PaymentAddress: ""},
		})
		if err != nil {
			return nil, err
		}
		clientDaemons = append(clientDaemons, d)
		services[fmt.Sprintf("les-client-%d", index)] = NewLesClientService(client, bcfg)
	}
	adapter, err := NewAdapter(config.Adapter, services)
	if err != nil {
		return nil, err
	}
	net := simulations.NewNetwork(adapter, &simulations.NetworkConfig{ID: "0"})

	cluster := &Cluster{
		network:        net,
		config:         config,
		oracleAddress:  oracleAddr,
		lotteryAddress: lotteryAddr,
	}
	// Initialize all nodes
	for index := range config.ServerConfig {
		cfg := adapters.RandomNodeConfig()
		cfg.Lifecycles = []string{fmt.Sprintf("les-server-%d", index)}
		cfg.Properties = []string{"server"}
		cfg.ExternalSigner = serverDaemons[index].RPCURL()
		server, err := net.NewNodeWithConfig(cfg)
		if err != nil {
			return nil, err
		}
		cluster.servers = append(cluster.servers, &LesServer{node: server, signer: serverDaemons[index]})
	}
	for index := range config.ClientConfig {
		cfg := adapters.RandomNodeConfig()
		cfg.Lifecycles = []string{fmt.Sprintf("les-client-%d", index)}
		cfg.Properties = []string{"client"}
		cfg.ExternalSigner = clientDaemons[index].RPCURL()
		client, err := net.NewNodeWithConfig(cfg)
		if err != nil {
			return nil, err
		}
		cluster.clients = append(cluster.clients, &LesClient{node: client, signer: clientDaemons[index]})
	}
	// Register system level contracts
	if lotteryAddr != (common.Address{}) {
		params.PaymentContracts[genesis.Hash()] = lotteryAddr
	}
	if oracleAddr != (common.Address{}) {
		params.CheckpointOracles[genesis.Hash()] = &params.CheckpointOracleConfig{
			Address:   oracleAddr,
			Signers:   []common.Address{masterAddr},
			Threshold: 1,
		}
	}
	return cluster, nil
}

func (cluster *Cluster) StartNodes() error {
	cluster.lock.Lock()
	defer cluster.lock.Unlock()

	for _, server := range cluster.servers {
		cluster.network.Start(server.node.ID())
	}
	log.Info("Started all servers")
	for _, client := range cluster.clients {
		cluster.network.Start(client.node.ID())
	}
	log.Info("Started all clients")
	return nil
}

func (cluster *Cluster) StopNodes() error {
	cluster.lock.Lock()
	defer cluster.lock.Unlock()

	for _, server := range cluster.servers {
		cluster.network.Stop(server.node.ID())
		server.signer.Stop()
	}
	for _, client := range cluster.clients {
		cluster.network.Stop(client.node.ID())
		client.signer.Stop()
	}
	return nil
}

func (cluster *Cluster) Connect() error {
	cluster.lock.Lock()
	defer cluster.lock.Unlock()

	if cluster.config.Conns == nil {
		// Connect each client to all servers.
		for _, client := range cluster.clients {
			for _, server := range cluster.servers {
				if err := cluster.network.Connect(client.node.ID(), server.node.ID()); err != nil {
					log.Error("Failed to establish the connection", "from", client.node.ID(), "to", server.node.ID(), "error", err)
					return err
				}
			}
		}
	} else {
		// Connect clients and servers with specified topology.
		for _, conn := range cluster.config.Conns {
			if conn.From >= len(cluster.clients) {
				return errors.New("invalid client index")
			}
			if conn.To >= len(cluster.servers) {
				return errors.New("invalid server index")
			}
			if err := cluster.network.Connect(cluster.clients[conn.From].node.ID(), cluster.servers[conn.To].node.ID()); err != nil {
				return err
			}
		}
	}
	// Connect servers together
	for i := range cluster.servers {
		for j := i + 1; j < len(cluster.servers); j++ {
			if err := cluster.network.Connect(cluster.servers[i].node.ID(), cluster.servers[j].node.ID()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (cluster *Cluster) Disconnect() error {
	cluster.lock.Lock()
	defer cluster.lock.Unlock()

	if cluster.config.Conns == nil {
		// Disconnect each client with all servers.
		for _, client := range cluster.clients {
			for _, server := range cluster.servers {
				if err := cluster.network.Disconnect(client.node.ID(), server.node.ID()); err != nil {
					return err
				}
			}
		}
	} else {
		// Disconnect clients and servers with specified topology.
		for _, conn := range cluster.config.Conns {
			if conn.From >= len(cluster.clients) {
				return errors.New("invalid client index")
			}
			if conn.To >= len(cluster.servers) {
				return errors.New("invalid server index")
			}
			if err := cluster.network.Disconnect(cluster.clients[conn.From].node.ID(), cluster.servers[conn.To].node.ID()); err != nil {
				return err
			}
		}
	}
	// Disconnect servers
	for i := range cluster.servers {
		for j := i + 1; j < len(cluster.servers); j++ {
			if err := cluster.network.Disconnect(cluster.servers[i].node.ID(), cluster.servers[j].node.ID()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (cluster *Cluster) Network() *simulations.Network {
	return cluster.network
}

func NewAdapter(typ string, services adapters.LifecycleConstructors) (adapters.NodeAdapter, error) {
	switch typ {
	case "sim":
		return adapters.NewSimAdapter(services), nil
	default:
		return nil, errors.New("invalid adapter")
	}
}
