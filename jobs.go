package main

import (
	"context"
	"time"
  "strconv"
	"github.com/ipfs/go-cid"
)

func sendSlackMessage(message string) {
	sendSlackNotification("https://errors.glif.io/verifier-cron-job-failed", message)
	return
}

func reconcileVerifierMessages() {
	users, err := getLockedUsers(UserLock_Verifier)
	sendSlackMessage("RUNNING VERIFIER JOB. Users: "+strconv.Itoa(len(users)))
	if err != nil {
		sendSlackMessage(err.Error()+"error getting locked users")
		return
	}

	for _, user := range users {
		cid, err := cid.Decode(user.MostRecentDataCapCid)
		if err != nil {
			sendSlackMessage(err.Error())
			sendSlackMessage("Helper to try and debug cid.Decode call: "+ cid.String())
			return
		}
		mLookup, err := lotusSearchMessageResult(context.TODO(), cid)
		if err != nil {
			sendSlackMessage(err.Error())
			sendSlackMessage("Helper to try and debug lotusSearchMsgResult call: "+ cid.String())
			return
		}

		finished := mLookup != nil
		confirmed := mLookup.Receipt.ExitCode.IsSuccess()
		if finished && confirmed {
			user.MostRecentAllocation = time.Now()
			user.Locked_Verifier = false
			err = saveUser(user)
			if err != nil {
				sendSlackMessage(err.Error())
				return
			}
		} else if finished {
			sendSlackMessage("TRANSACTION FAILED: "+mLookup.Receipt.ExitCode.Error())
			return
		}
	}
}

func reconcileFaucetMessages() {
	sendSlackMessage("RUNNING FAUCET JOB")
	users, err := getLockedUsers(UserLock_Faucet)
	if err != nil {
		sendSlackMessage(err.Error())
		return
	}

	for _, user := range users {
		cid, err := cid.Decode(user.MostRecentFaucetGrantCid)
		if err != nil {
			sendSlackMessage(err.Error())
			return
		}
		mLookup, err := lotusSearchMessageResult(context.TODO(), cid)
		if err != nil {
			sendSlackMessage(err.Error())
			return
		}

		finished := mLookup != nil
		confirmed := mLookup.Receipt.ExitCode.IsSuccess()
		if finished && confirmed {
			user.ReceivedFaucetGrant = true
			user.Locked_Faucet = false
			err = saveUser(user)
			if err != nil {
				sendSlackMessage(err.Error())
				return
			}
		} else if finished {
			sendSlackMessage("TRANSACTION FAILED: "+mLookup.Receipt.ExitCode.Error())
			return
		}
	}
}