# The promoted digest must have run healthy locally and on infrastructure you
# control, so there is always a way back out of the cloud.
package gate.exit_path

required := ["local", "onprem"]

deny contains msg if {
	some env in required
	not healthy_record(env)
	msg := sprintf("no signed, healthy %s record for %s", [env, input.digest])
}

healthy_record(env) if {
	some r in input.records
	r.env == env
	r.digest == input.digest
	r.healthy
}
