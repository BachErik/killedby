package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// Setup
	githubUsername = "BachErik"
	githubRepository = "killedby.json"
	os.Setenv("GITHUB_USERNAME", githubUsername)
	os.Setenv("GITHUB_REPOSITORY", githubRepository)
	code := m.Run()
	// Teardown
	os.Unsetenv("GITHUB_USERNAME")
	os.Unsetenv("GITHUB_REPOSITORY")
	os.Exit(code)
}

func TestFetchJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"testKey":"testValue"}`))
	}))
	defer server.Close()

	var data map[string]string
	err := fetchJSON(server.URL, "test.json", &data)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if data["testKey"] != "testValue" {
		t.Errorf("Expected testValue, got %v", data["testKey"])
	}
}

func TestIsFutureDate(t *testing.T) {
	futureDate := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
	pastDate := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")

	if !isFutureDate(futureDate) {
		t.Errorf("Expected true, got false")
	}
	if isFutureDate(pastDate) {
		t.Errorf("Expected false, got true")
	}
}

func TestIndexHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()

	updateRepoCache()

	handler := http.HandlerFunc(indexHandler)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestCompanyHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "/company/testCompany", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()

	updateRepoCache()

	handler := http.HandlerFunc(companyHandler)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestGenerateSimpleImage(t *testing.T) {
	img := generateSimpleImage("Test Text", "", "Test Description")
	if img == nil {
		t.Fatalf("Expected non-nil image, got nil")
	}

	// Check image dimensions
	if img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 628 {
		t.Errorf("Expected image dimensions 1200x628, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestGenerateDetailedImage(t *testing.T) {
	project := Project{
		Name:        "Test Project",
		Description: "Test Description",
		Company:     "Test Company",
		DateOpen:    "2020-01-01",
		DateClose:   "2021-01-01",
		Type:        "Test Type",
	}

	img := generateDetailedImage(project)
	if img == nil {
		t.Fatalf("Expected non-nil image, got nil")
	}

	// Check image dimensions
	if img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 628 {
		t.Errorf("Expected image dimensions 1200x628, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestGenerateHomePageImage(t *testing.T) {
	img := generateHomePageImage()
	if img == nil {
		t.Fatalf("Expected non-nil image, got nil")
	}

	// Check image dimensions
	if img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 628 {
		t.Errorf("Expected image dimensions 1200x628, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}
