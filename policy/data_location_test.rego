package gate.data_location_test

import data.gate.data_location

score := {"resources": {"db": {"type": "postgres"}, "web": {"type": "route"}}}

test_eu_region_passes if {
	count(data_location.deny) == 0 with input as {
		"score": score,
		"target": {"name": "eu", "provider": "scaleway", "resources": {"db": {"region": "fr-par"}}},
	}
}

test_us_region_fails if {
	some msg in data_location.deny with input as {
		"score": score,
		"target": {"name": "us", "provider": "aws", "resources": {"db": {"region": "us-east-1"}}},
	}
	contains(msg, "us-east-1")
}

test_eu_region_of_other_provider_fails if {
	count(data_location.deny) == 1 with input as {
		"score": score,
		"target": {"name": "x", "provider": "hetzner", "resources": {"db": {"region": "eu-west-1"}}},
	}
}

test_unknown_provider_fails if {
	count(data_location.deny) == 1 with input as {
		"score": score,
		"target": {"name": "x", "provider": "unknown", "resources": {"db": {"region": "fr-par"}}},
	}
}

test_missing_mapping_fails if {
	some msg in data_location.deny with input as {
		"score": score,
		"target": {"name": "x", "provider": "aws", "resources": {}},
	}
	contains(msg, "no region mapping")
}

test_no_stateful_resources_passes if {
	count(data_location.deny) == 0 with input as {
		"score": {"resources": {"web": {"type": "route"}}},
		"target": {"name": "x", "provider": "aws", "resources": {}},
	}
}
