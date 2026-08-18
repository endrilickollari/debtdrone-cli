// Package scanner provides DebtDrone's reusable, infrastructure-neutral scanning API.
//
// It intentionally exposes no database, worker, organization, or SaaS integration
// types. Consumers provide a repository path and receive a neutral report.
package scanner
