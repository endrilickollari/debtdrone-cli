package complexity

type ComplexityNodeType string

const (
	Branch   ComplexityNodeType = "branch"   // if, switch, case, catch, conditional expression
	Loop     ComplexityNodeType = "loop"     // for, while, do
	Closure  ComplexityNodeType = "closure"  // lambdas, blocks
	Operator ComplexityNodeType = "operator" // boolean operators (&&, ||)
	Nesting  ComplexityNodeType = "nesting"  // Blocks that add cognitive nesting but aren't loops/branches (though those usually add nesting anyway)
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
			// Cognitive Complexity calculation: 1 base point + the current nesting depth
			cognitive += (1 + n.Depth)
		case Operator:
			cyclomatic++
			cognitive++
		case Nesting:
			// Nesting itself doesn't add cyclomatic, but affects the maximum depth
			// Wait, the depth increment is natively processed by the standard nodes (Branch/Loop/Operator).
		}

		if n.Depth > nesting {
			nesting = n.Depth
		}
	}

	return cyclomatic, cognitive, nesting
}
