/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

// ColorConfig defines the structure of .color.toml file
type ColorConfig struct {
	R uint8 `toml:"r"`
	G uint8 `toml:"g"`
	B uint8 `toml:"b"`
	A uint8 `toml:"a"`
}

// ColorMapping defines color replacement rules
type ColorMapping struct {
	FromPath string `toml:"from_path"`
	ToPath   string `toml:"to_path"`
	SetA     *uint8 `toml:"set_a"` // Optional alpha override
}

// TextureVariation defines a single texture generation task
type TextureVariation struct {
	TemplatePath string         `toml:"template_path"`
	OutputPath   string         `toml:"output_path"`
	ColorMapping []ColorMapping `toml:"color_mapping"`
}

// AssetGenConfig top-level config structure
type AssetGenConfig struct {
	TextureVariation []TextureVariation `toml:"texture_variation"`
}

// genTextureCmd represents the genTexture command
var genTextureCmd = &cobra.Command{
	Use:   "genTexture",
	Short: "Generate textures from template images based on TOML config",
	Long: `This command generates game textures by replacing placeholder colors in template images.
It reads configuration from .asset_gen.toml and supports custom color mapping and alpha channel control.`,
	Run: runGenerateTexture,
}

// configFilePath flag for custom config path
var configFilePath string

func init() {
	rootCmd.AddCommand(genTextureCmd)

	// Add config flag: -c/--config
	genTextureCmd.Flags().StringVarP(&configFilePath, "config", "c", ".asset_gen.toml",
		"Path to the asset generation config file (default: .asset_gen.toml in current directory)")
}

// runGenerateTexture main execution logic
func runGenerateTexture(cmd *cobra.Command, args []string) {
	// 1. Validate config file exists
	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		printError("Config file not found: %s", configFilePath)
		return
	}
	printInfo("Loading config file: %s", configFilePath)

	// 2. Parse TOML config
	var config AssetGenConfig
	if _, err := toml.DecodeFile(configFilePath, &config); err != nil {
		printError("Failed to parse config file: %v", err)
		return
	}

	if len(config.TextureVariation) == 0 {
		printInfo("No texture variation tasks found in config")
		return
	}
	printInfo("Found %d texture generation tasks", len(config.TextureVariation))

	// 3. Process all texture variations
	successCount := 0
	for i, variation := range config.TextureVariation {
		printInfo("Processing task %d: %s -> %s", i+1, variation.TemplatePath, variation.OutputPath)
		if err := processSingleVariation(variation); err != nil {
			printError("Failed to process task %d: %v", i+1, err)
		} else {
			successCount++
		}
	}

	// 4. Final result
	printInfo("Generation completed: %d/%d tasks succeeded", successCount, len(config.TextureVariation))
}

// processSingleVariation handle one texture generation task
func processSingleVariation(variation TextureVariation) error {
	// 1. Load template image
	templateImg, err := loadImage(variation.TemplatePath)
	if err != nil {
		return err
	}

	// 2. Create output directory
	if err := os.MkdirAll(filepath.Dir(variation.OutputPath), 0755); err != nil {
		return err
	}

	// 3. Create editable image (RGBA format)
	bounds := templateImg.Bounds()
	rgbaImg := image.NewRGBA(bounds)
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			rgbaImg.Set(x, y, templateImg.At(x, y))
		}
	}

	// 4. Apply all color mappings
	for _, mapping := range variation.ColorMapping {
		if err := applyColorMapping(rgbaImg, mapping); err != nil {
			return err
		}
	}

	// 5. Save generated image
	return saveImage(rgbaImg, variation.OutputPath)
}

// applyColorMapping replace source color with target color + optional alpha
func applyColorMapping(img *image.RGBA, mapping ColorMapping) error {
	// Load source (placeholder) color
	fromColor, err := loadColor(mapping.FromPath)
	if err != nil {
		return err
	}

	// Load target color
	toColor, err := loadColor(mapping.ToPath)
	if err != nil {
		return err
	}

	// Override alpha if set_a is specified
	if mapping.SetA != nil {
		toColor.A = *mapping.SetA
	}

	// Replace all matching pixels
	bounds := img.Bounds()
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			currentColor := img.RGBAAt(x, y)
			if currentColor == fromColor {
				img.SetRGBA(x, y, toColor)
			}
		}
	}

	return nil
}

// loadColor read and parse .color.toml file
func loadColor(path string) (color.RGBA, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return color.RGBA{}, err
	}

	var config ColorConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return color.RGBA{}, err
	}

	return color.RGBA{
		R: config.R,
		G: config.G,
		B: config.B,
		A: config.A,
	}, nil
}

// loadImage read PNG image
func loadImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	return img, err
}

// saveImage write RGBA image to PNG file
func saveImage(img *image.RGBA, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}

// printf wrapper for console output
func printf(format string, args ...interface{}) {
	_, _ = os.Stdout.WriteString(fmt.Sprintf(format, args...))
}
