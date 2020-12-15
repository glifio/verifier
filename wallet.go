package main

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
)

var w wallet.LocalWallet
// FaucetAddr export
var FaucetAddr address.Address
// VerifierAddr export
var VerifierAddr address.Address

func handleErr(err error) error {
	if err != nil {
		return err
	}
	return nil
}

func importFaucetKey(ctx context.Context, w *wallet.LocalWallet) error {
	pk, err := base64.StdEncoding.DecodeString(env.FaucetPrivateKey)
	if err != nil { return err }

	FaucetAddr, err = w.WalletImport(ctx, &types.KeyInfo{Type: types.KTBLS, PrivateKey: pk})
	if err != nil { return err }
	fmt.Println("IMPORTED FAUCET KEY: ", FaucetAddr)
	return nil
}

func importVerifierKey(ctx context.Context, w *wallet.LocalWallet) error {
	pk, err := base64.StdEncoding.DecodeString(env.VerifierPrivateKey)
	if err != nil { return err }

	VerifierAddr, err = w.WalletImport(ctx, &types.KeyInfo{Type: types.KTBLS, PrivateKey: pk})
	if err != nil { return err }
	fmt.Println("IMPORTED VERIFIER KEY: ", VerifierAddr)
	return nil
}

func instantiateWallet(ctx context.Context) (w *wallet.LocalWallet, err error) {
	keystore := wallet.NewMemKeyStore()
	w, err = wallet.NewWallet(keystore)
	if err != nil { return w, err }

	if env.Mode == FaucetMode {
		if err := importFaucetKey(ctx, w); err != nil { return w, err }
		return w, nil
	} else if env.Mode == VerifierMode {
		if err := importVerifierKey(ctx, w); err != nil { return w, err }
		return w, nil
	}

	if err := importFaucetKey(ctx, w); err != nil { return w, err }
	if err := importVerifierKey(ctx, w); err != nil { return w, err }

	return w, nil
}

func walletSignMessage(ctx context.Context, signerAddr address.Address, message []byte, msgMeta api.MsgMeta) (*crypto.Signature, error) {
	w, err := instantiateWallet(ctx)
	if err != nil { return &crypto.Signature{}, err }
	return w.WalletSign(ctx, signerAddr, message, msgMeta)
}
