package minecraft

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"
)

func TestWriteAndReadPacket(t *testing.T) {
	var wire bytes.Buffer
	if err := writePacket(&wire, 42, 2, "list"); err != nil {
		t.Fatal(err)
	}
	data := wire.Bytes()

	if got, want := binary.LittleEndian.Uint32(data[:4]), uint32(14); got != want {
		t.Fatalf("packet length = %d, want %d", got, want)
	}
	if got := string(data[12 : len(data)-2]); got != "list" {
		t.Fatalf("packet body = %q, want %q", got, "list")
	}

	got, err := readPacket(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got.id != 42 || got.packetType != 2 || got.body != "list" {
		t.Errorf("decoded packet = %+v, want ID 42, type 2, body list", got)
	}
}

func TestReadPacketRejectsInvalidFrames(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "short header", data: []byte{1, 2}},
		{name: "undersized body", data: []byte{9, 0, 0, 0}},
		{name: "oversized body", data: []byte{1, 0, 16, 0}},
		{name: "invalid terminator", data: appendPacketFields(1, 2, "bad", 13, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readPacket(bytes.NewReader(test.data)); err == nil {
				t.Fatal("readPacket() accepted an invalid frame")
			}
		})
	}
}

func TestRCONExecute(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(time.Second))

		auth, err := readPacket(conn)
		if err != nil {
			serverErr <- err
			return
		}
		if auth.packetType != 3 || auth.body != "secret" {
			serverErr <- errors.New("unexpected RCON authentication request")
			return
		}
		if err := writePacket(conn, auth.id, 2, ""); err != nil {
			serverErr <- err
			return
		}

		command, err := readPacket(conn)
		if err != nil {
			serverErr <- err
			return
		}
		if command.packetType != 2 || command.body != "list" {
			serverErr <- errors.New("unexpected RCON command request")
			return
		}
		serverErr <- writePacket(conn, command.id, 0, "There are 0 of a max of 20 players online:")
	}()

	rcon := NewRCON(listener.Addr().String(), "secret")
	response, err := rcon.Execute(t.Context(), "list")
	if err != nil {
		t.Fatal(err)
	}
	if response != "There are 0 of a max of 20 players online:" {
		t.Fatalf("RCON response = %q", response)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestRCONExecuteRejectsInvalidAuthPacketType(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		request, err := readPacket(conn)
		if err == nil {
			_ = writePacket(conn, request.id, 0, "")
		}
	}()

	rcon := NewRCON(listener.Addr().String(), "secret")
	if _, err := rcon.Execute(t.Context(), "list"); err == nil {
		t.Fatal("Execute() accepted an authentication packet with an invalid type")
	}
}

func appendPacketFields(id, packetType int32, body string, length uint32, terminator byte) []byte {
	data := make([]byte, 4+length)
	binary.LittleEndian.PutUint32(data[:4], length)
	binary.LittleEndian.PutUint32(data[4:8], uint32(id))
	binary.LittleEndian.PutUint32(data[8:12], uint32(packetType))
	copy(data[12:], body)
	data[len(data)-1] = terminator
	return data
}
