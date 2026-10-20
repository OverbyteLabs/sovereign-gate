package gate.exit_path_test

import data.gate.exit_path

rec(env, digest, healthy) := {"env": env, "digest": digest, "healthy": healthy}

test_both_records_pass if {
	count(exit_path.deny) == 0 with input as {
		"digest": "sha256:a",
		"records": [rec("local", "sha256:a", true), rec("onprem", "sha256:a", true)],
	}
}

test_missing_onprem_fails if {
	some msg in exit_path.deny with input as {
		"digest": "sha256:a",
		"records": [rec("local", "sha256:a", true)],
	}
	contains(msg, "onprem")
}

test_unhealthy_record_fails if {
	count(exit_path.deny) == 1 with input as {
		"digest": "sha256:a",
		"records": [rec("local", "sha256:a", true), rec("onprem", "sha256:a", false)],
	}
}

test_record_for_other_digest_fails if {
	count(exit_path.deny) == 1 with input as {
		"digest": "sha256:a",
		"records": [rec("local", "sha256:a", true), rec("onprem", "sha256:b", true)],
	}
}

test_no_records_fails if {
	count(exit_path.deny) == 2 with input as {"digest": "sha256:a", "records": []}
}
