package platform

import rego.v1

test_limits_and_replicas_within_budget_pass if {
	count(deny) == 0 with input as fixture_workload
}

test_missing_memory_limit_is_denied if {
	count(deny) == 1 with input as fixture_workload
		with input.object.spec.resources.limits as {"cpu": "500m"}
}

test_missing_all_limits_is_denied if {
	count(deny) == 2 with input as fixture_workload
		with input.object.spec.resources as {}
}

test_replica_ceiling_is_enforced if {
	count(deny) == 1 with input as fixture_workload
		with input.object.spec.replicas as 50
}

test_replicas_at_ceiling_pass if {
	count(deny) == 0 with input as fixture_workload
		with input.object.spec.replicas as 10
}

test_limits_policy_only_applies_to_workloads if {
	count(deny) == 0 with input as {"kind": "Environment", "name": "pr-1", "object": {"metadata": {"labels": {"platform.mycompany.io/owner": "x"}}}}
}
