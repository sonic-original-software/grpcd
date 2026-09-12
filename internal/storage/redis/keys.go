package redis

import (
	"fmt"
	"strings"
)

// additionsPrefix begins every additions channel; additionsPattern matches
// them all, which is what one instance subscribes to.
const (
	additionsPrefix  = "additions:"
	additionsPattern = additionsPrefix + "*"
)

// methodKey returns the key holding the set of addresses serving a method
func methodKey(method string) string {
	return fmt.Sprintf("method:%s", method)
}

// anchorKey returns the key holding the instance that anchors an address
func anchorKey(address string) string {
	return fmt.Sprintf("anchor:%s", address)
}

// channel returns the pub/sub channel an instance is told about removals on
func channel(anchor string) string {
	return fmt.Sprintf("removals:%s", anchor)
}

// additions returns the pub/sub channel a method's new addresses are announced on
func additions(method string) string {
	return additionsPrefix + method
}

// methodOf reads the method back out of an additions channel name
func methodOf(channel string) string {
	return strings.TrimPrefix(channel, additionsPrefix)
}
