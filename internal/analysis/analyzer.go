package analysis

import "github.com/endrilickollari/debtdrone-cli/v2/internal/scancore"

// Compatibility aliases. New scanner code uses the infrastructure-free
// scancore package so importing the public scanner does not pull in caching.
type Result = scancore.Result
type Analyzer = scancore.Analyzer
