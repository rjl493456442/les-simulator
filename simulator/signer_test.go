package simulator

import (
	"io/ioutil"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/external"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func TestLookup(t *testing.T) {
	// Initialize signing daemon if required
	clefPath, err := ioutil.TempDir("", "clef-dir")
	if err != nil {
		t.Fatalf("Failed to new temp directory")
	}

	// Create accounts for simulations
	keystorePath, err := ioutil.TempDir("", "simulation-keystore")
	if err != nil {
		t.Fatalf("Failed to create temp dir, err %v", err)
	}
	defer os.RemoveAll(keystorePath)

	var prefunds = make(map[common.Address]*big.Int)
	var localAccounts []common.Address
	for i := 0; i < 2; i++ {
		account, err := keystore.StoreKey(keystorePath, "foobar", keystore.LightScryptN, keystore.LightScryptP)
		if err != nil {
			t.Fatalf("Failed to create test account file, err: %v", err)
		}
		prefunds[account.Address] = big.NewInt(params.Ether)
		localAccounts = append(localAccounts, account.Address)
	}
	// Create clef daemon
	signer, err := NewClefDaemon(&ClefConfig{
		Dir:      clefPath,
		Keystore: keystorePath,
		ChainID:  1337,
		Rules:    signingRules,
		Accounts: map[common.Address]string{localAccounts[0]: ""},
	})
	if err != nil {
		t.Fatalf("Failed to create clef daemon, error %v", err)
	}

	// Create wallet backend
	extapi, err := external.NewExternalBackend(signer.RPCURL())
	if err != nil {
		t.Fatalf("Failed to initialize backend")
	}

	// Create account manager with backends
	accMgr := accounts.NewManager(&accounts.Config{InsecureUnlockAllowed: true}, extapi)

	account := accounts.Account{Address: localAccounts[0]}
	_, err = accMgr.Find(account)
	if err != nil {
		t.Fatalf("Failed to lookup account %v", err)
	}
	account2 := accounts.Account{Address: common.HexToAddress("deadbeef")}
	_, err = accMgr.Find(account2)
	if err == nil {
		t.Fatalf("unknown account expected")
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
