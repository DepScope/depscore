// internal/graph/graph.go
package graph

// Graph represents a supply chain dependency graph.
type Graph struct {
	Nodes map[string]*Node // keyed by Node.ID
	Edges []*Edge
	adj   map[string][]string // adjacency list: from → [to IDs]
}

// New creates an empty graph.
func New() *Graph {
	return &Graph{
		Nodes: make(map[string]*Node),
		adj:   make(map[string][]string),
	}
}

// AddNode adds a node to the graph. If a node with the same ID exists, it is replaced.
func (g *Graph) AddNode(n *Node) {
	g.Nodes[n.ID] = n
}

// Node returns the node with the given ID, or nil if not found.
func (g *Graph) Node(id string) *Node {
	return g.Nodes[id]
}

// AddEdge adds an edge to the graph and updates the adjacency list.
func (g *Graph) AddEdge(e *Edge) {
	g.Edges = append(g.Edges, e)
	g.adj[e.From] = append(g.adj[e.From], e.To)
}

// Neighbors returns the IDs of all nodes directly reachable from the given node.
func (g *Graph) Neighbors(id string) []string {
	return g.adj[id]
}

// NodesOfType returns all nodes matching the given type.
func (g *Graph) NodesOfType(t NodeType) []*Node {
	var result []*Node
	for _, n := range g.Nodes {
		if n.Type == t {
			result = append(result, n)
		}
	}
	return result
}

// FindPaths returns all paths from src to dst, up to maxDepth.
// Uses DFS with backtracking.
func (g *Graph) FindPaths(src, dst string, maxDepth int) [][]string {
	var result [][]string
	g.dfs(src, dst, maxDepth, []string{src}, make(map[string]bool), &result)
	return result
}

func (g *Graph) dfs(current, dst string, maxDepth int, path []string, visited map[string]bool, result *[][]string) {
	if current == dst && len(path) > 1 {
		p := make([]string, len(path))
		copy(p, path)
		*result = append(*result, p)
		return
	}
	if len(path) > maxDepth {
		return
	}
	visited[current] = true
	for _, next := range g.adj[current] {
		if !visited[next] {
			g.dfs(next, dst, maxDepth, append(path, next), visited, result)
		}
	}
	visited[current] = false
}
