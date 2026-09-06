// Package rediskey defines the namespace for all project Redis keys.
package rediskey

import "strings"

// Prefix is fixed so the project can safely share Redis with other applications.
const Prefix = "ecommerce:"

// Key builds a namespaced key from trusted domain and identifier components.
func Key(parts ...string) string {
	return Prefix + strings.Join(parts, ":")
}
