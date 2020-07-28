package main

import (
	"bytes"
	"context"
	"log"
	"strings"
	"time"

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

func lotusMakeAccountAVerifier(ctx context.Context, targetAddr string, allowanceStr string) error {
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

	api, closer, err := lotusGetFullNodeAPI(ctx)
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

	smsg, err := api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return err
	}

	ok, err := lotusWaitMessageResult(ctx, smsg.Cid())
	if err != nil {
		return err
	} else if !ok {
		return errors.New("failed to make account a verifier")
	}
	return nil
}

func lotusVerifyAccount(ctx context.Context, targetAddr string, allowanceStr string) (cid.Cid, error) {
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

	api, closer, err := lotusGetFullNodeAPI(ctx)
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

func lotusListVerifiers(ctx context.Context) ([]AddrAndDataCap, error) {
	api, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		return nil, err
	}
	defer closer()

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

	vh, err := hamt.LoadNode(ctx, cst, st.Verifiers, hamt.UseTreeBitWidth(5))
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

func lotusListVerifiedClients(ctx context.Context) ([]AddrAndDataCap, error) {
	api, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		return nil, err
	}
	defer closer()

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

	vh, err := hamt.LoadNode(ctx, cst, st.VerifiedClients, hamt.UseTreeBitWidth(5))
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
	if err != nil && strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

func lotusCheckAccountRemainingBytes(ctx context.Context, targetAddr string) (big.Int, error) {
	caddr, err := address.NewFromString(targetAddr)
	if err != nil {
		return big.Int{}, err
	}

	api, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		return big.Int{}, err
	}
	defer closer()

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

	vh, err := hamt.LoadNode(ctx, cst, st.VerifiedClients, hamt.UseTreeBitWidth(5))
	if ignoreNotFound(err) != nil {
		return big.Int{}, err
	}

	var dcap verifreg.DataCap
	if err := vh.Find(ctx, string(caddr.Bytes()), &dcap); ignoreNotFound(err) != nil {
		return big.Int{}, err
	}

	if dcap.Int != nil {
		return dcap, nil
	}
	return big.NewInt(0), nil
}

func lotusCheckVerifierRemainingBytes(ctx context.Context, targetAddr string) (big.Int, error) {
	vaddr, err := address.NewFromString(targetAddr)
	if err != nil {
		return big.Int{}, err
	}

	api, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		return big.Int{}, err
	}
	defer closer()

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

	vh, err := hamt.LoadNode(ctx, cst, st.Verifiers, hamt.UseTreeBitWidth(5))
	if err != nil {
		return big.Int{}, err
	}

	var dcap verifreg.DataCap
	if err := vh.Find(ctx, string(vaddr.Bytes()), &dcap); err != nil {
		return big.Int{}, err
	}
	return dcap, nil
}

func lotusGetFullNodeAPI(ctx context.Context) (apiClient api.FullNode, closer jsonrpc.ClientCloser, err error) {
	err = retry(ctx, func() error {
		ainfo := lcli.APIInfo{Token: []byte(env.LotusAPIToken)}

		var innerErr error
		apiClient, closer, innerErr = client.NewFullNodeRPC(env.LotusAPIDialAddr, ainfo.AuthHeader())
		return innerErr
	})
	return
}

func lotusCheckBalance(ctx context.Context, address address.Address) (types.FIL, error) {
	api, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		return types.FIL{}, err
	}
	defer closer()

	balance, err := api.WalletBalance(ctx, address)
	if err != nil {
		return types.FIL{}, err
	}
	return types.FIL(balance), nil
}

func lotusSendFIL(ctx context.Context, fromAddr, toAddr address.Address, gasPrice, filAmount types.FIL) (cid.Cid, error) {
	api, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		return cid.Cid{}, err
	}
	defer closer()

	msg := &types.Message{
		From:     fromAddr,
		To:       toAddr,
		Value:    types.BigInt(filAmount),
		GasLimit: 10000,
		GasPrice: types.BigInt(gasPrice),
	}

	sm, err := api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return cid.Cid{}, err
	}
	return sm.Cid(), nil
}

func lotusWaitMessageResult(ctx context.Context, cid cid.Cid) (bool, error) {
	client, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		log.Println("error getting FullNodeAPI:", err)
		return false, err
	}
	defer closer()

	var mwait *api.MsgLookup
	err = retry(ctx, func() error {
		mwait, err = client.StateWaitMsg(ctx, cid, build.MessageConfidence)
		return err
	})
	if err != nil {
		log.Println("error awaiting message result:", err)
		return false, err
	}
	return mwait.Receipt.ExitCode == 0, nil
}

func retry(ctx context.Context, fn func() error) (err error) {
	wait := 5 * time.Second
	for {
		select {
		case <-ctx.Done():
			return err
		default:
		}

		err = fn()
		if err != nil {
			time.Sleep(wait)
			wait += wait / 2
			continue
		}
		return nil
	}
}

func withStack(err *error) {
	if *err != nil {
		*err = errors.WithStack(*err)
	}
}
