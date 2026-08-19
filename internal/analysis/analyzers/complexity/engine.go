package complexity

type ComplexityNodeType string

const (
	Branch   ComplexityNodeType = "branch"   // if, switch, case, catch, conditional expression
	Loop     ComplexityNodeType = "loop"     // for, while, do
	Closure  ComplexityNodeType = "closure"  // lambdas, blocks
	Operator ComplexityNodeType = "operator" // boolean operators (&&, ||)
	Nesting  ComplexityNodeType = "nesting"  // nesting-only block
)

type Node struct {
	Type  ComplexityNodeType
	Depth int
}

// CalculateComplexity evaluates standard metrics based on a flat array of mapped ComplexityNodes
func CalculateComplexity(nodes []Node) (cyclomatic int, cognitive int, nesting int) {
	cyclomatic = 1
	cognitive = 0
	nesting = 0

	for _, n := range nodes {
		switch n.Type {
		case Branch, Loop, Closure:
			cyclomatic++
			cognitive += (1 + n.Depth)
		case Operator:
			cyclomatic++
			cognitive++
		case Nesting:
			// Nesting affects only the maximum depth below.
		}

		if n.Depth > nesting {
			nesting = n.Depth
		}
	}

	return cyclomatic, cognitive, nesting
}
