package simulator

import (
	"errors"
	"net"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/signer/core"
	"github.com/ethereum/go-ethereum/signer/fourbyte"
	"github.com/ethereum/go-ethereum/signer/rules"
	"github.com/ethereum/go-ethereum/signer/storage"
)

const (
	DefaultMasterSeed = "foobar"
	DefaultAccountPWD = "foobar"
)

type ClefConfig struct {
	Dir        string
	Keystore   string
	ChainID    int64
	MasterSeed string                    // Default seed is used if empty
	Rules      []byte                    // Empty means no additional rules
	Accounts   map[common.Address]string // Unlock account
}

type ClefDaemon struct {
	config   *ClefConfig
	listener net.Listener
	server   *rpc.Server
	rpcURL   string
	ui       core.UIClientAPI
}

func NewClefDaemon(config *ClefConfig) (*ClefDaemon, error) {
	// Check config validity
	if config == nil {
		return nil, errors.New("emoty config")
	}
	if config.Dir == "" {
		return nil, errors.New("no directory specified")
	}
	if config.Keystore == "" {
		return nil, errors.New("no keystore specified")
	}
	var ui core.UIClientAPI
	ui = core.NewCommandlineUI()
	fbdb, err := fourbyte.NewWithFile("")
	if err != nil {
		return nil, errors.New("failed to open fourbyte db")
	}
	masterSeed := config.MasterSeed
	if masterSeed == "" {
		masterSeed = DefaultMasterSeed
	}
	vaultLocation := filepath.Join(config.Dir, common.Bytes2Hex(crypto.Keccak256([]byte("vault"), []byte(masterSeed))[:10]))

	// Generate domain specific keys
	pwkey := crypto.Keccak256([]byte("credentials"), []byte(masterSeed))
	jskey := crypto.Keccak256([]byte("jsstorage"), []byte(masterSeed))

	// Initialize the encrypted storages
	pwStorage := storage.NewAESEncryptedStorage(filepath.Join(vaultLocation, "credentials.json"), pwkey)
	jsStorage := storage.NewAESEncryptedStorage(filepath.Join(vaultLocation, "jsstorage.json"), jskey)

	// Do we have a rule-file?
	if len(config.Rules) > 0 {
		// Initialize rules
		ruleEngine, err := rules.NewRuleEvaluator(ui, jsStorage)
		if err != nil {
			log.Crit("Failed to init rule evaluator", "err", err)
		}
		ruleEngine.Init(string(config.Rules))
		ui = ruleEngine
	}
	if config.Accounts != nil {
		for account, pwd := range config.Accounts {
			if pwd == "" {
				pwd = DefaultAccountPWD
			}
			pwStorage.Put(account.Hex(), pwd)
		}
	}

	am := core.StartClefAccountManager(config.Keystore, true, true, "") // Light KDF = true
	apiImpl := core.NewSignerAPI(am, config.ChainID, true, ui, fbdb, true, pwStorage)

	// Establish the bidirectional communication, by creating a new UI backend and registering
	// it with the UI.
	ui.RegisterUIServer(core.NewUIServerAPI(apiImpl))

	rpcAPI := []rpc.API{{
		Namespace: "account",
		Public:    true,
		Service:   apiImpl,
		Version:   "1.0",
	}}
	ipcapiURL := filepath.Join(config.Dir, "clef.ipc")
	listener, rpcServer, err := rpc.StartIPCEndpoint(ipcapiURL, rpcAPI)
	if err != nil {
		log.Crit("Could not start IPC api", "err", err)
	}
	log.Info("IPC endpoint opened", "url", ipcapiURL)
	return &ClefDaemon{
		config:   config,
		listener: listener,
		server:   rpcServer,
		rpcURL:   ipcapiURL,
		ui:       ui,
	}, nil
}

func (c *ClefDaemon) Stop() {
	c.listener.Close()
}

func (c *ClefDaemon) RPCURL() string {
	return c.rpcURL
}
