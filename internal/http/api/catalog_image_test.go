package api

import (
	"bytes"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/e-commerce-be/internal/modules/catalog"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/gin-gonic/gin"
)

func TestImageSearchRouteRejectsNonImageUpload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	CatalogRoutes(router.Group("/api/v1"), func(c *gin.Context) { c.Next() }, catalog.New(shared.New(nil)))
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("image", "not-image.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("not an image")); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/products/search-by-image", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
}

func TestImageSearchRouteAcceptsImageAndKeepsDisabledFeatureUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	CatalogRoutes(router.Group("/api/v1"), func(c *gin.Context) { c.Next() }, catalog.New(shared.New(nil)))
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p4sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("image", "query.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/products/search-by-image", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusServiceUnavailable, resp.Body.String())
	}
}
