package lib

import "crypto/tls"

// Verified is suppressed in gosec style; the report counts it.
var Verified = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- fixture

// Insecure skips certificate verification: a gosec G402 blocker.
var Insecure = &tls.Config{InsecureSkipVerify: true}
