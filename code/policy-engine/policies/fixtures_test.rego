package platform

import rego.v1

# A compliant Workload. Individual tests break one property at a time.
fixture_workload := {
	"operation": "CREATE",
	"kind": "Workload",
	"namespace": "payments",
	"name": "checkout",
	"parameters": {"allowedRegistries": ["ghcr.io/acme/"], "maxReplicas": 10},
	"object": {
		"apiVersion": "platform.mycompany.io/v1alpha1",
		"kind": "Workload",
		"metadata": {
			"name": "checkout",
			"labels": {"platform.mycompany.io/owner": "payments", "app.kubernetes.io/name": "checkout"},
		},
		"spec": {
			"image": "ghcr.io/acme/checkout:1.2.3",
			"replicas": 2,
			"port": 8080,
			"resources": {"limits": {"cpu": "500m", "memory": "256Mi"}},
		},
	},
}
