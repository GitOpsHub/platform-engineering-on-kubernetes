package traits

import (
	"errors"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/workload-operator/api/v1alpha1"
)

func testWorkload() *v1alpha1.Workload {
	return &v1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "checkout", Namespace: "team-payments-prod",
			Labels: map[string]string{
				v1alpha1.LabelTenant:      "team-payments",
				v1alpha1.LabelEnvironment: "prod",
			},
		},
		Spec: v1alpha1.WorkloadSpec{Image: "nginx:1", Port: 8080},
	}
}

func TestLookupKnownAndUnknown(t *testing.T) {
	for _, typ := range []v1alpha1.TraitType{v1alpha1.TraitIngress, v1alpha1.TraitAutoscaling, v1alpha1.TraitPodDisruption} {
		tr, err := Lookup(typ)
		if err != nil || tr.Type() != typ {
			t.Fatalf("Lookup(%s) = %v, %v", typ, tr, err)
		}
	}
	if _, err := Lookup("cron"); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("unknown trait error = %v", err)
	}
}

func TestIngressBuild(t *testing.T) {
	tr, _ := Lookup(v1alpha1.TraitIngress)
	obj, err := tr.Build(testWorkload(), []byte(`{"host":"checkout.example.com","className":"nginx","tlsSecretName":"tls"}`))
	if err != nil {
		t.Fatal(err)
	}
	ing := obj.(*networkingv1.Ingress)
	if ing.Name != "checkout" || ing.Namespace != "team-payments-prod" {
		t.Fatalf("metadata = %+v", ing.ObjectMeta)
	}
	rule := ing.Spec.Rules[0]
	if rule.Host != "checkout.example.com" {
		t.Fatalf("host = %q", rule.Host)
	}
	p := rule.HTTP.Paths[0]
	if p.Path != "/" || p.Backend.Service.Name != "checkout" || p.Backend.Service.Port.Number != 8080 {
		t.Fatalf("path = %+v", p)
	}
	if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != "nginx" {
		t.Fatalf("class = %v", ing.Spec.IngressClassName)
	}
	if len(ing.Spec.TLS) != 1 || ing.Spec.TLS[0].SecretName != "tls" {
		t.Fatalf("tls = %+v", ing.Spec.TLS)
	}
	if ing.Labels[v1alpha1.LabelManagedBy] != v1alpha1.ManagedByValue {
		t.Fatalf("labels = %v", ing.Labels)
	}
}

func TestIngressRejectsBadConfig(t *testing.T) {
	tr, _ := Lookup(v1alpha1.TraitIngress)
	for name, raw := range map[string]string{
		"missing host":   `{}`,
		"bad host":       `{"host":"Not_A_Host"}`,
		"relative path":  `{"host":"a.example.com","path":"api"}`,
		"unknown field":  `{"host":"a.example.com","hots":"typo"}`,
		"malformed json": `{"host":`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tr.Build(testWorkload(), []byte(raw))
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("err = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestAutoscalingDefaultsAndBounds(t *testing.T) {
	tr, _ := Lookup(v1alpha1.TraitAutoscaling)

	obj, err := tr.Build(testWorkload(), []byte(`{"maxReplicas":5}`))
	if err != nil {
		t.Fatal(err)
	}
	hpa := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if *hpa.Spec.MinReplicas != 1 || hpa.Spec.MaxReplicas != 5 {
		t.Fatalf("min/max = %d/%d", *hpa.Spec.MinReplicas, hpa.Spec.MaxReplicas)
	}
	if hpa.Spec.ScaleTargetRef.Kind != "Deployment" || hpa.Spec.ScaleTargetRef.Name != "checkout" {
		t.Fatalf("target = %+v", hpa.Spec.ScaleTargetRef)
	}
	if *hpa.Spec.Metrics[0].Resource.Target.AverageUtilization != 80 {
		t.Fatalf("target utilization not defaulted to 80")
	}

	for name, raw := range map[string]string{
		"no max":          `{}`,
		"max below min":   `{"minReplicas":4,"maxReplicas":2}`,
		"zero min":        `{"minReplicas":0,"maxReplicas":2}`,
		"target too high": `{"maxReplicas":3,"targetCPUUtilizationPercentage":101}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tr.Build(testWorkload(), []byte(raw)); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestPodDisruptionBudget(t *testing.T) {
	tr, _ := Lookup(v1alpha1.TraitPodDisruption)

	obj, err := tr.Build(testWorkload(), nil)
	if err != nil {
		t.Fatal(err)
	}
	pdb := obj.(*policyv1.PodDisruptionBudget)
	if pdb.Spec.MinAvailable == nil || pdb.Spec.MinAvailable.IntValue() != 1 {
		t.Fatalf("default minAvailable = %v", pdb.Spec.MinAvailable)
	}
	if pdb.Spec.Selector.MatchLabels[v1alpha1.WorkloadLabel] != "checkout" {
		t.Fatalf("selector = %v", pdb.Spec.Selector.MatchLabels)
	}

	obj, err = tr.Build(testWorkload(), []byte(`{"maxUnavailable":"50%"}`))
	if err != nil {
		t.Fatal(err)
	}
	pdb = obj.(*policyv1.PodDisruptionBudget)
	if pdb.Spec.MaxUnavailable == nil || pdb.Spec.MaxUnavailable.String() != "50%" || pdb.Spec.MinAvailable != nil {
		t.Fatalf("spec = %+v", pdb.Spec)
	}

	if _, err := tr.Build(testWorkload(), []byte(`{"minAvailable":1,"maxUnavailable":1}`)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("both fields: err = %v", err)
	}
}

func TestPlaceholdersAreNamedAfterWorkload(t *testing.T) {
	w := testWorkload()
	for _, tr := range All() {
		obj := tr.Placeholder(w)
		if obj.GetName() != "checkout" || obj.GetNamespace() != "team-payments-prod" {
			t.Errorf("%s placeholder = %s/%s", tr.Type(), obj.GetNamespace(), obj.GetName())
		}
	}
}

func TestObjectLabelsCopyTenantAndEnvironment(t *testing.T) {
	l := ObjectLabels(testWorkload())
	if l[v1alpha1.LabelTenant] != "team-payments" || l[v1alpha1.LabelEnvironment] != "prod" ||
		l[v1alpha1.LabelManagedBy] != v1alpha1.ManagedByValue || l[v1alpha1.WorkloadLabel] != "checkout" {
		t.Fatalf("labels = %v", l)
	}
}
