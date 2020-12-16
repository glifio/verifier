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
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin/verifreg"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
)

func lotusVerifyAccount(ctx context.Context, targetAddr string, allowance types.BigInt) (cid.Cid, error) {
	target, err := address.NewFromString(targetAddr)
	if err != nil {
		return cid.Cid{}, err
	}

	params, err := actors.SerializeParams(&verifreg.AddVerifiedClientParams{Address: target, Allowance: allowance})
	if err != nil {
		return cid.Cid{}, err
	}

	lapi, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		return cid.Cid{}, err
	}
	defer closer()

	nonce, err := lapi.MpoolGetNonce(ctx, VerifierAddr)

	msg := &types.Message{
		To:     builtin.VerifiedRegistryActorAddr,
		From:   VerifierAddr,
		Method: builtin0.MethodsVerifiedRegistry.AddVerifiedClient,
		Params: params,
		Nonce: nonce,
	}

	sendSpec := &api.MessageSendSpec{
		MaxFee: types.BigInt(env.MaxFee),
	}

	msgWithGas, err := lapi.GasEstimateMessageGas(ctx, msg, sendSpec, types.EmptyTSK)
	if err != nil {
		return cid.Cid{}, err
	}

	sig, err := walletSignMessage(ctx, VerifierAddr, msgWithGas.Cid().Bytes(), api.MsgMeta{Type: api.MTUnknown})
	if err != nil {
		return cid.Cid{}, err
	}

	mCid, err := lapi.MpoolPush(ctx, &types.SignedMessage{Signature: *sig, Message: *msgWithGas})
	if err != nil {
		return cid.Cid{}, err
	}
	return mCid, nil
}

type addrAndDataCap struct {
	Address address.Address
	DataCap verifreg.DataCap
}

func lotusListVerifiers(ctx context.Context) ([]addrAndDataCap, error) {
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

	var resp []addrAndDataCap

	err = vh.ForEach(ctx, func(k string, val interface{}) error {
		addr, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}

		var dcap verifreg.DataCap
		if err := dcap.UnmarshalCBOR(bytes.NewReader(val.(*cbg.Deferred).Raw)); err != nil {
			return err
		}
		resp = append(resp, addrAndDataCap{addr, dcap})
		return nil
	})
	return resp, err
}

func lotusListVerifiedClients(ctx context.Context) ([]addrAndDataCap, error) {
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

	var resp []addrAndDataCap
	err = vh.ForEach(ctx, func(k string, val interface{}) error {
		addr, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}

		var dcap verifreg.DataCap
		if err := dcap.UnmarshalCBOR(bytes.NewReader(val.(*cbg.Deferred).Raw)); err != nil {
			return err
		}
		resp = append(resp, addrAndDataCap{addr, dcap})
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

	dcap, err := api.StateVerifiedClientStatus(ctx, caddr, types.EmptyTSK)
	err = ignoreNotFound(err)

	if err != nil {
		return big.Int{}, err
	}
	if dcap == nil || dcap.Int == nil {
		return big.NewInt(0), nil
	}
	return *dcap, nil
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
		ainfo := cliutil.APIInfo{Token: []byte(env.LotusAPIToken)}

		var innerErr error
		apiClient, closer, innerErr = client.NewFullNodeRPC(ctx, env.LotusAPIDialAddr, ainfo.AuthHeader())
		return innerErr
	})
	return
}

func lotusSendFIL(ctx context.Context, lapi api.FullNode, fromAddr, toAddr address.Address, filAmount types.FIL) (cid.Cid, error) {
	nonce, err := lapi.MpoolGetNonce(ctx, fromAddr)
	if err != nil {
		return cid.Cid{}, err
	}

	msg := &types.Message{
		From:  fromAddr,
		To:    toAddr,
		Value: types.BigInt(filAmount),
		Nonce: nonce,
	}

	sendSpec := &api.MessageSendSpec{
		MaxFee: types.BigInt(env.MaxFee),
	}

	msgWithGas, err := lapi.GasEstimateMessageGas(ctx, msg, sendSpec, types.EmptyTSK)
	if err != nil {
		return cid.Cid{}, err
	}
	sig, err := walletSignMessage(ctx, fromAddr, msgWithGas.Cid().Bytes(), api.MsgMeta{Type: api.MTUnknown})
	if err != nil {
		return cid.Cid{}, err
	}

	mCid, err := lapi.MpoolPush(ctx, &types.SignedMessage{Signature: *sig, Message: *msgWithGas})
	if err != nil {
		return cid.Cid{}, err
	}
	return mCid, nil
}

var errNotMiner = errors.New("not a miner")

func lotusTranslateError(err *error) {
	if *err == nil {
		return
	}
	if strings.Contains((*err).Error(), "not found") {
		*err = errNotMiner
	}
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
