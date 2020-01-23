package configuration

import (
	"fmt"
	"net"

	"github.com/mohamed-gougam/kube-agent/internal/configuration/version1"
	k8snginx_v1 "github.com/mohamed-gougam/kube-agent/pkg/apis/k8snginx/v1"
)

// TCPServerEx describes a TCPServerEx object.
type TCPServerEx struct {
	TCPServer        *k8snginx_v1.TCPServer
	ServiceAddresses []*net.TCPAddr
}

// NewTCPServerEx returns a new TCPServerEx.
func NewTCPServerEx(tcps *k8snginx_v1.TCPServer,
	svcExternalIPs []string) (*TCPServerEx, error) {
	var serviceAddresses []*net.TCPAddr
	var allErrors error

	for _, sAdr := range svcExternalIPs {
		adr, err := net.ResolveTCPAddr("", sAdr)
		if err != nil {
			if allErrors != nil {
				allErrors = fmt.Errorf("%v\nError Wrong TCP Address format of %v: %v", allErrors, sAdr, err)
			} else {
				allErrors = fmt.Errorf("Error Wrong TCP Address format of %v: %v", sAdr, err)
			}
			continue
		}
		serviceAddresses = append(serviceAddresses, adr)
	}

	return &TCPServerEx{
		TCPServer:        tcps,
		ServiceAddresses: serviceAddresses,
	}, allErrors
}

func generateNginxTCPServerCfg(tcpServerEx *TCPServerEx) *version1.TCPServerConf {
	// Very simple for now. Might be extended
	result := &version1.TCPServerConf{
		ListenPort: tcpServerEx.TCPServer.Spec.ListenPort,
		Upstream: version1.Upstream{
			Name:            getFileNameForTCPServer(tcpServerEx.TCPServer),
			UpstreamServers: []version1.UpstreamServer{},
		},
	}

	if len(tcpServerEx.ServiceAddresses) > 0 {
		for _, adr := range tcpServerEx.ServiceAddresses {
			result.Upstream.UpstreamServers = append(result.Upstream.UpstreamServers, version1.UpstreamServer{
				Address: *adr,
			})
		}
		return result
	}

	result.Upstream.UpstreamServers = version1.NewDefaultTCPServerUpstreamServers()

	return result
}
