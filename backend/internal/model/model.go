package model

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// View defines a view region in the template
type View struct {
	Name     string  `json:"name"`               // front, rear, left, right, top
	X        int     `json:"x"`                  // position in template
	Y        int     `json:"y"`
	W        int     `json:"w"`                  // width
	H        int     `json:"h"`                  // height
	Rotation float64 `json:"rotation,omitempty"` // degrees
	FlipH    bool    `json:"flip_h,omitempty"`   // mirror horizontally
	GapMatch string  `json:"gap_match,omitempty"`// edge color matching direction
	Skip     bool    `json:"skip,omitempty"`     // skip this region (label area etc)
}

// VehicleModel represents a Tesla vehicle wrap template
type VehicleModel struct {
	ID               string
	DisplayName      string
	TemplateWidth    int
	TemplateHeight   int
	TemplatePath     string
	VehicleImagePath string
	Views            []View
}

// ViewNames returns the names of all non-skipped views
func (m *VehicleModel) ViewNames() []string {
	var names []string
	for _, v := range m.Views {
		if !v.Skip {
			names = append(names, v.Name)
		}
	}
	return names
}

// ViewMappingJSON represents the JSON structure for view mappings
type ViewMappingJSON struct {
	Width  int        `json:"width"`
	Height int        `json:"height"`
	Views  []View     `json:"views"`
}

// Registry manages all vehicle models
type Registry struct {
	modelsDir string
	models    map[string]*VehicleModel
}

// NewRegistry creates a new registry
func NewRegistry(modelsDir string) *Registry {
	r := &Registry{
		modelsDir: modelsDir,
		models:    make(map[string]*VehicleModel),
	}
	if modelsDir != "" {
		r.Reload()
	}
	return r
}

// SetModelsDir updates the models directory
func (r *Registry) SetModelsDir(dir string) {
	r.modelsDir = dir
}

// Reload rescans the models directory
func (r *Registry) Reload() {
	if r.modelsDir == "" {
		return
	}

	entries, err := os.ReadDir(r.modelsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		m, err := loadModel(entry.Name(), filepath.Join(r.modelsDir, entry.Name()))
		if err == nil {
			r.models[entry.Name()] = m
		}
	}
}

// Get returns a model by ID
func (r *Registry) Get(id string) (*VehicleModel, bool) {
	m, ok := r.models[id]
	return m, ok
}

