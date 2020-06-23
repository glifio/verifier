package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multiaddr"
	cbg "github.com/whyrusleeping/cbor-gen"
	// "github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
)

func lotusMakeAccountAVerifier(targetAddr string, allowanceStr string) error {
	fromk, err := address.NewFromString("t3qfoulel6fy6gn3hjmbhpdpf6fs5aqjb5fkurhtwvgssizq4jey5nw4ptq5up6h7jk7frdvvobv52qzmgjinq")
	if err != nil {
		return err
	}

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
		From:     fromk,
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

	fmt.Printf("message sent, now waiting on cid: %s\n", smsg.Cid())

	mwait, err := api.StateWaitMsg(ctx, smsg.Cid(), build.MessageConfidence)
	if err != nil {
		return err
	}

	if mwait.Receipt.ExitCode != 0 {
		return fmt.Errorf("failed to add verifier: %d", mwait.Receipt.ExitCode)
	}

	return nil
}

func lotusVerifyAccount(fromAddr string, targetAddr string, allowanceStr string) error {
	from, err := address.NewFromString(fromAddr)
	if err != nil {
		return err
	}

	target, err := address.NewFromString(targetAddr)
	if err != nil {
		return err
	}

	allowance, err := types.BigFromString(allowanceStr)
	if err != nil {
		return err
	}

	params, err := actors.SerializeParams(&verifreg.AddVerifiedClientParams{Address: target, Allowance: allowance})
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
		From:     from,
		Method:   builtin.MethodsVerifiedRegistry.AddVerifiedClient,
		GasPrice: types.NewInt(1),
		GasLimit: 300000,
		Params:   params,
	}

	ctx := context.TODO()

	smsg, err := api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return err
	}

	fmt.Printf("message sent, now waiting on cid: %s\n", smsg.Cid())

	mwait, err := api.StateWaitMsg(ctx, smsg.Cid(), build.MessageConfidence)
	if err != nil {
		return err
	}

	if mwait.Receipt.ExitCode != 0 {
		return fmt.Errorf("failed to add verified client: %d", mwait.Receipt.ExitCode)
	}

	return nil
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

func lotusCheckAccountRemainingBytes(targetAddr string) (verifreg.DataCap, error) {
	caddr, err := address.NewFromString(targetAddr)
	if err != nil {
		return verifreg.DataCap{}, err
	}

	api, closer, err := GetFullNodeAPI()
	if err != nil {
		return verifreg.DataCap{}, err
	}
	defer closer()

	ctx := context.TODO()

	act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
	if err != nil {
		return verifreg.DataCap{}, err
	}

	apibs := apibstore.NewAPIBlockstore(api)
	cst := cbor.NewCborStore(apibs)

	var st verifreg.State
	if err := cst.Get(ctx, act.Head, &st); err != nil {
		return verifreg.DataCap{}, err
	}

	vh, err := hamt.LoadNode(ctx, cst, st.VerifiedClients)
	if err != nil {
		return verifreg.DataCap{}, err
	}

	var dcap verifreg.DataCap
	if err := vh.Find(ctx, string(caddr.Bytes()), &dcap); err != nil {
		return verifreg.DataCap{}, err
	}
	return dcap, nil
}

func lotusCheckVerifierRemainingBytes(targetAddr string) (verifreg.DataCap, error) {
	vaddr, err := address.NewFromString(targetAddr)
	if err != nil {
		return verifreg.DataCap{}, err
	}

	api, closer, err := GetFullNodeAPI()
	if err != nil {
		return verifreg.DataCap{}, err
	}
	defer closer()

	ctx := context.TODO()

	act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
	if err != nil {
		return verifreg.DataCap{}, err
	}

	apibs := apibstore.NewAPIBlockstore(api)
	cst := cbor.NewCborStore(apibs)

	var st verifreg.State
	if err := cst.Get(ctx, act.Head, &st); err != nil {
		return verifreg.DataCap{}, err
	}

	vh, err := hamt.LoadNode(ctx, cst, st.Verifiers)
	if err != nil {
		return verifreg.DataCap{}, err
	}

	var dcap verifreg.DataCap
	if err := vh.Find(ctx, string(vaddr.Bytes()), &dcap); err != nil {
		return verifreg.DataCap{}, err
	}
	return dcap, nil
}

func GetFullNodeAPI() (api.FullNode, jsonrpc.ClientCloser, error) {
	addr, headers, err := GetRawAPI(repo.FullNode)
	if err != nil {
		return nil, nil, err
	}

	return client.NewFullNodeRPC(addr, headers)
}

func GetRawAPI(t repo.RepoType) (string, http.Header, error) {
	ainfo, err := GetAPIInfo(repo.FullNode)
	if err != nil {
		return "", nil, xerrors.Errorf("could not get API info: %w", err)
	}

	addr, err := ainfo.DialArgs()
	if err != nil {
		return "", nil, xerrors.Errorf("could not get DialArgs: %w", err)
	}

	return addr, ainfo.AuthHeader(), nil
}

func flagForRepo(t repo.RepoType) string {
	switch t {
	case repo.FullNode:
		return "repo"
	case repo.StorageMiner:
		return "storagerepo"
	default:
		panic(fmt.Sprintf("Unknown repo type: %v", t))
	}
}

func envForRepo(t repo.RepoType) string {
	switch t {
	case repo.FullNode:
		return "FULLNODE_API_INFO"
	case repo.StorageMiner:
		return "STORAGE_API_INFO"
	default:
		panic(fmt.Sprintf("Unknown repo type: %v", t))
	}
}

