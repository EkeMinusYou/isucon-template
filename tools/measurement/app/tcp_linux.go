//go:build linux

package app

import (
	"golang.org/x/sys/unix"
	"net"
	"syscall"
)

func sampleTCP(c net.Conn) *TCPObservation {
	socket, ok := unwrapTCP(c).(syscall.Conn)
	if !ok {
		return nil
	}
	raw, err := socket.SyscallConn()
	if err != nil {
		return nil
	}
	var result *TCPObservation
	if err = raw.Control(func(fd uintptr) {
		info, e := unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
		if e == nil {
			result = &TCPObservation{RTTMicros: info.Rtt, RTTVarianceMicros: info.Rttvar, TotalRetransmits: info.Total_retrans}
		}
	}); err != nil {
		return nil
	}
	return result
}
