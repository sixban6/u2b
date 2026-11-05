package downloader

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sixban6/u2b/config"
)

// DefaultQualitySelector implements quality selection logic
type DefaultQualitySelector struct{}

// NewQualitySelector creates a new quality selector
func NewQualitySelector() QualitySelector {
	return &DefaultQualitySelector{}
}

// SelectBestQuality selects the best available quality based on preferences
func (q *DefaultQualitySelector) SelectBestQuality(available []string, preferred string, fallback string) (string, error) {
	if len(available) == 0 {
		return "", fmt.Errorf("no qualities available")
	}

	preferred = config.NormalizeQuality(preferred)
	fallback = config.NormalizeQuality(fallback)

	// Parse preferred quality
	preferredHeight, err := q.ParseQuality(preferred)
	if err != nil {
		return "", fmt.Errorf("invalid preferred quality: %w", err)
	}

	// Parse fallback quality
	fallbackHeight, err := q.ParseQuality(fallback)
	if err != nil {
		return "", fmt.Errorf("invalid fallback quality: %w", err)
	}
	
	// Convert available qualities to heights for comparison
	availableHeights := make(map[int]string)
	var heights []int
	
	for _, quality := range available {
		height, err := q.parseQualityToHeight(quality)
		if err != nil {
			continue // skip invalid qualities
		}
		availableHeights[height] = quality
		heights = append(heights, height)
	}
	
	if len(heights) == 0 {
		return "", fmt.Errorf("no valid qualities found in available list")
	}
	
	// Find best match for preferred quality
	preferredHeightInt, _ := strconv.Atoi(preferredHeight)
	fallbackHeightInt, _ := strconv.Atoi(fallbackHeight)
	
	// Special cases for "best" and "worst"
	if preferred == config.QualityBest {
		return q.getBestQuality(availableHeights, heights), nil
	}
	if preferred == config.QualityWorst {
		return q.getWorstQuality(availableHeights, heights), nil
	}
	
	// Try to find exact match for preferred quality
	if quality, exists := availableHeights[preferredHeightInt]; exists {
		return quality, nil
	}
	
	// Find closest available quality to preferred
	bestMatch := q.findClosestQuality(heights, preferredHeightInt)
	if bestMatch > 0 {
		return availableHeights[bestMatch], nil
	}
	
	// Try fallback quality
	if quality, exists := availableHeights[fallbackHeightInt]; exists {
		return quality, nil
	}
	
	// Find closest to fallback
	bestMatch = q.findClosestQuality(heights, fallbackHeightInt)
	if bestMatch > 0 {
		return availableHeights[bestMatch], nil
	}
	
	// Return best available as last resort
	return q.getBestQuality(availableHeights, heights), nil
}

// ParseQuality parses and validates quality string
func (q *DefaultQualitySelector) ParseQuality(quality string) (string, error) {
	norm := config.NormalizeQuality(quality)
	if norm == "" {
		return "", fmt.Errorf("quality cannot be empty")
	}

	// Handle special qualities
	if norm == config.QualityBest || norm == config.QualityWorst {
		return norm, nil
	}

	if height, ok := config.QualityToHeight(norm); ok {
		return height, nil
	}

	return "", fmt.Errorf("unsupported quality format: %s", quality)
}

// parseQualityToHeight converts quality string to height integer
func (q *DefaultQualitySelector) parseQualityToHeight(quality string) (int, error) {
	// Direct height numbers
	if height, err := strconv.Atoi(quality); err == nil {
		return height, nil
	}
	
	// Standard quality names
	qualityMap := map[string]int{
		config.Quality2160p: 2160,
		config.Quality1440p: 1440,
		config.Quality1080p: 1080, 
		config.Quality720p:  720,
		config.Quality480p:  480,
		config.Quality360p:  360,
		config.Quality240p:  240,
		"2160p": 2160,
		"1440p": 1440,
		"1080p": 1080,
		"720p":  720,
		"480p":  480,
		"360p":  360,
		"240p":  240,
	}
	
	if height, exists := qualityMap[quality]; exists {
		return height, nil
	}
	
	// Resolution format (1920x1080)
	if strings.Contains(quality, "x") {
		parts := strings.Split(quality, "x")
		if len(parts) == 2 {
			if height, err := strconv.Atoi(parts[1]); err == nil {
				return height, nil
			}
		}
	}
	
	// Format with quality info (e.g., "1080p - avc1")
	if strings.Contains(quality, "p") {
		parts := strings.Split(quality, "p")
		if len(parts) > 0 {
			if height, err := strconv.Atoi(parts[0]); err == nil {
				return height, nil
			}
		}
	}
	
	return 0, fmt.Errorf("unable to parse quality: %s", quality)
}

// getBestQuality returns the highest quality available
func (q *DefaultQualitySelector) getBestQuality(availableHeights map[int]string, heights []int) string {
	maxHeight := heights[0]
	for _, height := range heights {
		if height > maxHeight {
			maxHeight = height
		}
	}
	return availableHeights[maxHeight]
}

// getWorstQuality returns the lowest quality available
func (q *DefaultQualitySelector) getWorstQuality(availableHeights map[int]string, heights []int) string {
	minHeight := heights[0]
	for _, height := range heights {
		if height < minHeight {
			minHeight = height
		}
	}
	return availableHeights[minHeight]
}

// findClosestQuality finds the closest available quality to target
func (q *DefaultQualitySelector) findClosestQuality(heights []int, target int) int {
	if len(heights) == 0 {
		return 0
	}
	
	closest := heights[0]
	minDiff := abs(heights[0] - target)
	
	for _, height := range heights {
		diff := abs(height - target)
		if diff < minDiff {
			minDiff = diff
			closest = height
		}
	}
	
	return closest
}

// abs returns absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// QualityRanking defines quality preference order (higher is better)
var QualityRanking = map[string]int{
	config.Quality240p:  1,
	config.Quality360p:  2,
	config.Quality480p:  3,
	config.Quality720p:  4,
	config.Quality1080p: 5,
	config.Quality1440p: 6,
	config.Quality2160p: 7,
}

// CompareQualities compares two qualities and returns -1, 0, or 1
func (q *DefaultQualitySelector) CompareQualities(quality1, quality2 string) int {
	height1, err1 := q.parseQualityToHeight(quality1)
	height2, err2 := q.parseQualityToHeight(quality2)
	
	if err1 != nil || err2 != nil {
		// Fall back to string comparison if parsing fails
		if quality1 < quality2 {
			return -1
		} else if quality1 > quality2 {
			return 1
		}
		return 0
	}
	
	if height1 < height2 {
		return -1
	} else if height1 > height2 {
		return 1
	}
	return 0
}

// GetQualityInfo returns human-readable information about a quality
func (q *DefaultQualitySelector) GetQualityInfo(quality string) string {
	height, err := q.parseQualityToHeight(quality)
	if err != nil {
		return fmt.Sprintf("Quality: %s", quality)
	}
	
	switch height {
	case 2160:
		return "4K Ultra HD (2160p)"
	case 1440:
		return "2K Quad HD (1440p)"
	case 1080:
		return "Full HD (1080p)"
	case 720:
		return "HD (720p)"
	case 480:
		return "SD (480p)"
	case 360:
		return "Low Quality (360p)"
	case 240:
		return "Very Low Quality (240p)"
	default:
		return fmt.Sprintf("Custom Quality (%dp)", height)
	}
}
