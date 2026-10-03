package natsresources

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func TestVerifyMetadataRequestsHonorCallerDeadline(t *testing.T) {
	for _, blocked := range []string{"account", "stream"} {
		t.Run(blocked, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			t.Cleanup(func() { listener.Close() })
			done := make(chan struct{})
			go func() {
				defer close(done)
				c, err := listener.Accept()
				if err != nil {
					return
				}
				defer c.Close()
				fmt.Fprint(c, "INFO {\"server_id\":\"unit\",\"version\":\"2.15.0\",\"proto\":1,\"max_payload\":1048576}\r\n")
				reader := bufio.NewReader(c)
				sid := ""
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					fields := strings.Fields(line)
					if len(fields) == 0 {
						continue
					}
					switch fields[0] {
					case "PING":
						fmt.Fprint(c, "PONG\r\n")
					case "SUB":
						sid = fields[len(fields)-1]
					case "PUB":
						size, err := strconv.Atoi(fields[len(fields)-1])
						if err != nil {
							return
						}
						if _, err = io.CopyN(io.Discard, reader, int64(size+2)); err != nil {
							return
						}
						if blocked == "stream" && fields[1] == "$JS.API.INFO" && len(fields) == 4 {
							fmt.Fprintf(c, "MSG %s %s 2\r\n{}\r\n", fields[2], sid)
						}
					}
				}
			}()
			url := "nats://" + listener.Addr().String()
			nc, err := nats.Connect(url, nats.NoReconnect(), nats.Timeout(time.Second))
			require.NoError(t, err)
			t.Cleanup(func() { nc.Close(); <-done })
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer cancel()
			started := time.Now()
			err = Verify(ctx, nc, Config{URLs: []string{url}, Names: RequiredNames, ConnectTimeout: time.Second, RequestTimeout: time.Second})
			require.ErrorIs(t, err, context.DeadlineExceeded,
				"both account and stream requests must use the supplied context instead of their independent MaxWait")
			require.Less(t, time.Since(started), 500*time.Millisecond)
			if blocked == "account" {
				require.ErrorContains(t, err, "JetStream account")
			} else {
				require.ErrorContains(t, err, "FS_STATE: inspect")
			}
		})
	}
}
