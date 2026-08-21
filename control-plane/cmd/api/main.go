// Package main is the entrypoint for the skquad control-plane API server.
//
// It serves the REST API (see docs/api-design.md), performs OIDC authN and
// user RBAC, and creates the Squad/Agent custom resources that the operator
// reconciles.
package main

func main() {
	// TODO(phase-3): wire up the HTTP server, OIDC middleware, RBAC, and
	// the Squad/Agent CR writers. See docs/api-design.md.
}
