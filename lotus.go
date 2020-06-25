package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
)

func lotusMakeAccountAVerifier(targetAddr string, allowanceStr string) error {
	target, err := address.NewFromString(targetAddr)
	if err != nil {
		return err
	}

	allowance, err := types.BigFromString(allowanceStr)
	if err != nil {
		return err
	}

	params, err := actors.SerializeParams(&verifreg.AddVerifierParams{Address: target, Allowance: allowance})
	if err != nil {
		return err
	}

	api, closer, err := GetFullNodeAPI()
	if err != nil {
		return err
	}
	defer closer()

	msg := &types.Message{
		To:       builtin.VerifiedRegistryActorAddr,
		From:     env.LotusVerifierAddr,
		Method:   builtin.MethodsVerifiedRegistry.AddVerifier,
		GasPrice: types.NewInt(1),
		GasLimit: 300000,
		Params:   params,
	}

	ctx := context.TODO()

	smsg, err := api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return err
	}

	mwait, err := api.StateWaitMsg(ctx, smsg.Cid(), build.MessageConfidence)
	if err != nil {
		return err
	}

	if mwait.Receipt.ExitCode != 0 {
		return fmt.Errorf("failed to add verifier: %d", mwait.Receipt.ExitCode)
	}

	return nil
}

func lotusVerifyAccount(targetAddr string, allowanceStr string) (cid.Cid, error) {
	target, err := address.NewFromString(targetAddr)
	if err != nil {
		return cid.Cid{}, err
	}

	allowance, err := types.BigFromString(allowanceStr)
	if err != nil {
		return cid.Cid{}, err
	}

	params, err := actors.SerializeParams(&verifreg.AddVerifiedClientParams{Address: target, Allowance: allowance})
	if err != nil {
		return cid.Cid{}, err
	}

	api, closer, err := GetFullNodeAPI()
	if err != nil {
		return cid.Cid{}, err
	}
	defer closer()

	msg := &types.Message{
		To:       builtin.VerifiedRegistryActorAddr,
		From:     env.LotusVerifierAddr,
		Method:   builtin.MethodsVerifiedRegistry.AddVerifiedClient,
		GasPrice: types.NewInt(1),
		GasLimit: 300000,
		Params:   params,
	}

	ctx := context.TODO()

	smsg, err := api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return cid.Cid{}, err
	}

	return smsg.Cid(), nil
}

type AddrAndDataCap struct {
	Address address.Address
	DataCap verifreg.DataCap
}

func lotusListVerifiers() ([]AddrAndDataCap, error) {
	api, closer, err := GetFullNodeAPI()
	if err != nil {
		return nil, err
	}
	defer closer()

	ctx := context.TODO()

	act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	apibs := apibstore.NewAPIBlockstore(api)
	cst := cbor.NewCborStore(apibs)

	var st verifreg.State
	if err := cst.Get(ctx, act.Head, &st); err != nil {
		return nil, err
	}

	vh, err := hamt.LoadNode(ctx, cst, st.Verifiers)
	if err != nil {
		return nil, err
	}

	var resp []AddrAndDataCap

	err = vh.ForEach(ctx, func(k string, val interface{}) error {
		addr, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}

		var dcap verifreg.DataCap
		if err := dcap.UnmarshalCBOR(bytes.NewReader(val.(*cbg.Deferred).Raw)); err != nil {
			return err
		}
		resp = append(resp, AddrAndDataCap{addr, dcap})
		return nil
	})
	return resp, err
}

func lotusListVerifiedClients() ([]AddrAndDataCap, error) {
	api, closer, err := GetFullNodeAPI()
	if err != nil {
		return nil, err
	}
	defer closer()

	ctx := context.TODO()

	act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	apibs := apibstore.NewAPIBlockstore(api)
	cst := cbor.NewCborStore(apibs)

	var st verifreg.State
	if err := cst.Get(ctx, act.Head, &st); err != nil {
		return nil, err
	}

	vh, err := hamt.LoadNode(ctx, cst, st.VerifiedClients)
	if err != nil {
		return nil, err
	}

	var resp []AddrAndDataCap
	err = vh.ForEach(ctx, func(k string, val interface{}) error {
		addr, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}

		var dcap verifreg.DataCap
		if err := dcap.UnmarshalCBOR(bytes.NewReader(val.(*cbg.Deferred).Raw)); err != nil {
			return err
		}
		resp = append(resp, AddrAndDataCap{addr, dcap})
		return nil

	})
	return resp, err
}

func ignoreNotFound(err error) error {
	if strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

func lotusCheckAccountRemainingBytes(targetAddr string) (big.Int, error) {
	caddr, err := address.NewFromString(targetAddr)
	if err != nil {
		return big.Int{}, err
	}

	api, closer, err := GetFullNodeAPI()
	if err != nil {
		return big.Int{}, err
	}
	defer closer()

	ctx := context.TODO()

	act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
	if err != nil {
		return big.Int{}, err
	}

	apibs := apibstore.NewAPIBlockstore(api)
	cst := cbor.NewCborStore(apibs)

	var st verifreg.State
	if err := cst.Get(ctx, act.Head, &st); ignoreNotFound(err) != nil {
		return big.Int{}, err
	}

	vh, err := hamt.LoadNode(ctx, cst, st.VerifiedClients)
	if ignoreNotFound(err) != nil {
		return big.Int{}, err
	}

	var dcap verifreg.DataCap
	if err := vh.Find(ctx, string(caddr.Bytes()), &dcap); ignoreNotFound(err) != nil {
		return big.Int{}, err
	}
	return dcap, nil
}

func lotusCheckVerifierRemainingBytes(targetAddr string) (big.Int, error) {
	vaddr, err := address.NewFromString(targetAddr)
	if err != nil {
		return big.Int{}, err
	}

	api, closer, err := GetFullNodeAPI()
	if err != nil {
		return big.Int{}, err
	}
	defer closer()

	ctx := context.TODO()

	act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
	if err != nil {
		return big.Int{}, err
	}

	apibs := apibstore.NewAPIBlockstore(api)
	cst := cbor.NewCborStore(apibs)

	var st verifreg.State
	if err := cst.Get(ctx, act.Head, &st); err != nil {
		return big.Int{}, err
	}

	vh, err := hamt.LoadNode(ctx, cst, st.Verifiers)
	if err != nil {
		return big.Int{}, err
	}

	var dcap verifreg.DataCap
	if err := vh.Find(ctx, string(vaddr.Bytes()), &dcap); err != nil {
		return big.Int{}, err
	}
	return dcap, nil
}

func GetFullNodeAPI() (api.FullNode, jsonrpc.ClientCloser, error) {
	ainfo := lcli.APIInfo{
		Token: []byte(env.LotusAPIToken),
	}
	return client.NewFullNodeRPC(
		env.LotusAPIDialAddr,
		ainfo.AuthHeader(),
	)
}

func withStack(err *error) {
	if *err != nil {
		*err = errors.WithStack(*err)
	}
}
