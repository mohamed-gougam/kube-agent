package k8s

import (
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/mohamed-gougam/kube-agent/internal/configuration"
	k8snginx_v1 "github.com/mohamed-gougam/kube-agent/pkg/apis/k8snginx/v1"
	"github.com/mohamed-gougam/kube-agent/pkg/apis/k8snginx/validation"
	clientset "github.com/mohamed-gougam/kube-agent/pkg/client/clientset/versioned"
	k8snginxscheme "github.com/mohamed-gougam/kube-agent/pkg/client/clientset/versioned/scheme"
	informers "github.com/mohamed-gougam/kube-agent/pkg/client/informers/externalversions/k8snginx/v1"
	listers "github.com/mohamed-gougam/kube-agent/pkg/client/listers/k8snginx/v1"
)

const controllerAgentName = "k8s-nginx"

// Controller is the controller implementation
type Controller struct {
	kubeclient       kubernetes.Interface
	confclient       clientset.Interface
	servicesLister   corelisters.ServiceLister
	servicesSynced   cache.InformerSynced
	tcpServersLister listers.TCPServerLister
	tcpServersSynced cache.InformerSynced
	workqueue        workqueue.RateLimitingInterface
	recorder         record.EventRecorder
	configurer       *configuration.Configurer
}

// NewController returns a new controller
func NewController(kubeclient kubernetes.Interface,
	confclient clientset.Interface,
	serviceInformer coreinformers.ServiceInformer,
	tcpServerInformer informers.TCPServerInformer,
	configurer *configuration.Configurer) *Controller {

	utilruntime.Must(k8snginxscheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclient:       kubeclient,
		confclient:       confclient,
		servicesLister:   serviceInformer.Lister(),
		servicesSynced:   serviceInformer.Informer().HasSynced,
		tcpServersLister: tcpServerInformer.Lister(),
		tcpServersSynced: tcpServerInformer.Informer().HasSynced,
		workqueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "TCPServers"),
		recorder:         recorder,
		configurer:       configurer,
	}

	klog.Info("Setting up event handlers")
	tcpServerInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			tcps := obj.(*k8snginx_v1.TCPServer)
			klog.V(3).Infof("Adding TCPServer: %v", tcps.Name)
			controller.enqueue(obj)
		},
		DeleteFunc: func(obj interface{}) {
			tcps, isTcps := obj.(*k8snginx_v1.TCPServer)
			if !isTcps {
				delState, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					klog.V(3).Infof("Error: received unexpected object: %v", obj)
					return
				}
				tcps, ok = delState.Obj.(*k8snginx_v1.TCPServer)
				if !ok {
					klog.V(3).Infof("Error: DeletedFinalStateUnknown contained non TCPServer object: %v", delState.Obj)
					return
				}
			}
			klog.V(3).Infof("Removing TCPServer: %v", tcps.Name)
			controller.enqueue(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if !reflect.DeepEqual(oldObj, newObj) {
				newTcps := newObj.(*k8snginx_v1.TCPServer)
				klog.V(3).Infof("TCPServer %v updated, apllying changes", newTcps.Name)
				controller.enqueue(newObj)
			}
		},
	})

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			klog.V(3).Infof("Adding all TCPServers of namespace %v with serviceName %v", svc.Namespace, svc.Name)
			controller.enqueueFromService(svc)
		},
		DeleteFunc: func(obj interface{}) {
			svc, isSvc := obj.(*corev1.Service)
			if !isSvc {
				delState, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					klog.V(3).Infof("Error: received unexpected object: %v", obj)
					return
				}
				svc, ok = delState.Obj.(*corev1.Service)
				if !ok {
					klog.V(3).Infof("Error DeletedFinalStateUnknown contained non service object: %v", delState.Obj)
					return
				}
			}
			klog.V(3).Infof("Removing all TCPServers in namespace %v with serviceName %v", svc.Namespace, svc.Name)
			controller.enqueueFromService(svc)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if !reflect.DeepEqual(oldObj, newObj) {
				svc := newObj.(*corev1.Service)
				klog.V(3).Infof("Updating all TCPServers of namespace %v with serviceName %v", svc.Namespace, svc.Name)
				controller.enqueueFromService(svc)
			}
		},
	})

	return controller
}

