// internal/tui/graphview/edges.go
package graphview

import (
	"github.com/charmbracelet/lipgloss"
)

// Cell represents a single character in the rendering grid.
type Cell struct {
	Char  rune
	Style lipgloss.Style
}

// Unicode line-drawing characters.
const (
	charHorizontal = '─'
	charVertical   = '│'
	charDiagUp     = '╱'
	charDiagDown   = '╲'
	charCornerTL   = '╭'
	charCornerTR   = '╮'
	charCornerBL   = '╰'
	charCornerBR   = '╯'
	charCrossing   = '┼'
	charArrowRight = '→'
	charArrowDown  = '↓'
	charArrowUp    = '↑'
	charArrowLeft  = '←'
)

// isLineChar returns true if the rune is one of our edge-drawing characters.
func isLineChar(r rune) bool {
	switch r {
	case charHorizontal, charVertical, charDiagUp, charDiagDown,
		charCornerTL, charCornerTR, charCornerBL, charCornerBR,
		charCrossing, charArrowRight, charArrowDown, charArrowUp, charArrowLeft:
		return true
	}
	return false
}

// DrawEdge draws a line from (x1,y1) to (x2,y2) on the grid using Unicode
// line characters. Uses a modified Bresenham algorithm with direction-based
// character selection. An arrow is placed at the destination endpoint.
func DrawEdge(grid [][]Cell, x1, y1, x2, y2 int, style lipgloss.Style) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}
	rows := len(grid)
	cols := len(grid[0])

	inBounds := func(x, y int) bool {
		return y >= 0 && y < rows && x >= 0 && x < cols
	}

	setCell := func(x, y int, ch rune) {
		if !inBounds(x, y) {
			return
		}
		existing := grid[y][x].Char
		if existing != 0 && existing != ' ' {
			if isLineChar(existing) && isLineChar(ch) && existing != ch {
				// Two different line chars crossing -> crossing symbol.
				grid[y][x] = Cell{Char: charCrossing, Style: style}
				return
			}
			// Don't overwrite non-empty, non-space cells (nodes, labels).
			if !isLineChar(existing) {
				return
			}
		}
		grid[y][x] = Cell{Char: ch, Style: style}
	}

	dx := x2 - x1
	dy := y2 - y1

	// Place arrow at destination.
	if inBounds(x2, y2) {
		arrow := arrowChar(dx, dy)
		grid[y2][x2] = Cell{Char: arrow, Style: style}
	}

	// Bresenham line drawing for the body of the edge.
	bresenham(x1, y1, x2, y2, func(x, y int, first, last bool) {
		if last {
			return // arrow already placed
		}
		if first {
			return // skip start point (node is there)
		}

		// Select character based on local direction.
		ch := lineChar(dx, dy)
		setCell(x, y, ch)
	})
}

// arrowChar picks an arrow character based on the overall direction.
func arrowChar(dx, dy int) rune {
	absDx := abs(dx)
	absDy := abs(dy)

	if absDx >= absDy {
		// Primarily horizontal.
		if dx >= 0 {
			return charArrowRight
		}
		return charArrowLeft
	}
	// Primarily vertical.
	if dy >= 0 {
		return charArrowDown
	}
	return charArrowUp
}

// lineChar picks a line character based on the overall dx, dy direction.
func lineChar(dx, dy int) rune {
	absDx := abs(dx)
	absDy := abs(dy)

	if absDy == 0 {
		return charHorizontal
	}
	if absDx == 0 {
		return charVertical
	}

	// Diagonal: pick based on angle ratio.
	ratio := float64(absDy) / float64(absDx)
	if ratio < 0.4 {
		return charHorizontal
	}
	if ratio > 2.5 {
		return charVertical
	}

	// True diagonal.
	if (dx > 0 && dy < 0) || (dx < 0 && dy > 0) {
		return charDiagUp // ╱
	}
	return charDiagDown // ╲
}

// bresenham iterates over pixels along a line from (x1,y1) to (x2,y2).
// The callback receives each point plus whether it is the first or last.
func bresenham(x1, y1, x2, y2 int, fn func(x, y int, first, last bool)) {
	dx := abs(x2 - x1)
	dy := abs(y2 - y1)
	sx := 1
	if x1 > x2 {
		sx = -1
	}
	sy := 1
	if y1 > y2 {
		sy = -1
	}

	err := dx - dy
	first := true

	for {
		isLast := x1 == x2 && y1 == y2
		fn(x1, y1, first, isLast)
		first = false

		if isLast {
			break
		}

		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x1 += sx
		}
		if e2 < dx {
			err += dx
			y1 += sy
		}
	}
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
