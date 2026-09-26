// Local-only SSH integration fixture. It never executes a received shell command.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func main() {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(key)
	config := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if meta.User() == "phone-test" && string(password) == "local-fixture-password" {
			return nil, nil
		}
		return nil, fmt.Errorf("fixture authentication failed")
	}}
	config.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:22882")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("iOS SSH fixture listening on 127.0.0.1:22882")
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go serve(conn, config)
	}
}
func serve(conn net.Conn, config *ssh.ServerConfig) {
	server, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		conn.Close()
		return
	}
	defer server.Close()
	go ssh.DiscardRequests(requests)
	for channel := range channels {
		if channel.ChannelType() != "session" {
			channel.Reject(ssh.UnknownChannelType, "session only")
			continue
		}
		ch, reqs, err := channel.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer ch.Close()
			for req := range reqs {
				switch req.Type {
				case "pty-req", "window-change":
					req.Reply(true, nil)
				case "shell":
					req.Reply(true, nil)
					io.WriteString(ch, "\r\nphone-test@fixture:~$ ")
					go func() {
						for r := range reqs {
							r.Reply(r.Type == "window-change", nil)
						}
					}()
					buffer := make([]byte, 4096)
					for {
						n, err := ch.Read(buffer)
						if n > 0 {
							if bytes.Contains(buffer[:n], []byte{3}) {
								io.WriteString(ch, "^C\r\nphone-test@fixture:~$ ")
							} else {
								io.WriteString(ch, "PHONE_TERMINAL_OK\r\nphone-test@fixture:~$ ")
							}
						}
						if err != nil {
							return
						}
					}
				case "exec":
					var payload struct{ Command string }
					if ssh.Unmarshal(req.Payload, &payload) != nil {
						return
					}
					req.Reply(true, nil)
					marker := regexp.MustCompile(`__WRD_EXIT_[A-Fa-f0-9]+__`).FindString(payload.Command)
					if marker == "" {
						io.WriteString(ch, "unsupported fixture command")
						exit(ch, 2)
						return
					}
					if strings.Contains(payload.Command, "fixture-timeout") {
						time.Sleep(4 * time.Second)
						return
					}
					if strings.Contains(payload.Command, "fixture-no-marker") {
						io.WriteString(ch, "connection closed before result\n")
						return
					}
					if strings.Contains(payload.Command, "fixture-nonzero") {
						fmt.Fprintf(ch, "EXPECTED_FAILURE\n%s7\n", marker)
					} else {
						fmt.Fprintf(ch, "PHONE_EXEC_OK\n%s0\n", marker)
					}
					exit(ch, 0)
					return
				default:
					req.Reply(false, nil)
				}
			}
		}()
	}
}
func exit(ch ssh.Channel, status uint32) {
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, status)
	ch.SendRequest("exit-status", false, data)
}
