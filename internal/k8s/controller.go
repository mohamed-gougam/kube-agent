package k8s

import (
	"fmt"
	"reflect"
	"time"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	endpointsLister  corelisters.EndpointsLister
	endpointsSynced  cache.InformerSynced
	podLister        corelisters.PodLister
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
	endpointsInformer coreinformers.EndpointsInformer,
	podInformer coreinformers.PodInformer,
	tcpServerInformer informers.TCPServerInformer,
	configurer *configuration.Configurer) *Controller {

	utilruntime.Must(k8snginxscheme.AddToScheme(scheme.Scheme))
	glog.V(3).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclient:       kubeclient,
		confclient:       confclient,
		servicesLister:   serviceInformer.Lister(),
		servicesSynced:   serviceInformer.Informer().HasSynced,
		endpointsLister:  endpointsInformer.Lister(),
		endpointsSynced:  endpointsInformer.Informer().HasSynced,
		podLister:        podInformer.Lister(),
		tcpServersLister: tcpServerInformer.Lister(),
		tcpServersSynced: tcpServerInformer.Informer().HasSynced,
		workqueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "TCPServers"),
		recorder:         recorder,
		configurer:       configurer,
	}

	glog.Info("Setting up event handlers")
	tcpServerInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			tcps := obj.(*k8snginx_v1.TCPServer)
			glog.V(3).Infof("Queue Sync[tcpserver]: Adding TCPServer: %v", tcps.Name)
			controller.enqueue(obj)
		},
		DeleteFunc: func(obj interface{}) {
			tcps, isTcps := obj.(*k8snginx_v1.TCPServer)
			if !isTcps {
				delState, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					glog.V(3).Infof("Error: received unexpected object: %v", obj)
					return
				}
				tcps, ok = delState.Obj.(*k8snginx_v1.TCPServer)
				if !ok {
					glog.V(3).Infof("Error: DeletedFinalStateUnknown contained non TCPServer object: %v", delState.Obj)
					return
				}
			}
			glog.V(3).Infof("Queue Sync[tcpserver]: Removing TCPServer: %v", tcps.Name)
			controller.enqueue(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if !reflect.DeepEqual(oldObj, newObj) {
				newTcps := newObj.(*k8snginx_v1.TCPServer)
				glog.V(3).Infof("Queue Sync[tcpserver]: TCPServer %v updated, apllying changes", newTcps.Name)
				controller.enqueue(newObj)
			}
		},
	})

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc := obj.(*corev1.Service)
			glog.V(3).Infof("Queue Sync[service]: Checking and Adding all TCPServers of namespace %v with serviceName %v", svc.Namespace, svc.Name)
			controller.enqueueList(controller.getTCPServersWithService(svc.Namespace, svc.Namespace))
		},
		DeleteFunc: func(obj interface{}) {
			svc, isSvc := obj.(*corev1.Service)
			if !isSvc {
				delState, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					glog.V(3).Infof("Error: received unexpected object: %v", obj)
					return
				}
				svc, ok = delState.Obj.(*corev1.Service)
				if !ok {
					glog.V(3).Infof("Error DeletedFinalStateUnknown contained non service object: %v", delState.Obj)
					return
				}
			}
			glog.V(3).Infof("Queue Sync[service]: Removing all TCPServers in namespace %v with serviceName %v", svc.Namespace, svc.Name)
			controller.enqueueList(controller.getTCPServersWithService(svc.Namespace, svc.Name))
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if !reflect.DeepEqual(oldObj, newObj) {
				svc := newObj.(*corev1.Service)
				glog.V(3).Infof("Queue Sync[service]: Updating all TCPServers of namespace %v with serviceName %v", svc.Namespace, svc.Name)
				controller.enqueueList(controller.getTCPServersWithService(svc.Namespace, svc.Name))
			}
		},
	})

	endpointsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ept := obj.(*corev1.Endpoints)
			glog.V(3).Infof("Queue Sync[endpoints]: Checking and Adding all TCPServers of namespace %v with serviceName %v", ept.Namespace, ept.Name)
			controller.enqueueList(controller.getTCPServersWithEndpoints(ept.Namespace, ept.Name))
		},
		DeleteFunc: func(obj interface{}) {
			ept, isEpt := obj.(*corev1.Endpoints)
			if !isEpt {
				delState, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					glog.V(3).Infof("Error: received unexpected object: %v", obj)
					return
				}
				ept, ok = delState.Obj.(*corev1.Endpoints)
				if !ok {
					glog.V(3).Infof("Error DeletedFinalStateUnknown contained non endpoints object: %v", delState.Obj)
					return
				}
			}
			glog.V(3).Infof("Queue Sync[endpoints]: Removing all TCPServers in namespace %v with serviceName %v", ept.Namespace, ept.Name)
			controller.enqueueList(controller.getTCPServersWithEndpoints(ept.Namespace, ept.Name))
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if !reflect.DeepEqual(oldObj, newObj) {
				ept := newObj.(*corev1.Endpoints)
				glog.V(3).Infof("Queue Sync[endpoints]: Updating all TCPServers of namespace %v with serviceName %v", ept.Namespace, ept.Name)
				controller.enqueueList(controller.getTCPServersWithEndpoints(ept.Namespace, ept.Name))
			}
		},
	})

	return controller
}

