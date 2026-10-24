# Resource limits and replica ceiling.
# Workloads must set CPU and memory limits, and must not exceed the replica ceiling.
package platform

import rego.v1

deny contains {"policy": "resource-limits", "message": msg} if {
	input.kind == "Workload"
	some resource in ["cpu", "memory"]
	object.get(input.object, ["spec", "resources", "limits", resource], "") == ""
	msg := sprintf("Workload %q must set spec.resources.limits.%s", [input.name, resource])
}

deny contains {"policy": "resource-limits", "message": msg} if {
	input.kind == "Workload"
	replicas := object.get(input.object, ["spec", "replicas"], 0)
	replicas > input.parameters.maxReplicas
	msg := sprintf("Workload %q requests %v replicas; the maximum is %v", [input.name, replicas, input.parameters.maxReplicas])
}
