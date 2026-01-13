package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// @title           Hostus Taxonomy API
// @version         0.1.0
// @description     A high-performance taxonomy gateway for vascular plant autosuggest

// @contact.name   API Support
// @contact.url    https://github.com/jobrunner/hostus

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:8080
// @BasePath  /

type OpenAPISpec struct {
	OpenAPI    string                 `json:"openapi"`
	Info       OpenAPIInfo            `json:"info"`
	Servers    []OpenAPIServer        `json:"servers,omitempty"`
	Paths      map[string]interface{} `json:"paths"`
	Components OpenAPIComponents      `json:"components"`
}

type OpenAPIInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type OpenAPIComponents struct {
	Schemas map[string]interface{} `json:"schemas"`
}

func getVersion() string {
	data, err := os.ReadFile("VERSION")
	if err != nil {
		return "0.1.0"
	}
	return strings.TrimSpace(string(data))
}

func GenerateOpenAPISpec() OpenAPISpec {
	return OpenAPISpec{
		OpenAPI: "3.0.3",
		Info: OpenAPIInfo{
			Title:       "Hostus Taxonomy API",
			Description: "A high-performance taxonomy gateway for vascular plant autosuggest. Proxies GBIF API and groups synonyms under accepted taxa.",
			Version:     getVersion(),
		},
		Paths: map[string]interface{}{
			"/api/v1/taxa/suggest": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Search taxa for autosuggest",
					"description": "Returns vascular plant taxa matching the query, grouped by accepted name with synonyms embedded",
					"tags":        []string{"taxa"},
					"parameters": []map[string]interface{}{
						{
							"name":        "q",
							"in":          "query",
							"required":    true,
							"description": "Search query (minimum 3 characters)",
							"schema":      map[string]string{"type": "string", "minLength": "3"},
						},
						{
							"name":        "limit",
							"in":          "query",
							"required":    false,
							"description": "Maximum number of accepted taxa to return (default: 20, max: 100)",
							"schema":      map[string]interface{}{"type": "integer", "default": 20, "maximum": 100},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Successful response",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":  "array",
										"items": map[string]string{"$ref": "#/components/schemas/TaxonSuggestion"},
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Invalid query parameter",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]string{"$ref": "#/components/schemas/ErrorResponse"},
								},
							},
						},
						"429": map[string]interface{}{
							"description": "Rate limit exceeded",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]string{"$ref": "#/components/schemas/ErrorResponse"},
								},
							},
						},
						"502": map[string]interface{}{
							"description": "GBIF service unavailable",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]string{"$ref": "#/components/schemas/ErrorResponse"},
								},
							},
						},
						"503": map[string]interface{}{
							"description": "Upstream overloaded (load shedding active)",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]string{"$ref": "#/components/schemas/ErrorResponse"},
								},
							},
						},
						"504": map[string]interface{}{
							"description": "GBIF request timeout",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]string{"$ref": "#/components/schemas/ErrorResponse"},
								},
							},
						},
					},
				},
			},
			"/health": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Health check",
					"description": "Returns OK if service is healthy",
					"tags":        []string{"health"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Service is healthy",
							"content": map[string]interface{}{
								"text/plain": map[string]interface{}{
									"schema": map[string]string{"type": "string", "example": "OK"},
								},
							},
						},
					},
				},
			},
		},
		Components: OpenAPIComponents{
			Schemas: map[string]interface{}{
				"TaxonSuggestion": map[string]interface{}{
					"type": "object",
					"required": []string{"acceptedKey", "acceptedName", "rank", "family"},
					"properties": map[string]interface{}{
						"acceptedKey": map[string]interface{}{
							"type":        "integer",
							"description": "GBIF key of the accepted taxon",
							"example":     2704178,
						},
						"acceptedName": map[string]interface{}{
							"type":        "string",
							"description": "Scientific name of the accepted taxon",
							"example":     "Schoenoplectus lacustris",
						},
						"rank": map[string]interface{}{
							"type":        "string",
							"description": "Taxonomic rank",
							"enum":        []string{"FAMILY", "GENUS", "SPECIES", "SUBSPECIES"},
							"example":     "SPECIES",
						},
						"family": map[string]interface{}{
							"type":        "string",
							"description": "Family name",
							"example":     "Cyperaceae",
						},
						"synonyms": map[string]interface{}{
							"type":        "array",
							"description": "List of synonyms for this taxon",
							"items":       map[string]string{"$ref": "#/components/schemas/Synonym"},
						},
					},
				},
				"Synonym": map[string]interface{}{
					"type": "object",
					"required": []string{"key", "name", "status"},
					"properties": map[string]interface{}{
						"key": map[string]interface{}{
							"type":        "integer",
							"description": "GBIF key of the synonym",
							"example":     5298174,
						},
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Scientific name of the synonym",
							"example":     "Scirpus lacustris",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "Taxonomic status",
							"example":     "SYNONYM",
						},
					},
				},
				"ErrorResponse": map[string]interface{}{
					"type": "object",
					"required": []string{"error"},
					"properties": map[string]interface{}{
						"error": map[string]interface{}{
							"type": "object",
							"required": []string{"code", "message"},
							"properties": map[string]interface{}{
								"code": map[string]interface{}{
									"type":        "string",
									"description": "Error code",
									"enum":        []string{"INVALID_QUERY", "RATE_LIMIT_EXCEEDED", "UPSTREAM_OVERLOADED", "GBIF_TIMEOUT", "GBIF_UNAVAILABLE", "INTERNAL_ERROR"},
									"example":     "INVALID_QUERY",
								},
								"message": map[string]interface{}{
									"type":        "string",
									"description": "Human-readable error message",
									"example":     "Query must be at least 3 characters",
								},
							},
						},
					},
				},
			},
		},
	}
}

func ServeOpenAPI(w http.ResponseWriter, r *http.Request) {
	spec := GenerateOpenAPISpec()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(spec)
}