// Run runs the controller with threadiness number of workers.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	glog.Info("Starting Kube-Agent controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for services informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.servicesSynced, c.tcpServersSynced, c.endpointsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	// Launch threadiness workers to process TCPServers resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

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

		if err := c.syncTCPServers(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}

		c.workqueue.Forget(obj)
		glog.Infof("Successfully synced '%s'", key)

		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) syncTCPServers(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	tcps, err := c.tcpServersLister.TCPServers(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			glog.V(2).Infof("Deleting TCPServer: %v\n", key)

			err := c.configurer.DeleteTCPServer(key)
			if err != nil {
				glog.Errorf("Error when deleting configuration for %v: %v", key, err)
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
			glog.Errorf("Error when deleting configuration for %v: %v", key, err)
		}
		c.recorder.Eventf(tcps, corev1.EventTypeWarning, "Rejected", "TCPServer %v is invalid and was rejected: %v", key, validationErr)
		return nil
	}

	svc, err := c.servicesLister.Services(namespace).Get(tcps.Spec.ServiceName)
	if err != nil {
		if errors.IsNotFound(err) {
			glog.V(2).Infof("Adding or Updating TCPServer with serverName %v of a non existant service.\n", tcps.Spec.ServiceName)
			c.addOrUpdateTCPServerSync(tcps, &corev1.Service{}, &corev1.Endpoints{})
			return nil
		}
		return err
	}

	ept, err := c.endpointsLister.Endpoints(namespace).Get(tcps.Spec.ServiceName)
	if err != nil {
		if errors.IsNotFound(err) {
			glog.V(2).Infof("Adding or Updating TCPServer with serverName %v of a service with no endpoints.\n", tcps.Spec.ServiceName)
			c.addOrUpdateTCPServerSync(tcps, svc, &corev1.Endpoints{})
			return nil
		}
	}

	glog.V(2).Infof("Adding or updating TCPServer %v\n", key)

	c.addOrUpdateTCPServerSync(tcps, svc, ept)

	return nil
}

