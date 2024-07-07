package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var githubUsername = "bacherik"
var githubRepository = "killedby.json"
var cacheDir = "cache"
var cacheDuration = 12 * time.Hour

// Define the structs for JSON data
type Project struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Link        string `json:"link"`
	Type        string `json:"type"`
	Company     string `json:"company"`
	DateOpen    string `json:"dateOpen"`
	DateClose   string `json:"dateClose"`
}

type Company struct {
	Logo        string    `json:"logo"`
	Description string    `json:"description"`
	Projects    []Project `json:"projects"`
}

type ProjectType struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type BasePageData struct {
	Title     string
	Companies map[string]string
}

type IndexPageData struct {
	BasePageData
	Projects []Project
	Types    map[string]string
}

type CompanyPageData struct {
	BasePageData
	Projects []Project
	Types    map[string]string
}

type ProjectPageData struct {
	BasePageData
	Project Project
}

func main() {
	// URLs for the JSON files
	companiesURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/config/companies.json", githubUsername, githubRepository)
	typesURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/config/types.json", githubUsername, githubRepository)

	// Unmarshal company configs
	companyConfig := make(map[string]Company)
	err := fetchJSON(companiesURL, "companies.json", &companyConfig)
	if err != nil {
		log.Fatal("Error fetching company config:", err)
	}

	// Unmarshal project type configs
	projectTypes := make(map[string]string)
	err = fetchJSON(typesURL, "types.json", &projectTypes)
	if err != nil {
		log.Fatal("Error fetching project type config:", err)
	}

	// Fetch and unmarshal projects for each company
	var allProjects []Project
	for companyName := range companyConfig {
		projectsURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/companies/%s.json", githubUsername, githubRepository, companyName)
		var projects []Project
		cacheFileName := fmt.Sprintf("%s.json", companyName)
		err := fetchJSON(projectsURL, cacheFileName, &projects)
		if err != nil {
			log.Fatalf("Error fetching projects for %s: %v", companyName, err)
		}

		// Add projects to the company in the map
		company := companyConfig[companyName]
		company.Projects = projects
		companyConfig[companyName] = company
		allProjects = append(allProjects, projects...)
	}

	// Parse the templates from the templates folder
	tmpl, err := template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("Error parsing templates: %v", err)
	}

	// Handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		pageData := IndexPageData{
			BasePageData: BasePageData{
				Title:     "Companies and Projects",
				Companies: make(map[string]string),
			},
			Projects: allProjects,
			Types:    projectTypes,
		}

		for companyName, company := range companyConfig {
			pageData.Companies[companyName] = company.Logo
		}

		err := tmpl.ExecuteTemplate(w, "index", pageData)
		if err != nil {
			log.Printf("Error executing template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/company/", func(w http.ResponseWriter, r *http.Request) {
		companyName := strings.TrimPrefix(r.URL.Path, "/company/")
		company, ok := companyConfig[companyName]
		if !ok {
			http.NotFound(w, r)
			return
		}

		companyPageData := CompanyPageData{
			BasePageData: BasePageData{
				Title:     companyName,
				Companies: make(map[string]string),
			},
			Projects: company.Projects,
			Types:    projectTypes,
		}

		for companyName, company := range companyConfig {
			companyPageData.Companies[companyName] = company.Logo
		}

		err := tmpl.ExecuteTemplate(w, "company", companyPageData)
		if err != nil {
			log.Printf("Error executing template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/project/", func(w http.ResponseWriter, r *http.Request) {
		projectPath := strings.TrimPrefix(r.URL.Path, "/project/")
		parts := strings.SplitN(projectPath, "/", 2)
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}

		companyName, projectName := parts[0], parts[1]
		company, ok := companyConfig[companyName]
		if !ok {
			http.NotFound(w, r)
			return
		}

		var project Project
		found := false
		for _, p := range company.Projects {
			if p.Name == projectName {
				project = p
				found = true
				break
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}

		projectPageData := ProjectPageData{
			BasePageData: BasePageData{
				Title:     projectName,
				Companies: make(map[string]string),
			},
			Project: project,
		}

		for companyName, company := range companyConfig {
			projectPageData.Companies[companyName] = company.Logo
		}

		err := tmpl.ExecuteTemplate(w, "project", projectPageData)
		if err != nil {
			log.Printf("Error executing template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	fmt.Println("Starting server at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func fetchJSON(url, cacheFileName string, v interface{}) error {
	// Create cache directory if it doesn't exist
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		err := os.Mkdir(cacheDir, 0755)
		if err != nil {
			return err
		}
	}

	cacheFilePath := filepath.Join(cacheDir, cacheFileName)

	// Check if the cached file exists and is still valid
	if info, err := os.Stat(cacheFilePath); err == nil {
		if time.Since(info.ModTime()) < cacheDuration {
			// Read from the cached file
			bytes, err := ioutil.ReadFile(cacheFilePath)
			if err != nil {
				return err
			}
			return json.Unmarshal(bytes, v)
		}
	}

	fmt.Print("Fetching ", url, "... ")

	// Fetch from the URL
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Unmarshal the JSON data
	err = json.Unmarshal(bytes, v)
	if err != nil {
		return err
	}

	// Write the data to the cache file
	err = ioutil.WriteFile(cacheFilePath, bytes, 0644)
	if err != nil {
		return err
	}

	return nil
}
