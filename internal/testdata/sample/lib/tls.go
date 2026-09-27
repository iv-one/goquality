package lib

import "crypto/tls"

// Insecure skips certificate verification: a gosec G402 blocker.
var Insecure = &tls.Config{InsecureSkipVerify: true}
