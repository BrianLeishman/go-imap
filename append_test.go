package imap

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
)

// appendMockServer is a mock IMAP server that supports the APPEND continuation protocol.
type appendMockServer struct {
	listener     net.Listener
	address      string
	authAttempts int32
	validUser    string
	validPass    string
}

func newAppendMockServer(validUser, validPass string) (*appendMockServer, error) {
	cert, err := generateSelfSignedCertificate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificate: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS listener: %v", err)
	}

	server := &appendMockServer{
		listener:  listener,
		address:   listener.Addr().String(),
		validUser: validUser,
		validPass: validPass,
	}

	go server.serve()
	return server, nil
}

func (s *appendMockServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConnection(conn)
	}
}

func (s *appendMockServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	writer.WriteString("* OK IMAP4rev1 Mock Server Ready\r\n")
	writer.Flush()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		tag := parts[0]
		command := strings.ToUpper(parts[1])

		switch command {
		case "LOGIN":
			atomic.AddInt32(&s.authAttempts, 1)
			if len(parts) >= 4 {
				username := strings.Trim(parts[2], `"`)
				password := strings.Trim(parts[3], `"`)
				if username == s.validUser && password == s.validPass {
					fmt.Fprintf(writer, "%s OK LOGIN completed\r\n", tag)
				} else {
					fmt.Fprintf(writer, "%s NO [AUTHENTICATIONFAILED] Authentication failed\r\n", tag)
				}
			} else {
				fmt.Fprintf(writer, "%s BAD Invalid LOGIN command\r\n", tag)
			}

		case "APPEND":
			// Handle the APPEND continuation protocol
			// Parse literal size from {NNN} at end of line
			literalSize := 0
			if idx := strings.LastIndex(line, "{"); idx != -1 {
				endIdx := strings.LastIndex(line, "}")
				if endIdx > idx {
					sizeStr := line[idx+1 : endIdx]
					sizeStr = strings.TrimSuffix(sizeStr, "+") // handle LITERAL+
					literalSize, _ = strconv.Atoi(sizeStr)
				}
			}

			// Send continuation
			writer.WriteString("+ Ready for literal data\r\n")
			writer.Flush()

			// Read the literal bytes
			if literalSize > 0 {
				buf := make([]byte, literalSize)
				_, err = io.ReadFull(reader, buf)
				if err != nil {
					return
				}
			}

			// Read the trailing CRLF
			_, err = reader.ReadString('\n')
			if err != nil {
				return
			}

			fmt.Fprintf(writer, "%s OK [APPENDUID 1 100] APPEND completed\r\n", tag)

		case "SELECT":
			writer.WriteString("* 0 EXISTS\r\n")
			writer.WriteString("* 0 RECENT\r\n")
			fmt.Fprintf(writer, "%s OK SELECT completed\r\n", tag)

		case "EXAMINE":
			writer.WriteString("* 0 EXISTS\r\n")
			writer.WriteString("* 0 RECENT\r\n")
			fmt.Fprintf(writer, "%s OK EXAMINE completed\r\n", tag)

		case "LOGOUT":
			writer.WriteString("* BYE Server logging out\r\n")
			fmt.Fprintf(writer, "%s OK LOGOUT completed\r\n", tag)
			writer.Flush()
			return

		default:
			fmt.Fprintf(writer, "%s OK %s completed\r\n", tag, command)
		}

		writer.Flush()
	}
}

func (s *appendMockServer) Close() {
	s.listener.Close()
}

func (s *appendMockServer) GetHost() string {
	host, _, _ := net.SplitHostPort(s.address)
	return host
}

func (s *appendMockServer) GetPort() int {
	_, portStr, _ := net.SplitHostPort(s.address)
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}
