# Every stateful resource must map to an EU region of the target's provider.
package gate.data_location

stateful := {"postgres", "mysql", "mongodb", "redis", "s3", "volume"}

deny contains msg if {
	some name, res in input.score.resources
	res.type in stateful
	not input.target.resources[name]
	msg := sprintf("resource %q (%s) has no region mapping in target %q", [name, res.type, input.target.name])
}

deny contains msg if {
	some name, res in input.score.resources
	res.type in stateful
	region := input.target.resources[name].region
	not eu_region(input.target.provider, region)
	msg := sprintf("resource %q (%s) maps to %s, not an EU region of %s", [name, res.type, region, input.target.provider])
}

eu_region(provider, region) if region in data.eu_regions[provider]
