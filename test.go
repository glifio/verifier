package main

import (
	"context"

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
