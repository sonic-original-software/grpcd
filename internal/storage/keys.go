package storage

import "fmt"

// MethodKey returns the storage key for a method → address mapping
func MethodKey(method string) string {
	return fmt.Sprintf("method:%s", method)
}

// AddressKey returns the storage key for an address → methods set
func AddressKey(address string) string {
	return fmt.Sprintf("address:%s", address)
}
