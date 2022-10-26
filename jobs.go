package main

import (
	"context"
	"time"

	"github.com/glifio/go-logger"
	"github.com/ipfs/go-cid"
)

func reconcileVerifierMessages() {
	users, err := getLockedUsers(UserLock_Verifier)
	if err != nil {
		logger.Errorf("ERROR GETTING LOCKED USERS: %v", err)
		return
	}

	for _, user := range users {
		cid, err := cid.Decode(user.MostRecentDataCapCid)
		if err != nil {
			logger.Errorf("ERROR DECODING DATACAP CID: %v", err)
			continue
		}
		mLookup, err := lotusSearchMessageResult(context.TODO(), cid)
		if err != nil {
			logger.Errorf("ERROR SEARCHING LOTUS MESSAGE: %v", err)
			continue
		}

		finished := mLookup != nil
		confirmed := mLookup.Receipt.ExitCode.IsSuccess()
		if finished && confirmed {
			user.MostRecentAllocation = time.Now()
			user.Locked_Verifier = false
			err = saveUser(user)
			if err != nil {
				logger.Errorf("ERROR SAVING USER: %v", err)
				continue
			}
		} else if finished {
			logger.Errorf("TRANSACTION FAILED: %v", mLookup.Receipt.ExitCode.Error())
			continue
		}
	}
}

func reconcileFaucetMessages() {
	users, err := getLockedUsers(UserLock_Faucet)
	if err != nil {
		logger.Errorf("ERROR GETTING LOCKED USERS: %v", err)
		return
	}

	for _, user := range users {
		cid, err := cid.Decode(user.MostRecentFaucetGrantCid)
		if err != nil {
			logger.Errorf("ERROR DECODING FAUCET GRANT CID: %v", err)
			return
		}
		mLookup, err := lotusSearchMessageResult(context.TODO(), cid)
		if err != nil {
			logger.Errorf("ERROR SEARCHING LOTUS MESSAGE: %v", err)
			return
		}

		finished := mLookup != nil
		confirmed := mLookup.Receipt.ExitCode.IsSuccess()
		if finished && confirmed {
			user.ReceivedFaucetGrant = true
			user.Locked_Faucet = false
			err = saveUser(user)
			if err != nil {
				logger.Errorf("ERROR SAVING USER: %v", err)
				return
			}
		} else if finished {
			logger.Errorf("TRANSACTION FAILED: %v", mLookup.Receipt.ExitCode.Error())
			return
		}
	}
}