func (c *Controller) addOrUpdateTCPServerSync(tcps *k8snginx_v1.TCPServer, svc *corev1.Service, endpoints *corev1.Endpoints) {
	var stcpAdrs []string

	adrs, err := c.getEndpointsWithServiceAndPort(tcps.Spec.ServicePort, svc, endpoints)
	if err != nil {
		glog.V(3).Infof("Error getting endpoints for service %s/%s port %v: %v", svc.Namespace, svc.Name, tcps.Spec.ServicePort, err)
	} else {
		stcpAdrs = append(stcpAdrs, adrs...)
	}

	// Not exiting with error. Will serve tcp port 37 instead, default time.

	tcpsEx, err := configuration.NewTCPServerEx(tcps, stcpAdrs)
	if err != nil {
		// this case is impossible to happen
		glog.Errorf("Error when creating TCPServerEx for %s/%s: %v", tcps.Namespace, tcps.Name, err)
		c.recorder.Eventf(tcps, corev1.EventTypeWarning, "Altered", "Error creating TCPServerEx from TCPServer %s/%s: %v", tcps.Namespace, tcps.Name, err)
	}
	if err = c.configurer.AddOrUpdateTCPServer(tcpsEx); err != nil {
		glog.Errorf("Error when creating TCPServer NGINX config for %s/%s: %v", tcps.Namespace, tcps.Name, err)
		c.recorder.Eventf(tcps, corev1.EventTypeWarning, "AddedOrUpdatedWithError", "Configuration for %s/%s was added or updated but not applied %v", tcps.Namespace, tcps.Name, err)
	}
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

func (c *Controller) enqueueList(tcpss []*k8snginx_v1.TCPServer) {
	for _, tcps := range tcpss {
		c.enqueue(tcps)
	}
}

func (c *Controller) getTCPServersWithEndpoints(endpointsNamespace, endpointsName string) []*k8snginx_v1.TCPServer {
	// Enpoints Namespace and Name matchs exactly the service using them.
	return c.getTCPServersWithService(endpointsNamespace, endpointsName)
}

// Returns all TCPServers that have serviceName == service.Name and namespace == service.Namespace
func (c *Controller) getTCPServersWithService(serviceNamespace, serviceName string) []*k8snginx_v1.TCPServer {
	var result []*k8snginx_v1.TCPServer

	tcpss := c.getTCPServersInNamespace(serviceNamespace)

	for _, tcps := range tcpss {
		if tcps.Spec.ServiceName == serviceName {
			glog.V(3).Infof("Queue sync: TCPServer %s/%s synced.", tcps.Namespace, tcps.Name)
			result = append(result, tcps)
		}
	}

	return result
}

func (c *Controller) getTCPServersInNamespace(namespace string) []*k8snginx_v1.TCPServer {
	var result []*k8snginx_v1.TCPServer

	tcpss, err := c.tcpServersLister.TCPServers(namespace).List(labels.Everything())
	if err != nil {
		glog.Errorf("Error listing TCPServers of namespace %v: %v", namespace, err)
		return result
	}

	result = append(result, tcpss...)

	return result
}

func (c *Controller) getEndpointsWithServiceAndPort(tcpsServicePort int, svc *corev1.Service, endpoints *corev1.Endpoints) ([]string, error) {
	var targetPort int32
	var err error

	for _, port := range svc.Spec.Ports {
		if int(port.Port) == tcpsServicePort {
			targetPort, err = c.getTargetPort(&port, svc)
			if err != nil {
				return nil, fmt.Errorf("Error determining target port for port %v in Ingress: %v", tcpsServicePort, err)
			}
			break
		}
	}

	if targetPort == 0 {
		return nil, fmt.Errorf("No port %v in service %s/%s", tcpsServicePort, svc.Namespace, svc.Name)
	}

	for _, eptSub := range endpoints.Subsets {
		for _, eptPort := range eptSub.Ports {
			if eptPort.Port == targetPort {
				var stcpAdrs []string
				for _, eptAdr := range eptSub.Addresses {
					stcpAdrs = append(stcpAdrs, fmt.Sprintf("%s:%v", eptAdr.IP, eptPort.Port))
				}
				return stcpAdrs, nil
			}
		}
	}

	return nil, fmt.Errorf("No endpoints for target port %v in service %s", targetPort, svc.Name)
}

// COPIED
func (c *Controller) getTargetPort(svcPort *corev1.ServicePort, svc *corev1.Service) (int32, error) {
	if (svcPort.TargetPort == intstr.IntOrString{}) {
		return svcPort.Port, nil
	}

	if svcPort.TargetPort.Type == intstr.Int {
		return int32(svcPort.TargetPort.IntValue()), nil
	}

	//CHANGED To use own podLister
	pods, err := c.podLister.List(labels.Set(svc.Spec.Selector).AsSelector())
	if err != nil {
		return 0, fmt.Errorf("Error getting pod information: %v", err)
	}

	if len(pods) == 0 {
		return 0, fmt.Errorf("No pods of service %s", svc.Name)
	}

	pod := pods[0]

	portNum, err := findPort(pod, svcPort)
	if err != nil {
		return 0, fmt.Errorf("Error finding named port %v in pod %s: %v", svcPort, pod.Name, err)
	}

	return portNum, nil
}
