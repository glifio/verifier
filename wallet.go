package main

import (
	"context"
	"encoding/base64"

	"github.com/filecoin-project/go-address"
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

func instantiateWallet(ctx context.Context) (error) {
	keystore := wallet.NewMemKeyStore()
	w, err := wallet.NewWallet(keystore)
	if err != nil { return err }

	if env.Mode == FaucetMode {
		pk, err := base64.StdEncoding.DecodeString(env.FaucetPrivateKey)
		if err != nil { return err }

		FaucetAddr, err = w.WalletImport(ctx, &types.KeyInfo{Type: types.KTBLS, PrivateKey: pk})
		if err != nil { return err }

		return nil
	} else if env.Mode == VerifierMode {
		pk, err := base64.StdEncoding.DecodeString(env.VerifierPrivateKey)
		if err != nil { return err }

		VerifierAddr, err = w.WalletImport(ctx, &types.KeyInfo{Type: types.KTBLS, PrivateKey: pk})
		if err != nil { return err }

		return nil
	}

	pkVerifier, err := base64.StdEncoding.DecodeString(env.FaucetPrivateKey)
	if err != nil { return err }
	VerifierAddr, err = w.WalletImport(ctx, &types.KeyInfo{Type: types.KTBLS, PrivateKey: pkVerifier})
	if err != nil { return err }

	pkFaucet, err := base64.StdEncoding.DecodeString(env.FaucetPrivateKey)
	if err != nil { return err }
	FaucetAddr, err = w.WalletImport(ctx, &types.KeyInfo{Type: types.KTBLS, PrivateKey: pkFaucet})
	if err != nil { return err }

	return nil
}

func getFaucetAddress(ctx context.Context) (string) {
	return ""
}

func getVerifierAddress(ctx context.Context) (string) {
	return ""
}
