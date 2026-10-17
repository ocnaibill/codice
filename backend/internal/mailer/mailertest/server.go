// Package mailertest is an SMTP server of mentira for the tests of what sends mail: it takes a message, keeps it, and answers as a server does.
package mailertest

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
)

// Server keeps what it was sent.
type Server struct {
	Host string
	Port int

	mu       sync.Mutex
	messages []string
	rcpts    []string
	// Refuse makes it answer 550 to the recipient.
	Refuse bool
}

// Start listens on a free port of this machine, without protection and without login.
func Start(t *testing.T) *Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	addr := ln.Addr().(*net.TCPAddr)
	s := &Server{Host: "127.0.0.1", Port: addr.Port}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

// Messages are the messages taken, as they came.
func (s *Server) Messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.messages...)
}

// Recipients are the addresses the messages were for.
func (s *Server) Recipients() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.rcpts...)
}

func (s *Server) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	say := func(line string) { conn.Write([]byte(line + "\r\n")) }
	say("220 mentira ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		upper := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			say("250 mentira")
		case strings.HasPrefix(upper, "RCPT TO"):
			if s.Refuse {
				say("550 recipient refused")
				continue
			}
			s.mu.Lock()
			s.rcpts = append(s.rcpts, strings.TrimSpace(line[len("RCPT TO:"):]))
			s.mu.Unlock()
			say("250 ok")
		case upper == "DATA":
			say("354 go on")
			var sb strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				sb.WriteString(l)
			}
			s.mu.Lock()
			s.messages = append(s.messages, sb.String())
			s.mu.Unlock()
			say("250 queued")
		case upper == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}
