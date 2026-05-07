package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tesla-wrap-studio/internal/model"
	"tesla-wrap-studio/internal/renderer"
)

func setupTestRegistry(t *testing.T) (*model.Registry, string) {
	tmpDir, err := os.MkdirTemp("", "tesla-test")
	if err != nil {
		t.Fatal(err)
	}

	// Create a dummy model
	modelDir := filepath.Join(tmpDir, "cybertruck")
	os.MkdirAll(modelDir, 0755)

	// Create dummy template.png
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	f, _ := os.Create(filepath.Join(modelDir, "template.png"))
	png.Encode(f, img)
	f.Close()

	// Create view_mappings.json
	mappings := map[string]interface{}{
		"cybertruck": map[string]interface{}{
			"width":  100,
			"height": 100,
			"views": []map[string]interface{}{
				{"name": "front", "x": 0, "y": 0, "w": 50, "h": 50},
				{"name": "rear", "x": 50, "y": 0, "w": 50, "h": 50},
			},
		},
	}
	mappingPath := filepath.Join(tmpDir, "view_mappings.json")
	mdata, _ := json.Marshal(mappings)
	os.WriteFile(mappingPath, mdata, 0644)

	model.LoadViewMappings(mappingPath)
	registry := model.NewRegistry(tmpDir)
	registry.Reload()

	return registry, tmpDir
}

func TestHandleModels(t *testing.T) {
	registry, tmpDir := setupTestRegistry(t)
	defer os.RemoveAll(tmpDir)

	req, _ := http.NewRequest("GET", "/api/models", nil)
	rr := httptest.NewRecorder()
	handler := handleModels(registry)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var models []map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &models)
	if len(models) < 1 {
		t.Errorf("expected at least 1 model, got %v", len(models))
	}
}

func TestHandleModelDetail(t *testing.T) {
	registry, tmpDir := setupTestRegistry(t)
	defer os.RemoveAll(tmpDir)

	req, _ := http.NewRequest("GET", "/api/models/cybertruck", nil)
	rr := httptest.NewRecorder()
	handler := handleModelDetail(registry)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var m map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &m)
	if m["id"] != "cybertruck" {
		t.Errorf("expected cybertruck, got %v", m["id"])
	}
}

func TestHandleRender(t *testing.T) {
	registry, tmpDir := setupTestRegistry(t)
	defer os.RemoveAll(tmpDir)
	rend := renderer.New()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("model_id", "cybertruck")
	
	// Add a dummy image
	part, _ := writer.CreateFormFile("front", "test.png")
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	png.Encode(part, img)
	writer.Close()

	req, _ := http.NewRequest("POST", "/api/render", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	
	rr := httptest.NewRecorder()
	handler := handleRender(rend, registry)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v. Body: %s", status, http.StatusOK, rr.Body.String())
	}

	if rr.Header().Get("Content-Type") != "image/png" {
		t.Errorf("expected image/png, got %v", rr.Header().Get("Content-Type"))
	}
}
