package main

import (
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/glifio/go-logger"
)

var blocklist = make(map[address.Address]bool)

// cache the blocklist as a map in memory with faster lookups than reading the file everytime
func initBlockListCache() error {
	if len(env.BlockedAddresses) == 0 {
		return nil
	}

	for _, e := range strings.Split(env.BlockedAddresses, ",") {
		logger.Debugf("Adding %v to blocklist.", e)
		targetAddr, err := address.NewFromString(e)
		if err != nil {
			return err
		}
		blocklist[targetAddr] = true
	}

	return nil
}

func isAddressBlocked(address address.Address) bool {
	blocked := blocklist[address]
	if blocked {
		logger.Debugf("Blocked address: %v", address.String())
	}
	return blocked
}
