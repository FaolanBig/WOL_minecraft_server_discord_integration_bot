package host

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const sshTimeout = 5 * time.Second

type SSH struct {
	address string
	config  *ssh.ClientConfig
}

func NewSSH(address, user, privateKeyFile, knownHostsFile string) (*SSH, error) {
	key, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read host SSH key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse host SSH key: %w", err)
	}
	hostKeyCallback, err := knownhosts.New(knownHostsFile)
	if err != nil {
		return nil, fmt.Errorf("load host SSH known_hosts: %w", err)
	}
	return &SSH{
		address: address,
		config: &ssh.ClientConfig{
			User:            user,
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: hostKeyCallback,
			Timeout:         sshTimeout,
		},
	}, nil
}

func (s *SSH) StartMinecraft(ctx context.Context) error {
	return s.run(ctx, "start-minecraft")
}

func (s *SSH) ScheduleShutdown(ctx context.Context) error {
	return s.run(ctx, "schedule-shutdown")
}

func (s *SSH) CancelShutdown(ctx context.Context) error {
	return s.run(ctx, "cancel-shutdown")
}

func (s *SSH) run(ctx context.Context, command string) error {
	dialer := net.Dialer{Timeout: sshTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("connect to Minecraft host over SSH: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(sshTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set host SSH deadline: %w", err)
	}

	connection, channels, requests, err := ssh.NewClientConn(conn, s.address, s.config)
	if err != nil {
		return fmt.Errorf("establish host SSH session: %w", err)
	}
	client := ssh.NewClient(connection, channels, requests)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("open host SSH command session: %w", err)
	}
	defer session.Close()
	if err := session.Run(command); err != nil {
		return fmt.Errorf("run host control command %q: %w", command, err)
	}
	return nil
}
