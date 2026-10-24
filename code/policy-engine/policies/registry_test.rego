package platform

import rego.v1

test_allowed_registry_passes if {
	count(deny) == 0 with input as fixture_workload
}

test_unapproved_registry_is_denied if {
	count(deny) == 1 with input as fixture_workload
		with input.object.spec.image as "docker.io/library/nginx:latest"
}

test_prefix_must_match_exactly if {
	count(deny) == 1 with input as fixture_workload
		with input.object.spec.image as "ghcr.io/acme-evil/checkout:1"
}

test_missing_image_is_denied if {
	count(deny) == 1 with input as fixture_workload
		with input.object.spec.image as ""
}
