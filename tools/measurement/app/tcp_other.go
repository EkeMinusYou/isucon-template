//go:build !linux

package app

import "net"

func sampleTCP(net.Conn) *TCPObservation { return nil }
