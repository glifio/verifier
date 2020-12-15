package main

import (
	"fmt"
	"strings"

	"github.com/filecoin-project/go-address"
)

var blocklist = make(map[address.Address]bool)

// cache the blocklist as a map in memory with faster lookups than reading the file everytime
func initBlockListCache() error {
	if len(env.BlockedAddresses) == 0 {
		return nil
	}

	for _, e := range strings.Split(env.BlockedAddresses, ",") {
		fmt.Println("Adding " + e + " to blocklist.")
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
		fmt.Println("Blocked address: ", address.String())
	}
	return blocked
}
