package simulator

import (
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/external"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/les"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p/simulations/adapters"
)

// BlockchainConfig contains the setting for chain state.
type BlockchainConfig struct {
	Genesis *core.Genesis  // Nil if no customized genesis is required
	Chain   []*types.Block // Nil if the initial state is empty
}

type ClientServiceConfig struct {
	// LES settings
	ServicePay      bool
	PaymentAddress  common.Address
	TrustedServers  []string
	TrustedFraction int
}

func NewLesClientService(cfg *ClientServiceConfig, bcfg *BlockchainConfig, signer string) func(ctx *adapters.ServiceContext) (node.Service, error) {
	return func(ctx *adapters.ServiceContext) (service node.Service, e error) {
		// Using in-memory temporary database.
		ctx.Config.DataDir = ""
		backend, err := external.NewExternalBackend(signer)
		if err != nil {
			return nil, err
		}
		ctx.NodeContext.AccountManager = accounts.NewManager(&accounts.Config{}, backend)

		config := eth.DefaultConfig
		config.SyncMode = downloader.LightSync
		config.Ethash.PowMode = ethash.ModeFake

		// Add more customized configs
		if cfg != nil {
			config.LightServicePay = cfg.ServicePay
			config.LightAddress = cfg.PaymentAddress
			config.UltraLightServers = cfg.TrustedServers
			config.UltraLightFraction = cfg.TrustedFraction
		}
		if bcfg != nil && bcfg.Genesis != nil {
			config.Genesis = bcfg.Genesis
		}
		les, err := les.New(ctx.NodeContext, &config)
		if err != nil {
			return nil, err
		}
		// Do initialization.
		if bcfg != nil && len(bcfg.Chain) > 0 {
			var headers []*types.Header
			for _, block := range bcfg.Chain {
				headers = append(headers, block.Header())
			}
			les.BlockChain().InsertHeaderChain(headers, 0)
		}
		return les, nil
	}
}

type ServerServiceConfig struct {
	ServiceCharge  bool
	PaymentAddress common.Address
	LightServ      int
	LightPeers     int
}

func NewLesServerService(cfg *ServerServiceConfig, bcfg *BlockchainConfig, mining bool, signer string) func(ctx *adapters.ServiceContext) (node.Service, error) {
	return func(ctx *adapters.ServiceContext) (service node.Service, e error) {
		// Using in-memory temporary database.
		ctx.Config.DataDir = ""
		backend, err := external.NewExternalBackend(signer)
		if err != nil {
			return nil, err
		}
		ctx.NodeContext.AccountManager = accounts.NewManager(&accounts.Config{}, backend)

		config := eth.DefaultConfig
		config.SyncMode = downloader.FullSync
		config.Ethash.PowMode = ethash.ModeFake
		config.Miner.Etherbase = common.HexToAddress("deadbeef")

		// Add more customized configs
		if cfg != nil {
			config.LightServiceCharge = cfg.ServiceCharge
			config.LightAddress = cfg.PaymentAddress
			config.LightServ = cfg.LightServ
			config.LightPeers = cfg.LightPeers
		}
		if bcfg != nil && bcfg.Genesis != nil {
			config.Genesis = bcfg.Genesis
		}
		eth, err := eth.New(ctx.NodeContext, &config)
		if err != nil {
			return nil, err
		}
		// Do initialization.
		if bcfg != nil && len(bcfg.Chain) > 0 {
			eth.BlockChain().InsertChain(bcfg.Chain)
		}
		server, err := les.NewLesServer(ctx.NodeContext, eth, &config)
		if err != nil {
			return nil, err
		}
		eth.AddLesServer(server)

		// If mining is required, start it
		if mining {
			eth.Miner().DisablePreseal()
			eth.StartMining(1)
		}
		return eth, nil
	}
}
