package wol

import (
	"bytes"
	"net"
	"testing"
)

func TestMagicPacket(t *testing.T) {
	mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	packet := magicPacket(mac)

	if len(packet) != 102 {
		t.Fatalf("magic packet length = %d, want 102", len(packet))
	}
	if !bytes.Equal(packet[:6], bytes.Repeat([]byte{0xff}, 6)) {
		t.Fatal("magic packet does not start with six 0xff bytes")
	}
	for i := 0; i < 16; i++ {
		start := 6 + i*len(mac)
		if !bytes.Equal(packet[start:start+len(mac)], mac) {
			t.Fatalf("MAC repetition %d = %x, want %x", i, packet[start:start+len(mac)], mac)
		}
	}
}
