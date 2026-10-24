package platform

import rego.v1

test_compliant_workload_has_no_violations if {
	count(deny) == 0 with input as fixture_workload
}

test_missing_owner_label_is_denied if {
	count(deny) == 1 with input as fixture_workload
		with input.object.metadata.labels as {"app.kubernetes.io/name": "checkout"}
}

test_empty_owner_label_is_denied if {
	count(deny) == 1 with input as fixture_workload
		with input.object.metadata.labels as {"platform.mycompany.io/owner": "", "app.kubernetes.io/name": "checkout"}
}

test_missing_name_label_is_denied if {
	count(deny) == 1 with input as fixture_workload
		with input.object.metadata.labels as {"platform.mycompany.io/owner": "payments"}
}

test_tenant_requires_owner_label if {
	count(deny) == 1 with input as {"kind": "Tenant", "name": "team-a", "object": {"metadata": {"labels": {}}}}
}

test_tenant_with_owner_label_passes if {
	count(deny) == 0 with input as {
		"kind": "Tenant",
		"name": "team-a",
		"object": {"metadata": {"labels": {"platform.mycompany.io/owner": "team-a"}}},
	}
}
