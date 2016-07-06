// Package kubernetes provides the kubernetes backend.
package kubernetes

import (
	"errors"
	"fmt"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/kubernetes"
	"github.com/miekg/coredns/middleware/kubernetes/msg"
	"github.com/miekg/coredns/middleware/kubernetes/nametemplate"
	"github.com/miekg/coredns/middleware/kubernetes/util"
	"github.com/miekg/coredns/middleware/proxy"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

type Kubernetes struct {
	Next         middleware.Handler
	Zones        []string
	Proxy        proxy.Proxy // Proxy for looking up names during the resolution process
	Ctx          context.Context
	APIConn      *kubernetes.Controller
	NameTemplate *nametemplate.NameTemplate
	Namespaces   *[]string
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
	var (
		serviceName string
		namespace   string
		typeName    string
	)

	fmt.Println("enter Records('", name, "', ", exact, ")")
	zone, serviceSegments := g.getZoneForName(name)

	/*
	   // For initial implementation, assume namespace is first serviceSegment
	   // and service name is remaining segments.
	   serviceSegLen := len(serviceSegments)
	   if serviceSegLen >= 2 {
	       namespace = serviceSegments[serviceSegLen-1]
	       serviceName = strings.Join(serviceSegments[:serviceSegLen-1], ".")
	   }
	   // else we are looking up the zone. So handle the NS, SOA records etc.
	*/

	// TODO: Implementation above globbed together segments for the serviceName if
	//       multiple segments remained. Determine how to do similar globbing using
	//		 the template-based implementation.
	namespace = g.NameTemplate.GetNamespaceFromSegmentArray(serviceSegments)
	serviceName = g.NameTemplate.GetServiceFromSegmentArray(serviceSegments)
	typeName = g.NameTemplate.GetTypeFromSegmentArray(serviceSegments)

	fmt.Println("[debug] exact: ", exact)
	fmt.Println("[debug] zone: ", zone)
	fmt.Println("[debug] servicename: ", serviceName)
	fmt.Println("[debug] namespace: ", namespace)
	fmt.Println("[debug] typeName: ", typeName)
	fmt.Println("[debug] APIconn: ", g.APIConn)

	// TODO: Implement wildcard support to allow blank namespace value
	if namespace == "" {
		err := errors.New("Parsing query string did not produce a namespace value")
		fmt.Printf("[ERROR] %v\n", err)
		return nil, err
	}

	// Abort if the namespace is not published per CoreFile
	if g.Namespaces != nil && !util.StringInSlice(namespace, *g.Namespaces) {
		return nil, nil
	}

	k8sItem := g.APIConn.GetServiceItemInNamespace(namespace, serviceName)
	fmt.Println("[debug] k8s item:", k8sItem)

	// TODO: Update GetServiceItemInNamespace to produce a list of ServiceItems. (Stepping stone to wildcard support)

	if k8sItem == nil {
		// Did not find item in k8s
		return nil, nil
	}

	test := g.NameTemplate.GetRecordNameFromNameValues(nametemplate.NameValues{ServiceName: serviceName, TypeName: typeName, Namespace: namespace, Zone: zone})
	fmt.Printf("[debug] got recordname %v\n", test)

	records := g.getRecordsForServiceItems([]*k8sc.ServiceItem{k8sItem}, name)

	return records, nil
}

// TODO: assemble name from parts found in k8s data based on name template rather than reusing query string
func (g Kubernetes) getRecordsForServiceItems(serviceItems []*k8sc.ServiceItem, name string) []msg.Service {
	var records []msg.Service

	for _, item := range serviceItems {
		fmt.Println("[debug] clusterIP:", item.Spec.ClusterIP)
		for _, p := range item.Spec.Ports {
			fmt.Println("[debug]    port:", p.Port)
		}

		clusterIP := item.Spec.ClusterIP

		s := msg.Service{Host: name}
		records = append(records, s)
		for _, p := range item.Spec.Ports {
			s := msg.Service{Host: clusterIP, Port: p.Port}
			records = append(records, s)
		}
	}

	fmt.Printf("[debug] records from getRecordsForServiceItems(): %v\n", records)
	return records
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
