package setup

import (
	"fmt"
	"strings"

	"github.com/miekg/coredns/middleware"
	"github.com/miekg/coredns/middleware/kubernetes"
	k8sc "github.com/miekg/coredns/middleware/kubernetes"
	"github.com/miekg/coredns/middleware/kubernetes/nametemplate"
)

const (
	defaultNameTemplate = "${service}.${namespace}.${zone}"
)

// Kubernetes sets up the kubernetes middleware.
func Kubernetes(c *Controller) (middleware.Middleware, error) {
	fmt.Println("controller %v", c)
	// TODO: Determine if subzone support required

	kubernetes, err := kubernetesParse(c)

	if err != nil {
		return nil, err
	}

	return func(next middleware.Handler) middleware.Handler {
		kubernetes.Next = next
		return kubernetes
	}, nil
}

func kubernetesParse(c *Controller) (kubernetes.Kubernetes, error) {
	k8s := k8sc.NewK8sConnector()
	k8s.NameTemplate = new(nametemplate.NameTemplate)
	k8s.NameTemplate.SetTemplate(defaultNameTemplate)

	for c.Next() {
		if c.Val() == "kubernetes" {
			zones := c.RemainingArgs()

			if len(zones) == 0 {
				k8s.Zones = c.ServerBlockHosts
			} else {
				// Normalize requested zones
				k8s.Zones = kubernetes.NormalizeZoneList(zones)
			}

			middleware.Zones(k8s.Zones).FullyQualify()
			if c.NextBlock() {
				switch c.Val() {
				case "endpoint":
					/*args := c.RemainingArgs()
					if len(args) == 0 {
						return kubernetes.Kubernetes{}, c.ArgErr()
					}*/
				}
				for c.Next() {
					switch c.Val() {
					case "template":
						args := c.RemainingArgs()
						if len(args) != 0 {
							template := strings.Join(args, "")
							k8s.NameTemplate.SetTemplate(template)
						}
					case "namespaces":
						args := c.RemainingArgs()
						if len(args) != 0 {
							k8s.Namespaces = &args
						}
					}
				}
			}

			return k8s, nil
		}
	}
	return kubernetes.Kubernetes{}, nil
}
