package cmd

import (
	"fmt"
	"math"
	"os"

	"github.com/spf13/cobra"
)

// TileVector2D coordinate struct
type TileVector2D struct {
	X, Y int
}

// raycircCmd command definition
var raycircCmd = &cobra.Command{
	Use:   "raycirc",
	Short: "Calculate optimal ray count for full circle coverage",
	Long:  "Input a radius to automatically calculate the minimal optimal ray count that fully covers the circle grid without gaps.",
	Run:   runRayCirc,
}

func init() {
	rootCmd.AddCommand(raycircCmd)
	raycircCmd.Flags().IntP("radius", "r", 0, "Maximum circle radius (required)")
	_ = raycircCmd.MarkFlagRequired("radius")
}

func runRayCirc(cmd *cobra.Command, args []string) {
	maxRadius, _ := cmd.Flags().GetInt("radius")
	if maxRadius <= 0 {
		fmt.Println("Error: radius must be greater than 0")
		os.Exit(1)
	}

	fmt.Printf("===== Input Radius: %d =====\n", maxRadius)
	targetSet := generateCircleTileSet(maxRadius)
	fmt.Printf("Grid points to cover: %d\n", len(targetSet))

	fmt.Println("\nCalculating minimal optimal ray count...")
	bestRayCount, bestAngleStep, coveredSet := findMinOptimalRays(maxRadius, targetSet)

	fmt.Println("\n===== Calculation Complete =====")
	fmt.Printf("✅ Optimal ray count: %d\n", bestRayCount)
	fmt.Printf("✅ Angle step: %.4f degrees\n", bestAngleStep)
	fmt.Printf("✅ Covered: %d/%d\n", len(coveredSet), len(targetSet))
	fmt.Println("✅ Full coverage, no gaps!")
}

// generateCircleTileSet generates all grid points inside the circle, excluding center (0,0)
func generateCircleTileSet(r int) map[TileVector2D]bool {
	set := make(map[TileVector2D]bool)
	rSq := r * r
	for dx := -r; dx <= r; dx++ {
		for dy := -r; dy <= r; dy++ {
			if dx == 0 && dy == 0 {
				continue
			}
			if dx*dx+dy*dy <= rSq {
				set[TileVector2D{X: dx, Y: dy}] = true
			}
		}
	}
	return set
}

// findMinOptimalRays finds the minimal ray count starting from 4
func findMinOptimalRays(maxRadius int, targetSet map[TileVector2D]bool) (int, float64, map[TileVector2D]bool) {
	for tryRay := 4; ; tryRay++ {
		angleStep := 360.0 / float64(tryRay)
		covered := simulateRayTraverse(maxRadius, tryRay, angleStep)
		full := isFullCovered(targetSet, covered)

		status := "❌ INCOMPLETE"
		if full {
			status = "✅ FULL COVERAGE"
		}

		fmt.Printf("[TEST] Ray: %-5d Angle: %6.2f°  Covered: %d/%d  %s\n",
			tryRay, angleStep, len(covered), len(targetSet), status)

		if full {
			return tryRay, angleStep, covered
		}
	}
}

// simulateRayTraverse strictly matches C++ LightPropagationTraverser logic
func simulateRayTraverse(maxRadius, rayCount int, angleStep float64) map[TileVector2D]bool {
	visited := make(map[TileVector2D]bool)
	maxRadSq := maxRadius * maxRadius

	for rayIdx := 0; rayIdx < rayCount; rayIdx++ {
		angleDeg := float64(rayIdx) * angleStep
		angleRad := angleDeg * math.Pi / 180.0
		dirX := math.Cos(angleRad)
		dirY := math.Sin(angleRad)

		curX, curY := 0.0, 0.0
		for step := 0; step < maxRadius; step++ {
			curX += dirX
			curY += dirY
			nextX := int(math.Round(curX))
			nextY := int(math.Round(curY))
			tile := TileVector2D{X: nextX, Y: nextY}

			dx, dy := tile.X, tile.Y
			if dx*dx+dy*dy > maxRadSq {
				break
			}

			visited[tile] = true
		}
	}
	return visited
}

// isFullCovered checks if all target points are covered
func isFullCovered(target, covered map[TileVector2D]bool) bool {
	for t := range target {
		if !covered[t] {
			return false
		}
	}
	return true
}
