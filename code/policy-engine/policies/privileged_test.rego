package platform

import rego.v1

test_unprivileged_workload_passes if {
	count(deny) == 0 with input as fixture_workload
}

test_privileged_flag_is_denied if {
	count(deny) == 1 with input as fixture_workload
		with input.object.spec.securityContext as {"privileged": true}
}

test_nested_privileged_flag_is_denied if {
	count(deny) == 1 with input as fixture_workload
		with input.object.spec.sidecars as [{"name": "agent", "securityContext": {"privileged": true}}]
}

test_privileged_false_passes if {
	count(deny) == 0 with input as fixture_workload
		with input.object.spec.securityContext as {"privileged": false}
}
