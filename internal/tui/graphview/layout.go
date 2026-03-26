// internal/tui/graphview/layout.go
package graphview

import (
	"math"
	"math/rand"

	"github.com/depscope/depscope/internal/graph"
)

// Position represents a 2D coordinate in the layout space.
type Position struct {
	X, Y float64
}

// LayoutConfig controls the force-directed layout parameters.
type LayoutConfig struct {
	Width      float64
	Height     float64
	Iterations int
	Repulsion  float64
	Attraction float64
}

// DefaultLayoutConfig returns sensible defaults for terminal-sized layouts.
func DefaultLayoutConfig(width, height int) LayoutConfig {
	return LayoutConfig{
		Width:      float64(width),
		Height:     float64(height),
		Iterations: 100,
		Repulsion:  500.0,
		Attraction: 0.01,
	}
}

// Layout computes positions for graph nodes using a simplified
// Fruchterman-Reingold force-directed placement algorithm.
func Layout(g *graph.Graph, nodeIDs []string, cfg LayoutConfig) map[string]Position {
	n := len(nodeIDs)
	if n == 0 {
		return map[string]Position{}
	}

	// Single node: center it.
	if n == 1 {
		return map[string]Position{
			nodeIDs[0]: {X: cfg.Width / 2, Y: cfg.Height / 2},
		}
	}

	// Index for fast lookup.
	idx := make(map[string]int, n)
	for i, id := range nodeIDs {
		idx[id] = i
	}

	// Build edge pairs restricted to nodes in nodeIDs.
	type edgePair struct{ from, to int }
	var edges []edgePair
	for _, e := range g.Edges {
		fi, fok := idx[e.From]
		ti, tok := idx[e.To]
		if fok && tok {
			edges = append(edges, edgePair{fi, ti})
		}
	}

	// Margin keeps nodes away from the border.
	marginX := cfg.Width * 0.05
	marginY := cfg.Height * 0.05
	usableW := cfg.Width - 2*marginX
	usableH := cfg.Height - 2*marginY

	// Random initial positions within usable area.
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // deterministic layout
	posX := make([]float64, n)
	posY := make([]float64, n)
	for i := range n {
		posX[i] = marginX + rng.Float64()*usableW
		posY[i] = marginY + rng.Float64()*usableH
	}

	// Ideal spring length (area-based heuristic).
	area := usableW * usableH
	k := math.Sqrt(area / float64(n))

	// Temperature starts high and cools down linearly.
	temp := math.Max(usableW, usableH) / 2

	// Force accumulators.
	dx := make([]float64, n)
	dy := make([]float64, n)

	for iter := range cfg.Iterations {
		// Reset forces.
		for i := range n {
			dx[i] = 0
			dy[i] = 0
		}

		// Repulsion: every pair pushes apart (inverse square).
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				ddx := posX[i] - posX[j]
				ddy := posY[i] - posY[j]
				dist := math.Sqrt(ddx*ddx + ddy*ddy)
				if dist < 0.01 {
					dist = 0.01
				}
				// Repulsion force magnitude = repulsion * k^2 / dist
				force := cfg.Repulsion * k * k / (dist * dist)
				fx := ddx / dist * force
				fy := ddy / dist * force
				dx[i] += fx
				dy[i] += fy
				dx[j] -= fx
				dy[j] -= fy
			}
		}

		// Attraction: edges pull connected nodes together (spring).
		for _, e := range edges {
			ddx := posX[e.to] - posX[e.from]
			ddy := posY[e.to] - posY[e.from]
			dist := math.Sqrt(ddx*ddx + ddy*ddy)
			if dist < 0.01 {
				dist = 0.01
			}
			// Attraction force magnitude = attraction * dist^2 / k
			force := cfg.Attraction * dist * dist / k
			fx := ddx / dist * force
			fy := ddy / dist * force
			dx[e.from] += fx
			dy[e.from] += fy
			dx[e.to] -= fx
			dy[e.to] -= fy
		}

		// Apply forces, clamped by temperature.
		for i := range n {
			disp := math.Sqrt(dx[i]*dx[i] + dy[i]*dy[i])
			if disp > 0 {
				scale := math.Min(disp, temp) / disp
				posX[i] += dx[i] * scale
				posY[i] += dy[i] * scale
			}
			// Clamp to bounds.
			posX[i] = clamp(posX[i], marginX, cfg.Width-marginX)
			posY[i] = clamp(posY[i], marginY, cfg.Height-marginY)
		}

		// Cool down.
		temp *= 1.0 - float64(iter+1)/float64(cfg.Iterations)
		if temp < 0.1 {
			temp = 0.1
		}
	}

	// Build result map.
	result := make(map[string]Position, n)
	for i, id := range nodeIDs {
		result[id] = Position{X: posX[i], Y: posY[i]}
	}
	return result
}

// clamp restricts v to the range [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
