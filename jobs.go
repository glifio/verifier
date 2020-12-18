package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
)

func reconcileVerifierMessages() {
	users, err := getLockedUsers(UserLock_Verifier)
	if err != nil {
		fmt.Println("ERROR FOR NR", err.Error())
		return
	}

	for _, user := range users {
		cid, err := cid.Decode(user.MostRecentDataCapCid)
		if err != nil {
			fmt.Println("ERROR FOR NR", err.Error())
			return
		}
		mLookup, err := lotusSearchMessageResult(context.TODO(), cid)
		if err != nil {
			fmt.Println("ERROR FOR NR", err.Error())
			return
		}

		finished := mLookup != nil
		confirmed := mLookup.Receipt.ExitCode.IsSuccess()
		if finished && confirmed {
			user.MostRecentAllocation = time.Now()
			user.Locked_Verifier = false
			err = saveUser(user)
			if err != nil {
				fmt.Println("ERR FOR NR", err)
				return
			}
		} else if finished {
			fmt.Println("TRANSACTION FAILED ERR FOR NR", mLookup.Receipt.ExitCode.Error(), mLookup.Receipt.ExitCode.Error())
			return
		}
	}
}

func reconcileFaucetMessages() {
	users, err := getLockedUsers(UserLock_Faucet)
	if err != nil {
		fmt.Println("ERROR FOR NR", err.Error())
		return
	}

	for _, user := range users {
		cid, err := cid.Decode(user.MostRecentFaucetGrantCid)
		if err != nil {
			fmt.Println("ERROR FOR NR", err.Error())
			return
		}
		mLookup, err := lotusSearchMessageResult(context.TODO(), cid)
		if err != nil {
			fmt.Println("ERROR FOR NR", err.Error())
			return
		}

		finished := mLookup != nil
		confirmed := mLookup.Receipt.ExitCode.IsSuccess()
		if finished && confirmed {
			user.ReceivedFaucetGrant = true
			user.Locked_Faucet = false
			err = saveUser(user)
			if err != nil {
				fmt.Println("ERR FOR NR", err)
				return
			}
		} else if finished {
			fmt.Println("TRANSACTION FAILED ERR FOR NR", mLookup.Receipt.ExitCode.Error(), mLookup.Receipt.ExitCode.Error())
			return
		}
	}
}