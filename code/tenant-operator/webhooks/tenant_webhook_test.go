package webhooks

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	platformv1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/tenant-operator/api/v1alpha1"
)

func validator(t *testing.T, objs ...client.Object) *TenantValidator {
	t.Helper()
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = platformv1alpha1.AddToScheme(s)
	return &TenantValidator{Reader: fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()}
}

func tenant(name string, namespaces ...string) *platformv1alpha1.Tenant {
	return &platformv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: platformv1alpha1.TenantSpec{
			DisplayName: name,
			Owners:      []string{"alice"},
			Namespaces:  namespaces,
			Quota:       platformv1alpha1.TenantQuota{CPU: "2", Memory: "4Gi"},
		},
	}
}

func TestValidateCreate(t *testing.T) {
	other := tenant("team-b", "shared")
	unmanaged := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "legacy"}}
	mine := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "team-a-existing", Labels: map[string]string{platformv1alpha1.LabelTenant: "team-a"},
	}}

	tests := []struct {
		name    string
		tenant  *platformv1alpha1.Tenant
		wantErr string
	}{
		{name: "valid", tenant: tenant("team-a", "team-a-dev")},
		{name: "re-adopts own namespace", tenant: tenant("team-a", "team-a-existing")},
		{name: "collision with other tenant", tenant: tenant("team-a", "shared"), wantErr: `claimed by tenant "team-b"`},
		{name: "unmanaged namespace", tenant: tenant("team-a", "legacy"), wantErr: "not managed by any tenant"},
		{name: "duplicate", tenant: tenant("team-a", "x", "x"), wantErr: "Duplicate value"},
		{name: "invalid DNS label", tenant: tenant("team-a", "Team_A"), wantErr: "spec.namespaces[0]"},
		{
			name: "bad quota",
			tenant: func() *platformv1alpha1.Tenant {
				tn := tenant("team-a", "a")
				tn.Spec.Quota.Memory = "lots"
				return tn
			}(),
			wantErr: "spec.quota.memory",
		},
		{
			name: "negative quota",
			tenant: func() *platformv1alpha1.Tenant {
				tn := tenant("team-a", "a")
				tn.Spec.Quota.CPU = "-1"
				return tn
			}(),
			wantErr: "must not be negative",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := validator(t, other, unmanaged, mine)
			_, err := v.ValidateCreate(context.Background(), tc.tenant)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantErr)
			}
			if !apierrors.IsInvalid(err) {
				t.Errorf("expected Invalid status error, got %T", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateUpdateIgnoresSelf(t *testing.T) {
	existing := tenant("team-a", "team-a-dev")
	v := validator(t, existing)
	updated := existing.DeepCopy()
	updated.Spec.Namespaces = append(updated.Spec.Namespaces, "team-a-prod")
	if _, err := v.ValidateUpdate(context.Background(), existing, updated); err != nil {
		t.Fatalf("update should be allowed: %v", err)
	}
}

func TestWarnsWithoutOwners(t *testing.T) {
	v := validator(t)
	tn := tenant("team-a", "a")
	tn.Spec.Owners = nil
	warnings, err := v.ValidateCreate(context.Background(), tn)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestValidateDeleteAlwaysAllowed(t *testing.T) {
	if _, err := validator(t).ValidateDelete(context.Background(), tenant("x")); err != nil {
		t.Fatal(err)
	}
}
