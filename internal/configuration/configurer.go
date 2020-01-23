package configuration

import (
	"fmt"
	"strings"

	"github.com/mohamed-gougam/kube-agent/internal/configuration/version1"
	k8snginx_v1 "github.com/mohamed-gougam/kube-agent/pkg/apis/k8snginx/v1"
	"github.com/mohamed-gougam/kube-agent/internal/nginx"
)

// Configurer configures NGINX
type Configurer struct {
	nginxManager     nginx.Manager
	tcpServersEx     map[string]*TCPServerEx
	templateExecutor *version1.TemplateExecutor
}

// NewConfigurer return a new Configurer.
func NewConfigurer(nginxManager nginx.Manager, templateExecutor *version1.TemplateExecutor) *Configurer {
	return &Configurer{
		nginxManager:     nginxManager,
		tcpServersEx:     make(map[string]*TCPServerEx),
		templateExecutor: templateExecutor,
	}
}

// AddOrUpdateTCPServer adds TCPServer to nginx's config
func (cgr *Configurer) AddOrUpdateTCPServer(tcpServerEx *TCPServerEx) error {

	if err := cgr.addOrUpdateTCPServer(tcpServerEx); err != nil {
		return fmt.Errorf("Error adding or updating TCPServer %v/%v: %v", tcpServerEx.TCPServer.Namespace, tcpServerEx.TCPServer.Name, err)
	}

	if err := cgr.nginxManager.Reload(); err != nil {
		return fmt.Errorf("Error reloading NGINX for %v/%v: %v", tcpServerEx.TCPServer.Namespace, tcpServerEx.TCPServer.Name, err)
	}

	return nil
}

func (cgr *Configurer) addOrUpdateTCPServer(tcpServerEx *TCPServerEx) error {

	cfg := generateNginxTCPServerCfg(tcpServerEx)

	name := getFileNameForTCPServer(tcpServerEx.TCPServer)
	nginxConfig, err := cgr.templateExecutor.ExecuteTCPServerConfigTemplate(cfg)
	if err != nil {
		return fmt.Errorf("Error generating TCPServer Config %v: %v", name, err)
	}
	cgr.nginxManager.CreateConfig(name, nginxConfig)

	cgr.tcpServersEx[name] = tcpServerEx

	return nil
}

// DeleteTCPServer deletes NGINX configuration for the TCPServer
func (cgr *Configurer) DeleteTCPServer(key string) error {
	name := getFileNameForTCPServerFromKey(key)
	cgr.nginxManager.DeleteConfig(name)

	delete(cgr.tcpServersEx, key)

	if err := cgr.nginxManager.Reload(); err != nil {
		return fmt.Errorf("Error when removing TCPServer %v: %v", key, err)
	}

	return nil
}

func getFileNameForTCPServer(tcpServer *k8snginx_v1.TCPServer) string {
	return fmt.Sprintf("tcps_%s_%s", tcpServer.Namespace, tcpServer.Name)
}

func getFileNameForTCPServerFromKey(key string) string {
	return fmt.Sprintf("tcps_%s", strings.Replace(key, "/", "_", -1))
}
