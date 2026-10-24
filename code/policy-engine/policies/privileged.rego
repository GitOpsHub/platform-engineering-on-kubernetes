# No privileged containers.
# Nothing in a platform object may request privileged mode.
package platform

import rego.v1

deny contains {"policy": "no-privileged", "message": msg} if {
	walk(input.object, [path, value])
	value.privileged == true
	msg := sprintf("%s %q: privileged mode is not allowed (at %v)", [input.kind, input.name, path])
}
