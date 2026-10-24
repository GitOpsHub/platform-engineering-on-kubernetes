package traits

import (
	"fmt"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/workload-operator/api/v1alpha1"
)

// IngressConfig is the config of the ingress trait.
type IngressConfig struct {
	// Host is the public host name, e.g. checkout.example.com. Required.
	Host string `json:"host"`
	// Path is the URL path prefix. Defaults to "/".
	Path string `json:"path,omitempty"`
	// ClassName selects the IngressClass. Optional.
	ClassName string `json:"className,omitempty"`
	// TLSSecretName enables TLS for Host using this secret. Optional.
	TLSSecretName string `json:"tlsSecretName,omitempty"`
}

type ingressTrait struct{}

func (ingressTrait) Type() v1alpha1.TraitType { return v1alpha1.TraitIngress }

func (ingressTrait) Placeholder(w *v1alpha1.Workload) client.Object {
	return &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: w.Name, Namespace: w.Namespace}}
}

// Build renders an networking.k8s.io/v1 Ingress routing Host/Path to the Workload's Service.
func (t ingressTrait) Build(w *v1alpha1.Workload, raw []byte) (client.Object, error) {
	cfg := IngressConfig{Path: "/"}
	if err := decodeConfig(raw, &cfg); err != nil {
		return nil, err
	}
	if errs := validation.IsDNS1123Subdomain(cfg.Host); len(errs) > 0 {
		return nil, fmt.Errorf("%w: host %q: %s", ErrInvalidConfig, cfg.Host, strings.Join(errs, "; "))
	}
	if !strings.HasPrefix(cfg.Path, "/") {
		return nil, fmt.Errorf("%w: path %q must start with /", ErrInvalidConfig, cfg.Path)
	}
	if w.Spec.Port == 0 {
		return nil, fmt.Errorf("%w: ingress needs spec.port", ErrInvalidConfig)
	}

	pathType := networkingv1.PathTypePrefix
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: w.Name, Namespace: w.Namespace, Labels: ObjectLabels(w)},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: cfg.Host,
				IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{{
						Path:     cfg.Path,
						PathType: &pathType,
						Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
							Name: w.Name,
							Port: networkingv1.ServiceBackendPort{Number: w.Spec.Port},
						}},
					}},
				}},
			}},
		},
	}
	if cfg.ClassName != "" {
		ing.Spec.IngressClassName = &cfg.ClassName
	}
	if cfg.TLSSecretName != "" {
		ing.Spec.TLS = []networkingv1.IngressTLS{{Hosts: []string{cfg.Host}, SecretName: cfg.TLSSecretName}}
	}
	return ing, nil
}
