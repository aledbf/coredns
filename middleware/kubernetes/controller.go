package kubernetes

import (
	"fmt"
	"log"
	"sync"
	"time"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/controller/framework"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/watch"
)

var (
	keyFunc   = framework.DeletionHandlingMetaNamespaceKeyFunc
	namespace = api.NamespaceAll
)

type dnsController struct {
	client *client.Client

	podController  *framework.Controller
	endpController *framework.Controller
	svcController  *framework.Controller

	podLister  cache.StoreToPodLister
	svcLister  cache.StoreToServiceLister
	endpLister cache.StoreToEndpointsLister

	// stopLock is used to enforce only a single call to Stop is active.
	// Needed because we allow stopping through an http endpoint and
	// allowing concurrent stoppers leads to stack traces.
	stopLock sync.Mutex
	shutdown bool
	stopCh   chan struct{}
}

// newDNSController creates a controller for coredns
func newdnsController(kubeClient *client.Client, resyncPeriod time.Duration) *dnsController {
	dns := dnsController{
		client: kubeClient,
		stopCh: make(chan struct{}),
	}

	dns.podLister.Indexer, dns.podController = framework.NewIndexerInformer(
		cache.NewListWatchFromClient(dns.client, "pods", namespace, fields.Everything()),
		&api.Pod{},
		resyncPeriod,
		framework.ResourceEventHandlerFuncs{},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	dns.endpLister.Store, dns.endpController = framework.NewInformer(
		&cache.ListWatch{
			ListFunc:  endpointsListFunc(dns.client, namespace),
			WatchFunc: endpointsWatchFunc(dns.client, namespace),
		},
		&api.Endpoints{}, resyncPeriod, framework.ResourceEventHandlerFuncs{})

	dns.svcLister.Store, dns.svcController = framework.NewInformer(
		&cache.ListWatch{
			ListFunc:  serviceListFunc(dns.client, namespace),
			WatchFunc: serviceWatchFunc(dns.client, namespace),
		},
		&api.Service{}, resyncPeriod, framework.ResourceEventHandlerFuncs{})

	return &dns
}

func podsListFunc(c *client.Client, ns string) func(api.ListOptions) (runtime.Object, error) {
	return func(opts api.ListOptions) (runtime.Object, error) {
		return c.Pods(ns).List(opts)
	}
}

func podsWatchFunc(c *client.Client, ns string) func(options api.ListOptions) (watch.Interface, error) {
	return func(options api.ListOptions) (watch.Interface, error) {
		return c.Pods(ns).Watch(options)
	}
}

func serviceListFunc(c *client.Client, ns string) func(api.ListOptions) (runtime.Object, error) {
	return func(opts api.ListOptions) (runtime.Object, error) {
		return c.Services(ns).List(opts)
	}
}

func serviceWatchFunc(c *client.Client, ns string) func(options api.ListOptions) (watch.Interface, error) {
	return func(options api.ListOptions) (watch.Interface, error) {
		return c.Services(ns).Watch(options)
	}
}

func endpointsListFunc(c *client.Client, ns string) func(api.ListOptions) (runtime.Object, error) {
	return func(opts api.ListOptions) (runtime.Object, error) {
		return c.Endpoints(ns).List(opts)
	}
}

func endpointsWatchFunc(c *client.Client, ns string) func(options api.ListOptions) (watch.Interface, error) {
	return func(options api.ListOptions) (watch.Interface, error) {
		return c.Endpoints(ns).Watch(options)
	}
}

func (dns *dnsController) controllersInSync() bool {
	return dns.podController.HasSynced() && dns.svcController.HasSynced() && dns.endpController.HasSynced()
}

// Stop stops the  controller.
func (dns *dnsController) Stop() error {
	// Stop is invoked from the http endpoint.
	dns.stopLock.Lock()
	defer dns.stopLock.Unlock()

	// Only try draining the workqueue if we haven't already.
	if !dns.shutdown {

		close(dns.stopCh)
		log.Println("shutting down controller queues")
		dns.shutdown = true

		return nil
	}

	return fmt.Errorf("shutdown already in progress")
}

// Run starts the controller.
func (dns *dnsController) Run() {
	log.Println("starting coredns controller")

	go dns.podController.Run(dns.stopCh)
	go dns.endpController.Run(dns.stopCh)
	go dns.svcController.Run(dns.stopCh)

	<-dns.stopCh
	log.Println("shutting down coredns controller")
}

func (c *dnsController) GetNamespaceList() *api.NamespaceList {
	return nil
}

func (c *dnsController) GetServiceList() *api.ServiceList {
	return nil
}

// GetServicesByNamespace returns a map of
// namespacename :: [ kubernetesService ]
func (c *dnsController) GetServicesByNamespace() map[string][]api.Service {
	items := make(map[string][]api.Service)
	k8sServiceList := c.GetServiceList()
	if k8sServiceList == nil {
		return nil
	}

	for _, i := range k8sServiceList.Items {
		namespace := i.Namespace
		items[namespace] = append(items[namespace], i)
	}

	return items
}

// GetServiceInNamespace returns the Service that matches
// servicename in the namespace
func (c *dnsController) GetServiceInNamespace(namespace string, servicename string) *api.Service {
	// No matching item found in namespace
	return nil
}
