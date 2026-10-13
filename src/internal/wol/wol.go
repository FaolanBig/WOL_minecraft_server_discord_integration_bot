package wol

import (
	"context"
	"fmt"
	"net"
	"syscall"
)

type Sender struct {
	mac       net.HardwareAddr
	broadcast net.IP
	port      int
}

func New(mac net.HardwareAddr, broadcast net.IP, port int) *Sender {
	return &Sender{mac: mac, broadcast: broadcast, port: port}
}

func (s *Sender) Wake(ctx context.Context) error {
	packet := magicPacket(s.mac)

	if err := ctx.Err(); err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return fmt.Errorf("open Wake-on-LAN UDP socket: %w", err)
	}
	defer conn.Close()

	rawConn, err := conn.SyscallConn()
	if err != nil {
		return fmt.Errorf("access Wake-on-LAN UDP socket: %w", err)
	}
	var socketErr error
	if err := rawConn.Control(func(fd uintptr) {
		socketErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	}); err != nil {
		return fmt.Errorf("configure Wake-on-LAN UDP socket: %w", err)
	}
	if socketErr != nil {
		return fmt.Errorf("enable UDP broadcast: %w", socketErr)
	}
	destination := &net.UDPAddr{IP: s.broadcast, Port: s.port}
	if _, err := conn.WriteToUDP(packet, destination); err != nil {
		return fmt.Errorf("send Wake-on-LAN packet: %w", err)
	}
	return nil
}

func magicPacket(mac net.HardwareAddr) []byte {
	packet := make([]byte, 6+16*len(mac))
	for i := 0; i < 6; i++ {
		packet[i] = 0xff
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*len(mac):], mac)
	}
	return packet
}
