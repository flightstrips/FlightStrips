//go:build !task22fault

// Package faultgate provides command-specific barriers only in local fault builds.
package faultgate

func Reach(point, commandID string, sequence uint64)   {}
func State(point string, data []byte, sequence uint64) {}
