package main

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
)

func lotusListMiners() ([]address.Address, error) {
	api, closer, err := lotusGetFullNodeAPI(context.TODO())
	if err != nil {
		return nil, err
	}
	defer closer()

	tipset, err := api.ChainHead(context.TODO())
	if err != nil {
		return nil, err
	}

	return api.StateListMiners(context.TODO(), tipset.Key())
}

func lotusGetYesterdayTipsetKey() types.TipSetKey {
	api, closer, err := lotusGetFullNodeAPI(context.TODO())
	if err != nil {
		return types.EmptyTSK
	}
	defer closer()
	chainHead, _ := api.ChainHead(context.TODO())
	tipset, _ := api.ChainGetTipSetByHeight(context.TODO(), chainHead.Height()-2880, types.EmptyTSK)

	return tipset.Key()
}

func runTest() {
	addrs, err := lotusListMiners()
	if err != nil {
		panic(err)
	}

	fmt.Println("Miners:")
	for _, addr := range addrs {
		fmt.Println("  -", addr.String())
	}

	add1, _ := address.NewFromString("t01285")
	add2, _ := address.NewFromString("t01766")
	add3, _ := address.NewFromString("t01783")

	testMinerAmountToSend(add1)
	testMinerAmountToSend(add2)
	testMinerAmountToSend(add3)
}