// Run runs the controller with threadiness number of workers.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.Info("Starting Kube-Agent controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.servicesSynced, c.tcpServersSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch threadiness workers to process TCPServers resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)

		var key string
		var ok bool

		if key, ok = obj.(string); !ok {
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}

		if err := c.syncTCPServer(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}

		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)

		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) syncTCPServer(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	tcps, err := c.tcpServersLister.TCPServers(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(2).Infof("Deleting TCPServer: %v\n", key)

			err := c.configurer.DeleteTCPServer(key)
			if err != nil {
				klog.Errorf("Error when deleting configuration for %v: %v", key, err)
			}
			return nil
		}
		// network/transient error
		return err
	}

	validationErr := validation.ValidateTCPServer(tcps)
	if validationErr != nil {
		err := c.configurer.DeleteTCPServer(key)
		if err != nil {
			klog.Errorf("Error when deleting configuration for %v: %v", key, err)
		}
		c.recorder.Eventf(tcps, corev1.EventTypeWarning, "Rejected", "TCPServer %v is invalid and was rejected: %v", key, validationErr)
		return nil
	}

	svc, err := c.servicesLister.Services(namespace).Get(tcps.Spec.ServiceName)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(2).Infof("Adding or Updating TCPServer with serverName %v of a non existant service.\n", tcps.Spec.ServiceName)

			tcpsEx, err := configuration.NewTCPServerEx(tcps, []string{})
			if err != nil {
				// this case is impossible to happen
				klog.Errorf("Error when creating TCPServerEx for %v: %v", key, err)
				c.recorder.Eventf(tcps, corev1.EventTypeWarning, "Altered", "Error creating TCPServerEx from TCPServer %v: %v", key, err)
			}
			if err = c.configurer.AddOrUpdateTCPServer(tcpsEx); err != nil {
				klog.Errorf("Error when creating TCPServer NGINX config for %v: %v", key, err)
				c.recorder.Eventf(tcps, corev1.EventTypeWarning, "AddedOrUpdatedWithError", "Configuration for %v was added or updated but not applied %v", key, err)
			}
			return nil
		}
		return err
	}

	klog.V(2).Infof("Adding or updating TCPServer %v\n", key)
	tcpsEx, err := configuration.NewTCPServerEx(tcps, svc.Spec.ExternalIPs)
	if err != nil {
		klog.Errorf("Error when creating TCPServerEx for %v: %v", key, err)
		c.recorder.Eventf(tcps, corev1.EventTypeWarning, "Altered", "Error creating TCPServerEx from TCPServer %v: %v", key, err)
	}
	if err = c.configurer.AddOrUpdateTCPServer(tcpsEx); err != nil {
		klog.Errorf("Error when creating TCPServer NGINX config for %v: %v", key, err)
		c.recorder.Eventf(tcps, corev1.EventTypeWarning, "AddedOrUpdatedWithError", "Configuration for %v was added or updated but not applied %v", key, err)
	}

	return nil
}

func (c *Controller) enqueue(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

func (c *Controller) enqueueFromService(service *corev1.Service) {
	tcpss := c.getTCPServersWithService(service)

	for _, tcps := range tcpss {
		c.enqueue(tcps)
	}
}

// Returns all TCPServers that have serviceName == service.Name and namespace == service.Namespace
func (c *Controller) getTCPServersWithService(service *corev1.Service) []*k8snginx_v1.TCPServer {
	var result []*k8snginx_v1.TCPServer

	tcpss := c.getTCPServersInNamespace(service.Namespace)

	for _, tcps := range tcpss {
		if tcps.Spec.ServiceName == service.Name {
			result = append(result, tcps)
		}
	}

	return result
}

func (c *Controller) getTCPServersInNamespace(namespace string) []*k8snginx_v1.TCPServer {
	result, err := c.tcpServersLister.TCPServers(namespace).List(labels.Nothing())
	if err != nil {
		klog.Errorf("Error listing TCPServers of namespace %v: %v", namespace, err)
		return result
	}

	return result
}

// ----- might not need
func (c *Controller) getTCPServers() []*k8snginx_v1.TCPServer {
	result, err := c.tcpServersLister.List(labels.Nothing())
	if err != nil {
		klog.Errorf("Error listing TCPServers: %v", err)
		return result
	}

	return result
}
