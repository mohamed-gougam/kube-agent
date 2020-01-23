package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mohamed-gougam/kube-agent/internal/metrics/collectors"
	"github.com/mohamed-gougam/kube-agent/internal/nginx"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/mohamed-gougam/kube-agent/internal/configuration"
	"github.com/mohamed-gougam/kube-agent/internal/configuration/version1"
	"github.com/mohamed-gougam/kube-agent/internal/k8s"
	clientset "github.com/mohamed-gougam/kube-agent/pkg/client/clientset/versioned"
	informers "github.com/mohamed-gougam/kube-agent/pkg/client/informers/externalversions"
)

var (
	masterURL  string
	kubeconfig string
)

func main() {
	//klog.InitFlags(nil)
	flag.Parse()

	// TO-DO: Create the confdPath/tcp directory

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	confClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building conf client: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	confInformerFactory := informers.NewSharedInformerFactory(confClient, time.Second*30)

	nginxBinaryPath := "/usr/sbin/nginx"

	tcpServerTemplatePath := "nginx.tcpserver.tmpl"

	managerCollector := collectors.NewManagerFakeCollector()

	nginxManager := nginx.NewLocalManager("/etc/nginx/", nginxBinaryPath, managerCollector)

	nginxDone := make(chan error, 1)
	nginxManager.Start(nginxDone)

	stopCh := make(chan struct{})

	go startSignalHandler(stopCh, nginxManager, nginxDone)

	templateExecutor, err := version1.NewTemplateExecutor(tcpServerTemplatePath)
	if err != nil {
		klog.Fatalf("Error creating TemplateExecutor: %v", err)
	}

	configurer := configuration.NewConfigurer(nginxManager, templateExecutor)

	controller := k8s.NewController(kubeClient, confClient,
		kubeInformerFactory.Core().V1().Services(),
		confInformerFactory.K8s().V1().TCPServers(),
		configurer)

	kubeInformerFactory.Start(stopCh)
	confInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}

func startSignalHandler(stop chan struct{}, nginxManager nginx.Manager, nginxDone chan error) {

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)

	exitStatus := 0
	exited := false

	select {
	case err := <-nginxDone:
		if err != nil {
			klog.Errorf("nginx command exited with an error: %v", err)
			exitStatus = 1
		} else {
			klog.Info("nginx command exited successfully")
		}
		exited = true
	case <-c:
		klog.Infof("Received SIGTERM, shutting down")
	}

	klog.Infof("Shutting down the controller")
	close(stop)

	if !exited {
		klog.Infof("Shutting down NGINX")
		nginxManager.Quit()
		<-nginxDone
	}

	klog.Infof("Exiting with a status: %v", exitStatus)
	os.Exit(exitStatus)
}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}
