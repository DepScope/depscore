package graphview

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func makeGrid(width, height int) [][]Cell {
	grid := make([][]Cell, height)
	for y := range height {
		grid[y] = make([]Cell, width)
		for x := range width {
			grid[y][x] = Cell{Char: ' '}
		}
	}
	return grid
}

func gridChars(grid [][]Cell) [][]rune {
	result := make([][]rune, len(grid))
	for y, row := range grid {
		result[y] = make([]rune, len(row))
		for x, cell := range row {
			result[y][x] = cell.Char
		}
	}
	return result
}

func TestDrawEdgeHorizontalRight(t *testing.T) {
	grid := makeGrid(10, 3)
	style := lipgloss.NewStyle()

	DrawEdge(grid, 1, 1, 8, 1, style)

	chars := gridChars(grid)
	// Start point is skipped (node lives there).
	assert.Equal(t, ' ', chars[1][1])
	// Body should be horizontal dashes.
	for x := 2; x <= 7; x++ {
		assert.Equal(t, charHorizontal, chars[1][x], "char at x=%d", x)
	}
	// End point should be right arrow.
	assert.Equal(t, charArrowRight, chars[1][8])
}

func TestDrawEdgeHorizontalLeft(t *testing.T) {
	grid := makeGrid(10, 3)
	style := lipgloss.NewStyle()

	DrawEdge(grid, 8, 1, 1, 1, style)

	chars := gridChars(grid)
	assert.Equal(t, charArrowLeft, chars[1][1])
	for x := 2; x <= 7; x++ {
		assert.Equal(t, charHorizontal, chars[1][x], "char at x=%d", x)
	}
}

func TestDrawEdgeVerticalDown(t *testing.T) {
	grid := makeGrid(5, 8)
	style := lipgloss.NewStyle()

	DrawEdge(grid, 2, 1, 2, 6, style)

	chars := gridChars(grid)
	assert.Equal(t, ' ', chars[1][2]) // start skipped
	for y := 2; y <= 5; y++ {
		assert.Equal(t, charVertical, chars[y][2], "char at y=%d", y)
	}
	assert.Equal(t, charArrowDown, chars[6][2])
}

func TestDrawEdgeVerticalUp(t *testing.T) {
	grid := makeGrid(5, 8)
	style := lipgloss.NewStyle()

	DrawEdge(grid, 2, 6, 2, 1, style)

	chars := gridChars(grid)
	assert.Equal(t, charArrowUp, chars[1][2])
}

func TestDrawEdgeDiagonalDownRight(t *testing.T) {
	grid := makeGrid(12, 10)
	style := lipgloss.NewStyle()

	// dx > dy so horizontal arrow is expected.
	DrawEdge(grid, 1, 1, 9, 7, style)

	chars := gridChars(grid)
	assert.Equal(t, charArrowRight, chars[7][9], "end should be right arrow")

	// Middle should be diagonal-down characters.
	hasDiag := false
	for y := 2; y < 7; y++ {
		for x := 2; x < 9; x++ {
			if chars[y][x] == charDiagDown {
				hasDiag = true
			}
		}
	}
	assert.True(t, hasDiag, "diagonal line should contain ╲ chars")
}

func TestDrawEdgeDiagonalUpRight(t *testing.T) {
	grid := makeGrid(12, 10)
	style := lipgloss.NewStyle()

	// dx > dy so horizontal arrow is expected.
	DrawEdge(grid, 1, 7, 9, 1, style)

	chars := gridChars(grid)
	assert.Equal(t, charArrowRight, chars[1][9], "end should be right arrow")

	hasDiag := false
	for y := 2; y < 7; y++ {
		for x := 2; x < 9; x++ {
			if chars[y][x] == charDiagUp {
				hasDiag = true
			}
		}
	}
	assert.True(t, hasDiag, "diagonal line should contain ╱ chars")
}

func TestDrawEdgeCrossing(t *testing.T) {
	grid := makeGrid(10, 10)
	style := lipgloss.NewStyle()

	// Draw horizontal line through (5, 5).
	DrawEdge(grid, 1, 5, 9, 5, style)
	// Draw vertical line through (5, 5).
	DrawEdge(grid, 5, 1, 5, 9, style)

	chars := gridChars(grid)
	assert.Equal(t, charCrossing, chars[5][5], "intersection should be crossing character")
}

func TestDrawEdgeEmptyGrid(t *testing.T) {
	// Should not panic.
	DrawEdge(nil, 0, 0, 5, 5, lipgloss.NewStyle())

	grid := makeGrid(0, 0)
	DrawEdge(grid, 0, 0, 5, 5, lipgloss.NewStyle())
}

func TestDrawEdgeOutOfBounds(t *testing.T) {
	grid := makeGrid(5, 5)
	style := lipgloss.NewStyle()

	// Should not panic even with out-of-bounds coords.
	DrawEdge(grid, -1, -1, 10, 10, style)

	// Some in-bounds cells should still be drawn.
	found := false
	for y := range grid {
		for x := range grid[y] {
			if grid[y][x].Char != ' ' {
				found = true
			}
		}
	}
	assert.True(t, found, "should draw at least some in-bounds cells")
}

func TestDrawEdgeSamePoint(t *testing.T) {
	grid := makeGrid(5, 5)
	style := lipgloss.NewStyle()

	// Start and end are the same point.
	DrawEdge(grid, 2, 2, 2, 2, style)

	// The arrow is placed at destination but start is skipped; since they're
	// the same point, the arrow should be present.
	chars := gridChars(grid)
	assert.NotEqual(t, ' ', chars[2][2])
}

func TestBresenhamCoversEndpoints(t *testing.T) {
	var points [][2]int
	bresenham(0, 0, 5, 3, func(x, y int, first, last bool) {
		points = append(points, [2]int{x, y})
	})

	assert.Equal(t, [2]int{0, 0}, points[0], "should start at origin")
	assert.Equal(t, [2]int{5, 3}, points[len(points)-1], "should end at destination")
}

func TestIsLineChar(t *testing.T) {
	assert.True(t, isLineChar(charHorizontal))
	assert.True(t, isLineChar(charVertical))
	assert.True(t, isLineChar(charCrossing))
	assert.True(t, isLineChar(charArrowRight))
	assert.False(t, isLineChar('A'))
	assert.False(t, isLineChar(' '))
	assert.False(t, isLineChar(0))
}

func TestArrowChar(t *testing.T) {
	assert.Equal(t, charArrowRight, arrowChar(5, 0))
	assert.Equal(t, charArrowLeft, arrowChar(-5, 0))
	assert.Equal(t, charArrowDown, arrowChar(0, 5))
	assert.Equal(t, charArrowUp, arrowChar(0, -5))
	assert.Equal(t, charArrowRight, arrowChar(5, 2))
	assert.Equal(t, charArrowDown, arrowChar(2, 5))
}
