package redis

import "fmt"

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
