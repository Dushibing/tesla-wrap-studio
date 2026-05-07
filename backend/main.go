package main

import (
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"tesla-wrap-studio/internal/model"
	"tesla-wrap-studio/internal/renderer"
)

func main() {
	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}

	// Load view mappings BEFORE creating registry
	model.LoadViewMappings("view_mappings.json")

	registry := model.NewRegistry("templates")
	rend := renderer.New(registry)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/models", handleModels(registry))
	mux.HandleFunc("/api/models/", handleModelDetail(registry))
	mux.HandleFunc("/api/render", handleRender(rend, registry))

	fs := http.FileServer(http.Dir("frontend/dist"))
	mux.Handle("/", fs)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("🚗 Tesla Wrap Studio started on http://localhost%s\n", addr)
	log.Printf("📋 API: http://localhost%s/api/models\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleModels(registry *model.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		models := registry.List()
		result := make([]map[string]interface{}, 0, len(models))
		for _, m := range models {
			result = append(result, map[string]interface{}{
				"id":          m.ID,
				"name":        m.DisplayName,
				"width":       m.TemplateWidth,
				"height":      m.TemplateHeight,
				"views":       m.ViewNames(),
				"views_count": len(m.Views),
			})
		}
		json.NewEncoder(w).Encode(result)
	}
}

func handleModelDetail(registry *model.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/models/")
		id = strings.TrimSuffix(id, "/template")
		id = strings.TrimSuffix(id, "/vehicle_image")

		m, ok := registry.Get(id)
		if !ok {
			http.Error(w, fmt.Sprintf("model %s not found", id), http.StatusNotFound)
			return
		}

		if strings.HasSuffix(r.URL.Path, "/template") {
			w.Header().Set("Content-Type", "image/png")
			http.ServeFile(w, r, m.TemplatePath)
			return
		}

		if strings.HasSuffix(r.URL.Path, "/vehicle_image") {
			w.Header().Set("Content-Type", "image/png")
			http.ServeFile(w, r, m.VehicleImagePath)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          m.ID,
			"name":        m.DisplayName,
			"width":       m.TemplateWidth,
			"height":      m.TemplateHeight,
			"views":       m.Views,
			"views_count": len(m.Views),
		})
	}
}

func handleRender(rend *renderer.Renderer, registry *model.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		err := r.ParseMultipartForm(32 << 20)
		if err != nil {
			http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		modelID := r.FormValue("model_id")
		if modelID == "" {
			http.Error(w, "model_id is required", http.StatusBadRequest)
			return
		}

		m, ok := registry.Get(modelID)
		if !ok {
			http.Error(w, fmt.Sprintf("model %s not found", modelID), http.StatusNotFound)
			return
		}

		images := make(map[string]io.Reader)
		for _, view := range m.Views {
			file, _, err := r.FormFile(view.Name)
			if err != nil {
				continue
			}
			defer file.Close()
			images[view.Name] = file
		}

		if len(images) == 0 {
			http.Error(w, "no images uploaded", http.StatusBadRequest)
			return
		}

		// Parse adjustments from JSON
		var opts renderer.RenderOptions
		if adjJSON := r.FormValue("adjustments"); adjJSON != "" {
			if err := json.Unmarshal([]byte(adjJSON), &opts.Adjustments); err != nil {
				log.Printf("Warning: failed to parse adjustments JSON: %v", err)
			}
		}

		result, err := rend.Render(m, images, opts)
		if err != nil {
			http.Error(w, "render failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_wrap.png"`, modelID))
		png.Encode(w, result)
	}
}
