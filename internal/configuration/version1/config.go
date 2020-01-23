package version1

import (
	"net"
)

// TCPServerConf describes an NGINX TCPServer
type TCPServerConf struct {
	ListenPort int
	Upstream   Upstream
}

// Upstream describes an NGINX upstream.
type Upstream struct {
	Name            string
	UpstreamServers []UpstreamServer
	// Additional attributes might be added here.
	/*
		StickyCookie     string
		LBMethod         string
		Queue            int64
		QueueTimeout     int64
		UpstreamZoneSize string
	*/
}

// UpstreamServer describes a server in an NGINX upstream.
type UpstreamServer struct {
	Address net.TCPAddr
	// Additional attributes to be added here.
	/*
		MaxFails    int
		MaxConns    int
		FailTimeout string
		SlowStart   string
		Resolve     bool
	*/
}

// NewDefaultTCPServerUpstreamServers creates a upstream servers slice with the default server in it.
// proxy_pass to an upstream with the default server returns current time port 37.
// We use it for services that have no endpoints.
func NewDefaultTCPServerUpstreamServers() []UpstreamServer {
	return []UpstreamServer{
		{
			Address: net.TCPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 37,
			},
		},
	}
}