// List returns all models sorted by ID
func (r *Registry) List() []*VehicleModel {
	result := make([]*VehicleModel, 0, len(r.models))
	for _, m := range r.models {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// loadModel loads a vehicle model from a directory
func loadModel(id, dir string) (*VehicleModel, error) {
	templatePath := filepath.Join(dir, "template.png")
	vehicleImagePath := filepath.Join(dir, "vehicle_image.png")

	// Check template exists
	templateFile, err := os.Open(templatePath)
	if err != nil {
		return nil, fmt.Errorf("template not found for %s: %w", id, err)
	}
	defer templateFile.Close()

	// Decode template to get dimensions
	templateImg, _, err := image.Decode(templateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to decode template for %s: %w", id, err)
	}

	bounds := templateImg.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Check for vehicle_image
	if _, err := os.Stat(vehicleImagePath); os.IsNotExist(err) {
		vehicleImagePath = ""
	}

	m := &VehicleModel{
		ID:               id,
		DisplayName:      displayName(id),
		TemplateWidth:    w,
		TemplateHeight:   h,
		TemplatePath:     templatePath,
		VehicleImagePath: vehicleImagePath,
	}

	// Try to use pre-computed view mappings from JSON cache
	if vm, ok := getViewMappingsForModel(id); ok {
		var views []View
		for _, v := range vm.Views {
			if !v.Skip {
				views = append(views, v)
			}
		}
		if len(views) > 0 {
			m.Views = views
			return m, nil
		}
	}

	// Fallback: try loading from view_mappings.json in parent directory
	mappingsPath := filepath.Join(filepath.Dir(dir), "view_mappings.json")
	if data, err := os.ReadFile(mappingsPath); err == nil {
		var allMappings map[string]ViewMappingJSON
		if err := json.Unmarshal(data, &allMappings); err == nil {
			if vm, ok := allMappings[id]; ok {
				// Filter out skipped views
				var views []View
				for _, v := range vm.Views {
					if !v.Skip {
						views = append(views, v)
					}
				}
				m.Views = views
				return m, nil
			}
		}
	}

	// Fallback: auto-detect view regions
	views := detectViewRegions(templateImg)
	m.Views = views

	return m, nil
}

// displayName converts an ID like "model3-2024-base" to "Model 3 (2024+) Standard & Premium"
func displayName(id string) string {
	parts := strings.Split(id, "-")
	if len(parts) == 0 {
		return id
	}

	var modelName, year, variant string

	for i, p := range parts {
		if p == "2024" || p == "2025" {
			year = p
			continue
		}
		if p == "cybertruck" {
			modelName = "Cybertruck"
		} else if p == "model3" {
			if i < len(parts)-1 && parts[i+1] == "2024" {
				continue
			}
			modelName = "Model 3"
		} else if p == "models" {
			modelName = "Model S"
		} else if p == "modelx" {
			modelName = "Model X"
		} else if p == "modely" {
			if i < len(parts)-1 && (parts[i+1] == "2025" || parts[i+1] == "l") {
				continue
			}
			modelName = "Model Y"
		} else if p == "modely-l" || p == "l" {
			modelName = "Model Y L"
		} else if p == "plaid" {
			variant = "Plaid"
		} else if p == "base" {
			variant = "Standard"
		} else if p == "performance" {
			variant = "Performance"
		} else if p == "premium" {
			variant = "Premium"
		} else if modelName == "" {
			modelName = strings.Title(p)
		}
	}

	result := modelName
	if year != "" {
		result += fmt.Sprintf(" (%s+)", year)
	}
	if variant != "" && !strings.Contains(result, variant) {
		result += " " + variant
	}
	return strings.TrimSpace(strings.ReplaceAll(result, "  ", " "))
}

// detectViewRegions auto-detects white view regions in the template
func detectViewRegions(img image.Image) []View {
	bounds := img.Bounds()

	// Find all white rectangle regions
	regions := findWhiteRegions(img)

	// Remove small regions (likely text labels)
	var views []View
	for _, r := range regions {
		if r.W >= 80 && r.H >= 80 {
			views = append(views, r)
		}
	}

	// Sort by position
	sort.Slice(views, func(i, j int) bool {
		if abs(views[i].Y-views[j].Y) < 20 {
			return views[i].X < views[j].X
		}
		return views[i].Y < views[j].Y
	})

	// Name the views based on position
	nameViews(views, bounds.Dx(), bounds.Dy())

	return views
}

func findWhiteRegions(img image.Image) []View {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	visited := make(map[[2]int]bool)
	var regions []View

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if visited[[2]int{x, y}] {
				continue
			}

			r, g, b, a := img.At(x, y).RGBA()
			isWhite := a > 32768 && r > 50000 && g > 50000 && b > 50000
			if !isWhite {
				visited[[2]int{x, y}] = true
				continue
			}

			// BFS flood-fill
			minX, maxX := x, x
			minY, maxY := y, y
			stack := [][2]int{{x, y}}

			for len(stack) > 0 {
				pos := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				cx, cy := pos[0], pos[1]

				if visited[[2]int{cx, cy}] {
					continue
				}
				visited[[2]int{cx, cy}] = true

				if cx < minX { minX = cx }
				if cx > maxX { maxX = cx }
				if cy < minY { minY = cy }
				if cy > maxY { maxY = cy }

				for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
					nx, ny := cx+d[0], cy+d[1]
					if nx < 0 || nx >= w || ny < 0 || ny >= h { continue }
					if visited[[2]int{nx, ny}] { continue }
					nr, ng, nb, na := img.At(nx, ny).RGBA()
					if na > 32768 && nr > 50000 && ng > 50000 && nb > 50000 {
						stack = append(stack, [2]int{nx, ny})
					}
				}
			}

			rw := maxX - minX + 1
			rh := maxY - minY + 1
			regions = append(regions, View{
				X: minX, Y: minY, W: rw, H: rh,
			})
		}
	}

	return regions
}

// nameViews assigns view names based on position analysis
func nameViews(views []View, tmplW, tmplH int) {
	if len(views) == 0 {
		return
	}

	// Calculate mid-point of template
	midY := tmplH / 2

	// Separate into rows based on vertical position
	var topRow, bottomRow []*View
	for i := range views {
		if views[i].Y+views[i].H/2 < midY {
			topRow = append(topRow, &views[i])
		} else {
			bottomRow = append(bottomRow, &views[i])
		}
	}

	// Sort each row by X
	sort.Slice(topRow, func(i, j int) bool { return topRow[i].X < topRow[j].X })
	sort.Slice(bottomRow, func(i, j int) bool { return bottomRow[i].X < bottomRow[j].X })

	// Assign names: typical Tesla template layout
	// Top row: front, rear, (possibly top)
	// Bottom row: left, right, top (if top not in top row)
	if len(topRow) >= 1 {
		topRow[0].Name = "front"
	}
	if len(topRow) >= 2 {
		topRow[1].Name = "rear"
	}
	if len(topRow) >= 3 {
		topRow[2].Name = "top"
	}

	for i, v := range bottomRow {
		switch i {
		case 0: v.Name = "left"
		case 1: v.Name = "right"
		case 2: v.Name = "top"
		}
	}
}

func abs(x int) int {
	if x < 0 { return -x }
	return x
}

// LoadViewMappings loads pre-computed view coordinates from a JSON file
// This overrides auto-detected view regions for known vehicle models
func LoadViewMappings(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err // file not found, use auto-detection
	}

	var allMappings map[string]ViewMappingJSON
	if err := json.Unmarshal(data, &allMappings); err != nil {
		return err
	}

	// Store mappings for later use during model loading
	viewMappingsCache = allMappings
	return nil
}

// viewMappingsCache holds pre-computed view coordinates
var viewMappingsCache map[string]ViewMappingJSON

// getViewMappingsForModel returns cached mappings for a model ID
func getViewMappingsForModel(id string) (ViewMappingJSON, bool) {
	if viewMappingsCache == nil {
		return ViewMappingJSON{}, false
	}
	m, ok := viewMappingsCache[id]
	return m, ok
}
