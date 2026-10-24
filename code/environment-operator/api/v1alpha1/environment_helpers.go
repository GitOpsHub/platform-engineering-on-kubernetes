package v1alpha1

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var dnsLabelRegexp = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// NamespaceFor returns the namespace generated for an environment: "<tenant>-<name>".
func NamespaceFor(tenant, name string) string {
	return tenant + "-" + name
}

// URLFor returns the URL an environment is served at: https://<name>.<tenant>.<baseDomain>.
func URLFor(tenant, name, baseDomain string) string {
	return fmt.Sprintf("https://%s.%s.%s", name, tenant, baseDomain)
}

// IsReservedNamespace reports whether name may never be used for an environment namespace.
func IsReservedNamespace(name, operatorNamespace string) bool {
	if name == "" || name == "default" || name == "platform-system" || strings.HasPrefix(name, "kube-") {
		return true
	}
	return operatorNamespace != "" && name == operatorNamespace
}

// ValidateNamespace checks the generated namespace name for tenant/name.
func ValidateNamespace(tenant, name, operatorNamespace string) (string, error) {
	ns := NamespaceFor(tenant, name)
	if len(ns) > 63 {
		return ns, fmt.Errorf("namespace %q must be at most 63 characters", ns)
	}
	if !dnsLabelRegexp.MatchString(ns) {
		return ns, fmt.Errorf("namespace %q must be a DNS-1123 label", ns)
	}
	if IsReservedNamespace(ns, operatorNamespace) {
		return ns, fmt.Errorf("namespace %q is reserved", ns)
	}
	return ns, nil
}

// ExpiresAt returns creationTimestamp + ttl, or nil when no TTL is set.
func (e *Environment) ExpiresAt() *metav1.Time {
	if e.Spec.TTL == nil {
		return nil
	}
	t := metav1.NewTime(e.CreationTimestamp.Add(e.Spec.TTL.Duration))
	return &t
}

// IsExpired reports whether the environment's TTL has elapsed at now.
func (e *Environment) IsExpired(now time.Time) bool {
	exp := e.ExpiresAt()
	return exp != nil && !now.Before(exp.Time)
}

// OwnerUID returns the UID that generated objects must carry to be considered owned by e.
func (e *Environment) OwnerUID() types.UID { return e.UID }

// IsOwnedBy reports whether obj was generated for env: it carries the operator's
// managed-by label and the owner-uid annotation equal to env's UID. An empty UID never matches.
func IsOwnedBy(obj metav1.Object, env *Environment) bool {
	if env.UID == "" {
		return false
	}
	return obj.GetLabels()[LabelManagedBy] == ManagedByValue &&
		obj.GetAnnotations()[OwnerUIDAnnotation] == string(env.UID)
}

// CanAdoptNamespace reports whether an existing namespace may be taken over for env.
// It requires the managed-by, tenant and environment labels to match. A namespace
// already annotated for a different Environment UID is never adopted.
func CanAdoptNamespace(obj metav1.Object, env *Environment) bool {
	l := obj.GetLabels()
	if l[LabelManagedBy] != ManagedByValue || l[LabelTenant] != env.Spec.Tenant || l[LabelEnvironment] != env.Name {
		return false
	}
	owner, annotated := obj.GetAnnotations()[OwnerUIDAnnotation]
	return !annotated || owner == string(env.UID)
}
