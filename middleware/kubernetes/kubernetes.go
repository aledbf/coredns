// Package kubernetes provides the kubernetes backend.
package kubernetes

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/kubernetes/msg"
	"github.com/miekg/coredns/middleware/kubernetes/nametemplate"
	"github.com/miekg/coredns/middleware/proxy"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"

	"github.com/miekg/dns"
)

const (
	resyncPeriod = 30 * time.Second
)

type Kubernetes struct {
	Next         middleware.Handler
	Zones        []string
	Proxy        proxy.Proxy // Proxy for looking up names during the resolution process
	APIConn      *dnsController
	NameTemplate *nametemplate.NameTemplate
	Namespaces   *[]string
}

func NewK8sConnector() Kubernetes {
	kubeClient, err := unversioned.NewInCluster()
	if err != nil {
		// ("failed to create client: %v", err)
	}

	k8s := Kubernetes{
		APIConn: newdnsController(kubeClient, resyncPeriod),
	}

	go k8s.APIConn.Run()
	return k8s
}

// getZoneForName returns the zone string that matches the name and a
// list of the DNS labels from name that are within the zone.
// For example, if "coredns.local" is a zone configured for the
// Kubernetes middleware, then getZoneForName("a.b.coredns.local")
// will return ("coredns.local", ["a", "b"]).
func (g Kubernetes) getZoneForName(name string) (string, []string) {
	var zone string
	var serviceSegments []string

	for _, z := range g.Zones {
		if dns.IsSubDomain(z, name) {
			zone = z

			serviceSegments = dns.SplitDomainName(name)
			serviceSegments = serviceSegments[:len(serviceSegments)-dns.CountLabel(zone)]
			break
		}
	}

	return zone, serviceSegments
}

// Records looks up services in kubernetes.
// If exact is true, it will lookup just
// this name. This is used when find matches when completing SRV lookups
// for instance.
func (g Kubernetes) Records(name string, exact bool) ([]msg.Service, error) {
	if strings.HasSuffix(name, arpaSuffix) {
		ip, _ := extractIP(name)
		records := g.getServiceRecordForIP(ip, name)
		return records, nil
	}

	_, serviceSegments := g.getZoneForName(name)

	namespace := g.NameTemplate.GetNamespaceFromSegmentArray(serviceSegments)
	serviceName := g.NameTemplate.GetServiceFromSegmentArray(serviceSegments)

	// TODO: Implement wildcard support to allow blank namespace value
	if namespace == "" {
		err := errors.New("Parsing query string did not produce a namespace value")
		fmt.Printf("[ERROR] %v\n", err)
		return nil, err
	}

	k8sItem := g.APIConn.GetServiceInNamespace(namespace, serviceName)
	// TODO: Update GetServiceInNamespace to produce a list of Service. (Stepping stone to wildcard support)
	if k8sItem == nil {
		return nil, nil
	}

	records := g.getRecordsForService([]*api.Service{k8sItem}, name)
	return records, nil
}

// TODO: assemble name from parts found in k8s data based on name template rather than reusing query string
func (g Kubernetes) getRecordsForService(services []*api.Service, name string) []msg.Service {
	var records []msg.Service

	for _, item := range services {
		clusterIP := item.Spec.ClusterIP

		s := msg.Service{Host: name}
		records = append(records, s)
		for _, p := range item.Spec.Ports {
			s := msg.Service{Host: clusterIP, Port: int(p.Port)}
			records = append(records, s)
		}
	}

	return records
}

func (g Kubernetes) getServiceRecordForIP(ip, name string) []msg.Service {
	svcList, err := g.APIConn.svcLister.List()
	if err != nil {
		return nil
	}

	for _, service := range svcList.Items {
		if service.Spec.ClusterIP == ip {
			return []msg.Service{msg.Service{Host: ip}}
		}
	}

	return nil
}

func (g Kubernetes) splitDNSName(name string) []string {
	l := dns.SplitDomainName(name)

	for i, j := 0, len(l)-1; i < j; i, j = i+1, j-1 {
		l[i], l[j] = l[j], l[i]
	}

	return l
}

// isKubernetesNameError checks if the error is ErrorCodeKeyNotFound from kubernetes.
func isKubernetesNameError(err error) bool {
	return false
}
