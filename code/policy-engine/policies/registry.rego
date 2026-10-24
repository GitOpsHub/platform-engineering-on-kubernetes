# Registry allowlist.
# Workload images must come from an approved registry prefix.
package platform

import rego.v1

deny contains {"policy": "registry-allowlist", "message": msg} if {
	input.kind == "Workload"
	image := object.get(input.object, ["spec", "image"], "")
	not image_allowed(image)
	msg := sprintf("Workload %q image %q is not under an allowed registry %v", [input.name, image, input.parameters.allowedRegistries])
}

image_allowed(image) if {
	some prefix in input.parameters.allowedRegistries
	startswith(image, prefix)
}
