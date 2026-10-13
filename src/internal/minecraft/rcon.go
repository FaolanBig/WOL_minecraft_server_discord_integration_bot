package minecraft

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	rconTimeout   = 5 * time.Second
	maxPacketSize = 1 << 20
)

type RCON struct {
	address  string
	password string
}

func NewRCON(address, password string) *RCON {
	return &RCON{address: address, password: password}
}

func (r *RCON) Players(ctx context.Context) (int, error) {
	response, err := r.Execute(ctx, "list")
	if err != nil {
		return 0, err
	}
	count, err := ParsePlayerCount(response)
	if err != nil {
		return 0, fmt.Errorf("parse Minecraft RCON player list: %w", err)
	}
	return count, nil
}

func (r *RCON) Stop(ctx context.Context) error {
	_, err := r.Execute(ctx, "stop")
	if err != nil {
		return fmt.Errorf("send Minecraft shutdown command: %w", err)
	}
	return nil
}

func (r *RCON) Execute(ctx context.Context, command string) (string, error) {
	dialer := net.Dialer{Timeout: rconTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", r.address)
	if err != nil {
		return "", fmt.Errorf("connect to Minecraft RCON: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(rconTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", fmt.Errorf("set Minecraft RCON deadline: %w", err)
	}

	const authID = int32(1)
	if err := writePacket(conn, authID, 3, r.password); err != nil {
		return "", fmt.Errorf("send Minecraft RCON authentication: %w", err)
	}
	auth, err := readPacket(conn)
	if err != nil {
		return "", fmt.Errorf("read Minecraft RCON authentication: %w", err)
	}
	if auth.id != authID || auth.packetType != 2 {
		return "", errors.New("Minecraft RCON authentication failed")
	}

	const commandID = int32(2)
	if err := writePacket(conn, commandID, 2, command); err != nil {
		return "", fmt.Errorf("send Minecraft RCON command: %w", err)
	}
	response, err := readPacket(conn)
	if err != nil {
		return "", fmt.Errorf("read Minecraft RCON response: %w", err)
	}
	if response.id != commandID || response.packetType != 0 {
		return "", fmt.Errorf("unexpected Minecraft RCON response packet (ID %d, type %d)", response.id, response.packetType)
	}
	return response.body, nil
}

type packet struct {
	id         int32
	packetType int32
	body       string
}

func writePacket(w io.Writer, id, packetType int32, body string) error {
	length := 4 + 4 + len(body) + 2
	data := make([]byte, 4+length)
	binary.LittleEndian.PutUint32(data[:4], uint32(length))
	binary.LittleEndian.PutUint32(data[4:8], uint32(id))
	binary.LittleEndian.PutUint32(data[8:12], uint32(packetType))
	copy(data[12:], body)
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func readPacket(r io.Reader) (packet, error) {
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return packet{}, err
	}
	length := binary.LittleEndian.Uint32(size[:])
	if length < 10 || length > maxPacketSize {
		return packet{}, fmt.Errorf("invalid Minecraft RCON packet size %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return packet{}, err
	}
	if data[len(data)-1] != 0 || data[len(data)-2] != 0 {
		return packet{}, errors.New("malformed Minecraft RCON packet terminator")
	}
	return packet{
		id:         int32(binary.LittleEndian.Uint32(data[:4])),
		packetType: int32(binary.LittleEndian.Uint32(data[4:8])),
		body:       string(data[8 : len(data)-2]),
	}, nil
}
