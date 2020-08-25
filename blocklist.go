package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/filecoin-project/go-address"
)

var blocklist = make(map[address.Address]bool)

// cache the blocklist as a map in memory with faster lookups than reading the file everytime
func initBlockListCache() error {
	f, err := os.Open(env.PathToBlocklistTxtFile)

	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		fmt.Println("Adding " + scanner.Text() + " to blocklist.")
		targetAddr, err := address.NewFromString(scanner.Text())
		if err != nil {
			return err
		}
		blocklist[targetAddr] = true
	}

	if err := scanner.Err(); err != nil {
		return err
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
