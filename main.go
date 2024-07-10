package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"image"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fogleman/gg"
)

var githubUsername = os.Getenv("GITHUB_USERNAME")
var githubRepository = os.Getenv("GITHUB_REPOSITORY")
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
	Title         string
	Companies     map[string]string
	OGTitle       string // Open Graph title
	OGUrl         string // Open Graph URL
	OGImage       string // Open Graph image URL
	OGDescription string // Open Graph description
}

type YearProjects struct {
	Year     string
	Projects []Project
}

type IndexPageData struct {
	BasePageData
	YearsProjects []YearProjects
	Types         map[string]string
}

type CompanyPageData struct {
	BasePageData
	YearsProjects []YearProjects
	Types         map[string]string
}

type ProjectPageData struct {
	BasePageData
	Project Project
}

func main() {
	// URLs for the JSON files and other initial setup
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

	// Group projects by year
	projectYearMap := make(map[string][]Project)
	for _, project := range allProjects {
		year := strings.Split(project.DateClose, "-")[0] // Assuming DateClose is in YYYY-MM-DD format
		projectYearMap[year] = append(projectYearMap[year], project)
	}

	// Sort projects within each year
	for _, projects := range projectYearMap {
		sort.Slice(projects, func(i, j int) bool {
			return projects[i].DateClose > projects[j].DateClose
		})
	}

	// Convert the map to a slice for ordered processing in templates
	var yearsProjects []YearProjects
	for year, projects := range projectYearMap {
		yearsProjects = append(yearsProjects, YearProjects{Year: year, Projects: projects})
	}

	// Sort years to display them in order
	sort.Slice(yearsProjects, func(i, j int) bool {
		return yearsProjects[i].Year > yearsProjects[j].Year
	})

	funcMap := template.FuncMap{
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, errors.New("invalid dict call")
			}
			dict := make(map[string]interface{})
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, errors.New("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"isFutureDate": isFutureDate,
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("Error parsing templates: %v", err)
	}

	// Handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		pageData := IndexPageData{
			BasePageData: BasePageData{
				Title:     "Projects by Year",
				Companies: make(map[string]string),
				// Adding Open Graph properties
				OGTitle:       "Killed by - Home",
				OGUrl:         getFullURL(r),
				OGImage:       scheme + "://" + r.Host + "/og/home",
				OGDescription: "Explore discontinued projects and their histories.",
			},
			YearsProjects: yearsProjects, // Using the grouped projects by year
			Types:         projectTypes,
		}

		for companyName, company := range companyConfig {
			pageData.Companies[companyName] = company.Logo
		}

		err := tmpl.ExecuteTemplate(w, "index", pageData)
		if err != nil {
			log.Printf("Error executing template: %v", err)
			http.Error(w, "Internal Server Error", 500)
		}
	})

	http.HandleFunc("/company/", func(w http.ResponseWriter, r *http.Request) {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		companyName := strings.TrimPrefix(r.URL.Path, "/company/")
		company, ok := companyConfig[companyName]
		if !ok {
			http.NotFound(w, r)
			return
		}

		// Group projects by year within the company
		projectYearMap := make(map[string][]Project)
		for _, project := range company.Projects {
			year := strings.Split(project.DateClose, "-")[0] // Extract year from DateClose
			projectYearMap[year] = append(projectYearMap[year], project)
		}

		// Convert the map to a slice for ordered processing in templates
		var yearsProjects []YearProjects
		for year, projects := range projectYearMap {
			yearsProjects = append(yearsProjects, YearProjects{Year: year, Projects: projects})
		}

		// Sort years to display them in order
		sort.Slice(yearsProjects, func(i, j int) bool {
			return yearsProjects[i].Year > yearsProjects[j].Year
		})

		companyPageData := CompanyPageData{
			BasePageData: BasePageData{
				Title:     companyName,
				Companies: make(map[string]string),
				// Adding Open Graph properties
				OGTitle:       "Killed by - " + companyName,
				OGUrl:         getFullURL(r),
				OGImage:       scheme + "://" + r.Host + "/og/" + r.RequestURI,
				OGDescription: "Explore discontinued projects and their histories.",
			},
			YearsProjects: yearsProjects, // Use grouped projects by year
			Types:         projectTypes,
		}

		for companyName, company := range companyConfig {
			companyPageData.Companies[companyName] = company.Logo
		}

		err := tmpl.ExecuteTemplate(w, "company", companyPageData)
		if err != nil {
			log.Printf("Error executing template: %v", err)
			http.Error(w, "Internal Server Error", 500)
		}
	})

	http.HandleFunc("/project/", func(w http.ResponseWriter, r *http.Request) {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
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
				// Adding Open Graph properties
				OGTitle:       "Killed by - " + projectName,
				OGUrl:         getFullURL(r),
				OGImage:       scheme + "://" + r.Host + "/og/" + r.RequestURI,
				OGDescription: "Explore discontinued projects and their histories.",
			},
			Project: project,
		}

		http.HandleFunc("/og/", ogImageHandler)

		for companyName, company := range companyConfig {
			projectPageData.Companies[companyName] = company.Logo
		}

		err := tmpl.ExecuteTemplate(w, "project", projectPageData)
		if err != nil {
			log.Printf("Error executing template: %v", err)
			http.Error(w, "Internal Server Error", 500)
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
			bytes, err := os.ReadFile(cacheFilePath)
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
	err = os.WriteFile(cacheFilePath, bytes, 0644)
	if err != nil {
		return err
	}

	return nil
}

// isFutureDate checks if the given date string is in the future.
func isFutureDate(dateStr string) bool {
	layout := "2006-01-02" // Adjust this layout as necessary to match the date format
	date, err := time.Parse(layout, dateStr)
	if err != nil {
		log.Printf("Error parsing date: %v", err)
		return false
	}
	return date.After(time.Now())
}

// getFullURL returns the full URL from the given http.Request object.
func getFullURL(r *http.Request) string {
	// Check if the request is made over HTTPS
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI)
}

func generateImage(text string) image.Image {
	const W = 1200
	const H = 630
	dc := gg.NewContext(W, H)
	dc.SetRGB(0, 0, 0) // Set background color
	dc.Clear()
	dc.SetRGB(1, 1, 1)                       // Set text color
	dc.LoadFontFace("/path/to/font.ttf", 48) // Load your font
	dc.DrawStringAnchored(text, float64(W)/2, float64(H)/2, 0.5, 0.5)
	return dc.Image()
}

func ogImageHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the path after /og/
	path := strings.TrimPrefix(r.URL.Path, "/og/")
	if path == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Decide what text to display based on the 'path'
	text := generateTextForOGImage(path)

	// Generate the image
	img := generateImage(text) // Implement this function to generate an image based on the text

	// Set the header and write the image
	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, img); err != nil {
		http.Error(w, "Failed to encode image", http.StatusInternalServerError)
	}
}

func generateTextForOGImage(path string) string {
	// Simple routing logic based on path
	if strings.HasPrefix(path, "company/") {
		return "Information about " + strings.TrimPrefix(path, "company/")
	} else if strings.HasPrefix(path, "project/") {
		return "Details of project " + strings.TrimPrefix(path, "project/")
	} else if strings.HasPrefix(path, "home/") {
		return "Killed by - Explore discontinued projects"
	}
	return "Killed by - Explore discontinued projects"
}
