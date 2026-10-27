package docker

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/docker/docker/client"
)

type rconAddressTransport struct {
	fakeDaemon
	body string
}

func (d *rconAddressTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasSuffix(req.URL.Path, "/containers/server/json") {
		return d.response(http.StatusOK, d.body)
	}
	return d.fakeDaemon.RoundTrip(req)
}

func TestRCONAddresses(t *testing.T) {
	for _, test := range []struct {
		name, body string
		want       []string
	}{
		{"published port", `{"NetworkSettings":{"Ports":{"25575/tcp":[{"HostIp":"0.0.0.0","HostPort":"25590"}],"25565/tcp":[{"HostIp":"0.0.0.0","HostPort":"25565"}]},"Networks":{"mcm-network":{"IPAddress":"172.20.0.2"}}}}`, []string{"remote-host:25590", "172.20.0.2:25575"}},
		{"private network", `{"NetworkSettings":{"Networks":{"mcm-network":{"IPAddress":"172.20.0.2"},"other":{"IPAddress":"172.21.0.2"}}}}`, []string{"172.20.0.2:25575", "172.21.0.2:25575"}},
		{"host network", `{"HostConfig":{"NetworkMode":"host"}}`, []string{"remote-host:25575"}},
		{"missing bindings", `{"NetworkSettings":{"Ports":{"25565/tcp":[{"HostIp":"0.0.0.0","HostPort":"25565"}]}}}`, []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			daemon := &rconAddressTransport{body: test.body}
			cli, err := client.NewClientWithOpts(client.WithHost("tcp://remote-host"), client.WithVersion("1.40"), client.WithHTTPClient(&http.Client{Transport: daemon}))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			manager := &Manager{client: cli, host: "tcp://remote-host:2375"}
			addresses, err := manager.RCONAddresses(context.Background(), "server", 25575)
			if err != nil || !slices.Equal(addresses, test.want) {
				t.Fatalf("addresses=%v want=%v err=%v", addresses, test.want, err)
			}
			if _, err := manager.RCONAddresses(context.Background(), "server", 70000); err == nil {
				t.Fatal("invalid port accepted")
			}
		})
	}
}
