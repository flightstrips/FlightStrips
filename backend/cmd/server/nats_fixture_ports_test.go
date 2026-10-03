package main

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFixtureAddressReservationsRemainUniqueInParallel(t *testing.T) {
	const count = 96
	addresses := make(chan string, count)
	var workers sync.WaitGroup
	for range count {
		workers.Add(1)
		go func() {
			defer workers.Done()
			addresses <- entrypointAddress(t)
		}()
	}
	workers.Wait()
	close(addresses)
	seen := make(map[string]bool, count)
	for address := range addresses {
		require.False(t, seen[address], "parallel fixtures reserved the same address")
		seen[address] = true
	}
	require.Len(t, seen, count)
}
