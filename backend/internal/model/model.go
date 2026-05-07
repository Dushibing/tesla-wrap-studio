package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

	templateOnce  sync.Once
	templateImage *image.NRGBA
	templateErr   error
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

// TemplateImage returns a cached decoded template image.
func (m *VehicleModel) TemplateImage() (*image.NRGBA, error) {
	m.templateOnce.Do(func() {
		f, err := os.Open(m.TemplatePath)
		if err != nil {
			m.templateErr = err
			return
		}
		defer f.Close()

		img, err := png.Decode(f)
		if err != nil {
			m.templateErr = err
			return
		}

		bounds := img.Bounds()
		out := image.NewNRGBA(bounds)
		draw.Draw(out, bounds, img, bounds.Min, draw.Src)
		m.templateImage = out
	})

	return m.templateImage, m.templateErr
}

// ViewMappingJSON represents the JSON structure for view mappings
type ViewMappingJSON struct {
	Width  int        `json:"width"`
	Height int        `json:"height"`
	Views  []View     `json:"views"`
}

// Registry manages all vehicle models
type Registry struct {
	mu        sync.RWMutex
	modelsDir string
	models    map[string]*VehicleModel
}

// NewRegistry creates a new registry
func NewRegistry(modelsDir string) *Registry {
	return &Registry{
		modelsDir: modelsDir,
		models:    make(map[string]*VehicleModel),
	}
}

// SetModelsDir updates the models directory
func (r *Registry) SetModelsDir(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelsDir = dir
}

// Reload rescans the models directory
func (r *Registry) Reload() error {
	r.mu.RLock()
	modelsDir := r.modelsDir
	r.mu.RUnlock()

	if modelsDir == "" {
		return nil
	}

	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		return err
	}

	models := make(map[string]*VehicleModel, len(entries))
	var loadErrs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		modelID := entry.Name()
		if !isSafeModelID(modelID) {
			loadErrs = append(loadErrs, fmt.Errorf("skipping invalid model directory %q", modelID))
			continue
		}

		m, err := loadModel(modelID, filepath.Join(modelsDir, modelID))
		if err != nil {
			loadErrs = append(loadErrs, err)
			continue
		}
		models[modelID] = m
	}

	r.mu.Lock()
	r.models = models
	r.mu.Unlock()

	return errors.Join(loadErrs...)
}

// Get returns a model by ID
func (r *Registry) Get(id string) (*VehicleModel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.models[id]
	return m, ok
}

// List returns all models sorted by ID
func (r *Registry) List() []*VehicleModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	if !isSafeModelID(id) {
		return nil, fmt.Errorf("invalid model ID %q", id)
	}

	templatePath := filepath.Join(dir, "template.png")
	vehicleImagePath := filepath.Join(dir, "vehicle_image.png")

	// Check template exists
	templateFile, err := os.Open(templatePath)
	if err != nil {
		return nil, fmt.Errorf("template not found for %s: %w", id, err)
	}
	defer templateFile.Close()

	// Decode template to get dimensions
	templateConfig, err := png.DecodeConfig(templateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to decode template for %s: %w", id, err)
	}

	w := templateConfig.Width
	h := templateConfig.Height

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

	// Fallback: auto-detect view regions
	templateImg, err := m.TemplateImage()
	if err != nil {
		return nil, fmt.Errorf("failed to load template for %s: %w", id, err)
	}
	views := detectViewRegions(templateImg)
	m.Views = views

	return m, nil
}

func isSafeModelID(id string) bool {
	if id == "" || strings.Contains(id, "..") {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
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

	viewMappingsMu.Lock()
	viewMappingsCache = allMappings
	viewMappingsMu.Unlock()
	return nil
}

// viewMappingsCache holds pre-computed view coordinates
var viewMappingsCache map[string]ViewMappingJSON
var viewMappingsMu sync.RWMutex

// getViewMappingsForModel returns cached mappings for a model ID
func getViewMappingsForModel(id string) (ViewMappingJSON, bool) {
	viewMappingsMu.RLock()
	defer viewMappingsMu.RUnlock()
	if viewMappingsCache == nil {
		return ViewMappingJSON{}, false
	}
	m, ok := viewMappingsCache[id]
	return m, ok
}
