package main

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/glifio/go-logger"
	"github.com/pkg/errors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/v0api"
	apibstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregany "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
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
		Method: builtin.MethodsVerifiedRegistry.AddVerifiedClient,
		Params: params,
		Nonce:  nonce,
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

	head, err := api.ChainHead(ctx)
	if err != nil {
		return big.Int{}, err
	}

	act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, head.Key())
	if err != nil {
		return big.Int{}, err
	}

	vid, err := api.StateLookupID(ctx, vaddr, head.Key())
	if err != nil {
		return big.Int{}, err
	}

	apibs := apibstore.NewAPIBlockstore(api)
	store := adt.WrapStore(ctx, cbor.NewCborStore(apibs))

	st, err := verifregany.Load(store, act)
	if err != nil {
		return big.Int{}, err
	}

	found, dcap, err := st.VerifierDataCap(vid)
	if err != nil {
		return big.Int{}, err
	}
	if !found {
		return big.Int{}, errors.New("not found")
	}

	return dcap, nil
}

func lotusGetFullNodeAPI(ctx context.Context) (apiClient v0api.FullNode, closer jsonrpc.ClientCloser, err error) {
	err = retry(ctx, func() error {
		ainfo := cliutil.APIInfo{Token: []byte(env.LotusAPIToken)}

		var innerErr error
		apiClient, closer, innerErr = client.NewFullNodeRPCV0(ctx, env.LotusAPIDialAddr, ainfo.AuthHeader())
		return innerErr
	})
	return
}

func lotusSendFIL(ctx context.Context, lapi v0api.FullNode, fromAddr, toAddr address.Address, filAmount types.FIL) (cid.Cid, error) {
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

func lotusSearchMessageResult(ctx context.Context, cid cid.Cid) (*api.MsgLookup, error) {
	client, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		logger.Errorf("error getting FullNodeAPI: %v", err)
		return &api.MsgLookup{}, err
	}
	defer closer()

	var mLookup *api.MsgLookup
	mLookup, err = client.StateSearchMsg(ctx, cid)
	if err != nil {
		return &api.MsgLookup{}, err
	}

	return mLookup, nil
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
