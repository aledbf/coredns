package kubernetes

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/client/unversioned"
	kube_client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/controller/framework"
	kubectl_util "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/watch"
)

// Defaults
const (
	defaultBaseUrl = "http://localhost:8080"
)

var (
	keyFunc   = framework.DeletionHandlingMetaNamespaceKeyFunc
	namespace = api.NamespaceAll
)

type controller struct {
	client *kube_client.Client

	domain string

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

func (c *controller) GetNamespaceList() *NamespaceList {
	return namespaces
}

func (c *controller) GetServiceList() *ServiceList {
}

// GetServicesByNamespace returns a map of
// namespacename :: [ kubernetesServiceItem ]
func (c *controller) GetServicesByNamespace() map[string][]ServiceItem {

	items := make(map[string][]ServiceItem)

	k8sServiceList := c.GetServiceList()

	if k8sServiceList == nil {
		return nil
	}

	k8sItemList := k8sServiceList.Items

	for _, i := range k8sItemList {
		namespace := i.Metadata.Namespace
		items[namespace] = append(items[namespace], i)
	}

	return items
}

// GetServiceItemInNamespace returns the ServiceItem that matches
// servicename in the namespace
func (c *controller) GetServiceItemInNamespace(namespace string, servicename string) *ServiceItem {
	// No matching item found in namespace
	return nil
}

func NewController(baseUrl string) *controller {
	k := new(controller)

	clientConfig := kubectl_util.DefaultClientConfig(flags)
	flags.Parse(os.Args)

	var err error
	var kubeClient *unversioned.Client

	if *cluster {
		if kubeClient, err = unversioned.NewInCluster(); err != nil {
			glog.Fatalf("Failed to create client: %v", err)
		}
	} else {
		config, err := clientConfig.ClientConfig()
		if err != nil {
			glog.Fatalf("error connecting to the client: %v", err)
		}
		kubeClient, err = unversioned.New(config)
	}

	domain := *argDomain
	if !strings.HasSuffix(domain, ".") {
		domain = fmt.Sprintf("%s.", domain)
	}

	ks, err := newdnsController(domain, kubeClient, *resyncPeriod)
	if err != nil {
		glog.Fatalf("error creating dns controller: %v", err)
	}

	ks.Run()

	dns.podLister.Store, dns.podController = framework.NewInformer(
		&cache.ListWatch{
			ListFunc:  podsListFunc(dns.client, namespace),
			WatchFunc: podsWatchFunc(dns.client, namespace),
		},
		&extensions.Ingress{}, resyncPeriod, framework.ResourceEventHandlerFuncs{})

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
