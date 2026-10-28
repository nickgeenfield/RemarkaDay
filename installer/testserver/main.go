// Minimal fake tablet SSH server for testing the installer (password auth, exec, sftp).
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func main() {
	addr, pass, noSftp := os.Args[1], os.Args[2], len(os.Args) > 3 && os.Args[3] == "nosftp"
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
		if c.User() == "root" && string(p) == pass {
			return nil, nil
		}
		return nil, fmt.Errorf("denied")
	}}
	cfg.AddHostKey(signer)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	for {
		nc, err := l.Accept()
		if err != nil {
			continue
		}
		go func() {
			_, chans, reqs, err := ssh.NewServerConn(nc, cfg)
			if err != nil {
				return
			}
			go ssh.DiscardRequests(reqs)
			for nch := range chans {
				ch, creqs, _ := nch.Accept()
				go func() {
					defer ch.Close()
					for r := range creqs {
						switch r.Type {
						case "exec":
							r.Reply(true, nil)
							l := binary.BigEndian.Uint32(r.Payload[:4])
							cmd := string(r.Payload[4 : 4+l])
							log.Println("EXEC:", cmd)
							code := 0
							if strings.Contains(cmd, "backend/entry 2000-01-01") || strings.Contains(cmd, "backend/entry google-sync") {
								// Simulate aarch64 entry: accept if the binary is a static aarch64 ELF.
								idx := strings.Index(cmd, "/home/root/xovi/exthome/appload/remarkable-calendar/backend/entry")
								p := "/home/root/xovi/exthome/appload/remarkable-calendar/backend/entry"
								if idx >= 0 {
									p = cmd[idx : idx+len(p)]
								}
								b, _ := os.ReadFile(p)
								st, _ := os.Stat(p)
								if len(b) > 20 && bytes.Equal(b[:4], []byte{0x7f, 'E', 'L', 'F'}) && binary.LittleEndian.Uint16(b[18:]) == 183 && st.Mode()&0111 != 0 {
									if strings.Contains(cmd, "google-sync") {
										io.WriteString(ch, "synced (simulated)\n")
									} else {
										io.WriteString(ch, "{\"ok\":true}\n")
									}
								} else {
									io.WriteString(ch.Stderr(), "selftest FAILED\n")
									code = 1
								}
							} else {
								c := exec.Command("/bin/sh", "-c", cmd)
								c.Stdin, c.Stdout, c.Stderr = ch, ch, ch.Stderr()
								if err := c.Run(); err != nil {
									if ee, ok := err.(*exec.ExitError); ok {
										code = ee.ExitCode()
									} else {
										code = 127
									}
								}
							}
							ch.CloseWrite()
							ch.SendRequest("exit-status", false, ssh.Marshal(struct{ C uint32 }{uint32(code)}))
							return
						case "subsystem":
							if noSftp {
								r.Reply(false, nil)
								continue
							}
							r.Reply(true, nil)
							s, _ := sftp.NewServer(ch)
							s.Serve()
							return
						default:
							r.Reply(false, nil)
						}
					}
				}()
			}
		}()
	}
}
