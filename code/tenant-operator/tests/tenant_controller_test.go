package tests

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/tenant-operator/api/v1alpha1"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/tenant-operator/controllers"
)

const timeout = 20 * time.Second

func newTenant(name string, namespaces ...string) *platformv1alpha1.Tenant {
	return &platformv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: platformv1alpha1.TenantSpec{
			DisplayName:      strings.ToUpper(name),
			Owners:           []string{"alice@example.com"},
			Namespaces:       namespaces,
			Quota:            platformv1alpha1.TenantQuota{CPU: "2", Memory: "4Gi", Pods: 10},
			NetworkIsolation: true,
		},
	}
}

func TestTenantProvisioning(t *testing.T) {
	requireEnvtest(t)
	ctx := context.Background()

	tn := newTenant("it-alpha", "it-alpha-dev", "it-alpha-prod")
	if err := k8sClient.Create(ctx, tn); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, tn) })

	eventually(t, timeout, func() error {
		got := &platformv1alpha1.Tenant{}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(tn), got); err != nil {
			return err
		}
		if !meta.IsStatusConditionTrue(got.Status.Conditions, platformv1alpha1.ConditionReady) {
			return fmt.Errorf("not ready yet: phase=%q", got.Status.Phase)
		}
		return nil
	})

	for _, ns := range tn.Spec.Namespaces {
		n := &corev1.Namespace{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: ns}, n); err != nil {
			t.Fatalf("namespace %s: %v", ns, err)
		}
		if n.Labels[platformv1alpha1.LabelTenant] != tn.Name {
			t.Errorf("namespace %s labels = %v", ns, n.Labels)
		}
		checks := []client.Object{
			&corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: controllers.ResourceQuotaName}},
			&corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: controllers.LimitRangeName}},
			&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: controllers.DefaultDenyPolicyName}},
			&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: controllers.AllowDNSPolicyName}},
			&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: controllers.OwnersRoleBindingName}},
		}
		for _, obj := range checks {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
				t.Errorf("%T %s/%s: %v", obj, ns, obj.GetName(), err)
			}
		}
	}
}

func TestWebhookRejectsNamespaceCollision(t *testing.T) {
	requireEnvtest(t)
	ctx := context.Background()

	first := newTenant("it-beta", "it-shared")
	if err := k8sClient.Create(ctx, first); err != nil {
		t.Fatalf("create first tenant: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, first) })

	second := newTenant("it-gamma", "it-shared")
	err := k8sClient.Create(ctx, second)
	if err == nil {
		_ = k8sClient.Delete(ctx, second)
		t.Fatal("expected webhook to reject namespace collision")
	}
	if !strings.Contains(err.Error(), "already claimed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCRDRejectsReservedNamespace(t *testing.T) {
	requireEnvtest(t)
	err := k8sClient.Create(context.Background(), newTenant("it-delta", "kube-hack"))
	if !apierrors.IsInvalid(err) {
		t.Fatalf("expected CEL validation error, got %v", err)
	}
}

func TestTenantDeletionReleasesFinalizer(t *testing.T) {
	requireEnvtest(t)
	ctx := context.Background()

	tn := newTenant("it-epsilon", "it-epsilon-dev")
	if err := k8sClient.Create(ctx, tn); err != nil {
		t.Fatal(err)
	}
	eventually(t, timeout, func() error {
		return k8sClient.Get(ctx, client.ObjectKey{Name: "it-epsilon-dev"}, &corev1.Namespace{})
	})

	if err := k8sClient.Delete(ctx, tn); err != nil {
		t.Fatal(err)
	}
	// envtest has no namespace lifecycle controller, so namespaces stay
	// Terminating; the operator only waits for the delete to be accepted.
	eventually(t, timeout, func() error {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(tn), &platformv1alpha1.Tenant{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("tenant still present (err=%v)", err)
	})
	ns := &corev1.Namespace{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: "it-epsilon-dev"}, ns); err == nil && ns.DeletionTimestamp.IsZero() {
		t.Fatal("namespace was not deleted")
	}
}
