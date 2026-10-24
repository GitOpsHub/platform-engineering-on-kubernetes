# Required labels.
# Tenants and Environments need an owner label. Workloads also need app.kubernetes.io/name.
package platform

import rego.v1

required_labels := {
	"Tenant": ["platform.mycompany.io/owner"],
	"Environment": ["platform.mycompany.io/owner"],
	"Workload": ["platform.mycompany.io/owner", "app.kubernetes.io/name"],
}

# A label that is absent or empty counts as missing.
deny contains {"policy": "required-labels", "message": msg} if {
	some label in required_labels[input.kind]
	object.get(input.object, ["metadata", "labels", label], "") == ""
	msg := sprintf("%s %q is missing required label %q", [input.kind, input.name, label])
}
