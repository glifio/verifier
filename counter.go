package main

var count uint

func initCounter() {
	count = 0
}

func reachedCounter() bool {
	if env.MaxTotalAllocations == 0 {
		return false
	}
	return count >= env.MaxTotalAllocations
}

func incrementCounter() {
	count++
}

func resetCounter() {
	count = 0
}