func GetAPIInfo(t repo.RepoType) (lcli.APIInfo, error) {
	if env, ok := os.LookupEnv(envForRepo(t)); ok {
		sp := strings.SplitN(env, ":", 2)
		if len(sp) != 2 {
			fmt.Printf("invalid env(%s) value, missing token or address\n", envForRepo(t))
		} else {
			ma, err := multiaddr.NewMultiaddr(sp[1])
			if err != nil {
				return lcli.APIInfo{}, xerrors.Errorf("could not parse multiaddr from env(%s): %w", envForRepo(t), err)
			}
			return lcli.APIInfo{
				Addr:  ma,
				Token: []byte(sp[0]),
			}, nil
		}
	}

	// repoFlag := flagForRepo(t)

	// p, err := homedir.Expand(ctx.String(repoFlag))
	// if err != nil {
	// 	return lcli.APIInfo{}, xerrors.Errorf("cound not expand home dir (%s): %w", repoFlag, err)
	// }
	temp := os.TempDir()

	r, err := repo.NewFS(temp)
	if err != nil {
		return lcli.APIInfo{}, xerrors.Errorf("could not open repo at path: %w", err)
	}

	ma, err := r.APIEndpoint()
	if err != nil {
		return lcli.APIInfo{}, xerrors.Errorf("could not get api endpoint: %w", err)
	}

	token, err := r.APIToken()
	if err != nil {
		fmt.Printf("Couldn't load CLI token, capabilities may be limited: %v\n", err)
	}

	return lcli.APIInfo{
		Addr:  ma,
		Token: token,
	}, nil
}

// var verifRegListClientsCmd = &cli.Command{
// 	Name:  "list-clients",
// 	Usage: "list all verified clients",
// 	Action: func(cctx *cli.Context) error {
// 		api, closer, err := lcli.GetFullNodeAPI(cctx)
// 		if err != nil {
// 			return err
// 		}
// 		defer closer()
// 		ctx := lcli.ReqContext(cctx)

// 		act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
// 		if err != nil {
// 			return err
// 		}

// 		apibs := apibstore.NewAPIBlockstore(api)
// 		cst := cbor.NewCborStore(apibs)

// 		var st verifreg.State
// 		if err := cst.Get(ctx, act.Head, &st); err != nil {
// 			return err
// 		}

// 		vh, err := hamt.LoadNode(ctx, cst, st.VerifiedClients)
// 		if err != nil {
// 			return err
// 		}

// 		if err := vh.ForEach(ctx, func(k string, val interface{}) error {
// 			addr, err := address.NewFromBytes([]byte(k))
// 			if err != nil {
// 				return err
// 			}

// 			var dcap verifreg.DataCap

// 			if err := dcap.UnmarshalCBOR(bytes.NewReader(val.(*cbg.Deferred).Raw)); err != nil {
// 				return err
// 			}

// 			fmt.Printf("%s: %s\n", addr, dcap)

// 			return nil
// 		}); err != nil {
// 			return err
// 		}

// 		return nil
// 	},
// }

// var verifRegCheckClientCmd = &cli.Command{
// 	Name:  "check-client",
// 	Usage: "check verified client remaining bytes",
// 	Action: func(cctx *cli.Context) error {
// 		if !cctx.Args().Present() {
// 			return fmt.Errorf("must specify client address to check")
// 		}

// 		caddr, err := address.NewFromString(cctx.Args().First())
// 		if err != nil {
// 			return err
// 		}

// 		api, closer, err := lcli.GetFullNodeAPI(cctx)
// 		if err != nil {
// 			return err
// 		}
// 		defer closer()
// 		ctx := lcli.ReqContext(cctx)

// 		act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
// 		if err != nil {
// 			return err
// 		}

// 		apibs := apibstore.NewAPIBlockstore(api)
// 		cst := cbor.NewCborStore(apibs)

// 		var st verifreg.State
// 		if err := cst.Get(ctx, act.Head, &st); err != nil {
// 			return err
// 		}

// 		vh, err := hamt.LoadNode(ctx, cst, st.VerifiedClients)
// 		if err != nil {
// 			return err
// 		}

// 		var dcap verifreg.DataCap
// 		if err := vh.Find(ctx, string(caddr.Bytes()), &dcap); err != nil {
// 			return err
// 		}

// 		fmt.Println(dcap)

// 		return nil
// 	},
// }

// var verifRegCheckVerifierCmd = &cli.Command{
// 	Name:  "check-verifier",
// 	Usage: "check verifiers remaining bytes",
// 	Action: func(cctx *cli.Context) error {
// 		if !cctx.Args().Present() {
// 			return fmt.Errorf("must specify verifier address to check")
// 		}

// 		vaddr, err := address.NewFromString(cctx.Args().First())
// 		if err != nil {
// 			return err
// 		}

// 		api, closer, err := lcli.GetFullNodeAPI(cctx)
// 		if err != nil {
// 			return err
// 		}
// 		defer closer()
// 		ctx := lcli.ReqContext(cctx)

// 		act, err := api.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
// 		if err != nil {
// 			return err
// 		}

// 		apibs := apibstore.NewAPIBlockstore(api)
// 		cst := cbor.NewCborStore(apibs)

// 		var st verifreg.State
// 		if err := cst.Get(ctx, act.Head, &st); err != nil {
// 			return err
// 		}

// 		vh, err := hamt.LoadNode(ctx, cst, st.Verifiers)
// 		if err != nil {
// 			return err
// 		}

// 		var dcap verifreg.DataCap
// 		if err := vh.Find(ctx, string(vaddr.Bytes()), &dcap); err != nil {
// 			return err
// 		}

// 		fmt.Println(dcap)

// 		return nil
// 	},
// }
