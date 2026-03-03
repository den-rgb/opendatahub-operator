package e2e_test

import (
	"strings"
)

// FailureCategory classifies a test failure by its root cause.
type FailureCategory int

const (
	FailureCategoryUnknown        FailureCategory = iota
	FailureCategoryInfrastructure                 // Cluster/network/operator-level issues
	FailureCategoryTestLogic                      // Assertion failures, wrong values, test bugs
)

func (fc FailureCategory) String() string {
	switch fc {
	case FailureCategoryInfrastructure:
		return "infrastructure"
	case FailureCategoryTestLogic:
		return "test-logic"
	default:
		return "unknown"
	}
}

// indicate a cluster or network-level problem rather than a test logic bug.
var infrastructurePatterns = []string{
	"connection refused",
	"connection reset by peer",
	"i/o timeout",
	"no such host",
	"dial tcp",
	"no route to host",
	"network is unreachable",
	"tls handshake timeout",
	"certificate has expired",
	"x509: certificate",
	"server unavailable",
	"the server was unable to return a response",
	"the server is currently unable to handle the request",
	"etcdserver: request timed out",
	"etcdserver: leader changed",
	"context deadline exceeded",
	"net/http: request canceled",
	"unable to upgrade connection",
	"storageerror",
	"no matches for kind",
	"the server could not find the requested resource",
	"serviceaccount \"default\" not found",
	"nodes are available",
	"back-off restarting failed container",
	"crashloopbackoff",
	"imagepullbackoff",
	"errimagepull",
}

// testLogicPatterns are substrings that indicate a Gomega/testify assertion failure.
var testLogicPatterns = []string{
	"Expected\n",
	"to equal",
	"not to equal",
	"to contain element",
	"not to have occurred",
	"to be true",
	"to be false",
	"to match",
	"to consist of",
	"to have http status",
	"Unexpected extra element",
	"Missing expected element",
	"Expected error:",
}

// ClassifyFailure categorizes a test failure based on pattern matching against
// the failure output text.
//
// When both infrastructure and test-logic patterns are present, the category
// with more pattern matches wins. This handles cases like assertion failures
// that wrap infrastructure errors.
func ClassifyFailure(output string) FailureCategory {
	if output == "" {
		return FailureCategoryUnknown
	}

	lower := strings.ToLower(output)

	infraScore := 0
	for _, pattern := range infrastructurePatterns {
		if strings.Contains(lower, pattern) {
			infraScore++
		}
	}

	testLogicScore := 0
	for _, pattern := range testLogicPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			testLogicScore++
		}
	}

	if infraScore > 0 && infraScore >= testLogicScore {
		return FailureCategoryInfrastructure
	}
	if testLogicScore > 0 {
		return FailureCategoryTestLogic
	}

	return FailureCategoryUnknown
}
