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
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/draw"
	// Import a package that can handle SVG to PNG conversion.
)

var (
	githubUsername   = os.Getenv("GITHUB_USERNAME")
	githubRepository = os.Getenv("GITHUB_REPOSITORY")
	cacheDir         = "cache"
	cacheDuration    = 12 * time.Hour
	companyConfig    = make(map[string]Company)
	projectTypes     = make(map[string]string)
	yearsProjects    []YearProjects
)

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
	OGTitle       string
	OGUrl         string
	OGImage       string
	OGDescription string
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
	companiesURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/config/companies.json", githubUsername, githubRepository)
	typesURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/config/types.json", githubUsername, githubRepository)

	err := fetchJSON(companiesURL, "companies.json", &companyConfig)
	if err != nil {
		log.Fatal("Error fetching company config:", err)
	}

	err = fetchJSON(typesURL, "types.json", &projectTypes)
	if err != nil {
		log.Fatal("Error fetching project type config:", err)
	}

	var allProjects []Project
	for companyName := range companyConfig {
		projectsURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/companies/%s.json", githubUsername, githubRepository, companyName)
		var projects []Project
		cacheFileName := fmt.Sprintf("%s.json", companyName)
		err := fetchJSON(projectsURL, cacheFileName, &projects)
		if err != nil {
			log.Fatalf("Error fetching projects for %s: %v", companyName, err)
		}

		company := companyConfig[companyName]
		company.Projects = projects
		companyConfig[companyName] = company
		allProjects = append(allProjects, projects...)
	}

	projectYearMap := make(map[string][]Project)
	for _, project := range allProjects {
		year := strings.Split(project.DateClose, "-")[0]
		projectYearMap[year] = append(projectYearMap[year], project)
	}

	for _, projects := range projectYearMap {
		sort.Slice(projects, func(i, j int) bool {
			return projects[i].DateClose > projects[j].DateClose
		})
	}

	for year, projects := range projectYearMap {
		yearsProjects = append(yearsProjects, YearProjects{Year: year, Projects: projects})
	}

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

	http.HandleFunc("/", indexHandler(tmpl))
	http.HandleFunc("/company/", companyHandler(tmpl))
	http.HandleFunc("/project/", projectHandler(tmpl))
	http.HandleFunc("/og/", ogImageHandler)

	fmt.Println("Starting server at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func indexHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

func companyHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
				OGImage:       scheme + "://" + r.Host + "/og/company/" + companyName,
				OGDescription: company.Description,
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
	}
}

func projectHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
				OGTitle:       projectName + " is a " + project.Type + " being killed by " + companyName + " in " + strings.Split(project.DateClose, "-")[0],
				OGUrl:         getFullURL(r),
				OGImage:       scheme + "://" + r.Host + "/og/project/" + companyName + "/" + projectName,
				OGDescription: project.Description,
			},
			Project: project,
		}

		for companyName, company := range companyConfig {
			projectPageData.Companies[companyName] = company.Logo
		}

		err := tmpl.ExecuteTemplate(w, "project", projectPageData)
		if err != nil {
			log.Printf("Error executing template: %v", err)
			http.Error(w, "Internal Server Error", 500)
		}
	}
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

func fetchJSON(url, cacheFileName string, v interface{}) error {
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		err := os.Mkdir(cacheDir, 0755)
		if err != nil {
			return err
		}
	}

	cacheFilePath := filepath.Join(cacheDir, cacheFileName)
	if info, err := os.Stat(cacheFilePath); err == nil {
		if time.Since(info.ModTime()) < cacheDuration {
			bytes, err := os.ReadFile(cacheFilePath)
			if err != nil {
				return err
			}
			return json.Unmarshal(bytes, v)
		}
	}

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

	err = json.Unmarshal(bytes, v)
	if err != nil {
		return err
	}

	err = os.WriteFile(cacheFilePath, bytes, 0644)
	if err != nil {
		return err
	}

	return nil
}

func isFutureDate(dateStr string) bool {
	layout := "2006-01-02"
	date, err := time.Parse(layout, dateStr)
	if err != nil {
		log.Printf("Error parsing date: %v", err)
		return false
	}
	return date.After(time.Now())
}

func downloadImage(url string) (image.Image, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if strings.HasSuffix(url, ".svg") {
		// Parse SVG data
		icon, err := oksvg.ReadIconStream(resp.Body)
		if err != nil {
			return nil, err
		}

		w, h := int(icon.ViewBox.W), int(icon.ViewBox.H)
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
		raster := rasterx.NewDasher(w, h, scanner)

		icon.SetTarget(0, 0, float64(w), float64(h))
		icon.Draw(raster, 1.0)

		return img, nil
	} else {
		// Handle other image formats
		img, _, err := image.Decode(resp.Body)
		if err != nil {
			return nil, err
		}
		return img, nil
	}
}

func generateSimpleImage(text string, logoURL string, description string) image.Image {
	const W = 1200
	const H = 630
	const LogoMaxWidth = 300
	const LogoMaxHeight = 150

	dc := gg.NewContext(W, H)
	dc.SetRGB(0, 0, 0) // Set background color
	dc.Clear()
	dc.SetRGB(1, 1, 1) // Set text color

	fontPath := "./classic-stroke/ClassicStroke-Texture.ttf"
	if err := dc.LoadFontFace(fontPath, 48); err != nil {
		log.Fatalf("Error loading font: %v", err)
	}

	dc.DrawStringAnchored(text, float64(W)/2, float64(H)/4, 0.5, 0.5)

	dc.LoadFontFace(fontPath, 24)
	dc.DrawStringWrapped(description, float64(W)/2, float64(H)/2, 0.5, 0.5, float64(W)-40, 1.5, gg.AlignCenter)

	if logoURL != "" {
		logo, err := downloadImage(logoURL)
		if err == nil {
			// Calculate scaling factor to fit the logo within the maximum dimensions if necessary
			scaleWidth := float64(LogoMaxWidth) / float64(logo.Bounds().Dx())
			scaleHeight := float64(LogoMaxHeight) / float64(logo.Bounds().Dy())
			scale := math.Min(scaleWidth, scaleHeight)

			// If the logo is larger than the max dimensions, scale it down
			if scale < 1 {
				logoWidth := float64(logo.Bounds().Dx()) * scale
				logoHeight := float64(logo.Bounds().Dy()) * scale

				dc.DrawImageAnchored(resizeImage(logo, int(logoWidth), int(logoHeight)), W/2, 3*H/4, 0.5, 0.5)
			} else {
				// If no scaling is needed, draw the logo at original size
				dc.DrawImageAnchored(logo, W/2, 3*H/4, 0.5, 0.5)
			}
		} else {
			log.Printf("Error downloading logo: %v", err)
		}
	}

	return dc.Image()
}

// Helper function to resize an image while maintaining aspect ratio
func resizeImage(img image.Image, width int, height int) image.Image {
	rect := image.Rect(0, 0, width, height)
	dst := image.NewRGBA(rect)
	draw.BiLinear.Scale(dst, rect, img, img.Bounds(), draw.Over, nil)
	return dst
}

// Handler for generating Open Graph images
func ogImageHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/og/")
	if path == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	var img image.Image

	// Handle different types of OG paths
	switch {
	case path == "home":
		// Generate a default image for the home page
		img = generateHomePageImage()
	case strings.HasPrefix(path, "company/"):
		companyName := strings.TrimPrefix(path, "company/")
		company, ok := companyConfig[companyName]
		if !ok {
			http.NotFound(w, r)
			return
		}
		img = generateSimpleImage("Explore projects by "+companyName, company.Logo, company.Description)
	case strings.HasPrefix(path, "project/"):
		parts := strings.SplitN(path[len("project/"):], "/", 2)
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		company, ok := companyConfig[parts[0]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		project, ok := findProjectByName(company.Projects, parts[1])
		if !ok {
			http.NotFound(w, r)
			return
		}
		img = generateDetailedImage(project)
	default:
		http.NotFound(w, r)
		return
	}

	// Encode and write the image as PNG to the response
	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, img); err != nil {
		http.Error(w, "Failed to encode image", http.StatusInternalServerError)
	}
}

// generateHomePageImage creates a generic image for the home page
func generateHomePageImage() image.Image {
	const W = 1200
	const H = 630
	dc := gg.NewContext(W, H)
	dc.SetRGB(0.9, 0.9, 0.9) // Light grey background
	dc.Clear()

	dc.SetRGB(0, 0, 0)                                                // Black text
	dc.LoadFontFace("./classic-stroke/ClassicStroke-Texture.ttf", 48) // Adjust path and size as needed
	dc.DrawStringAnchored("Welcome to Killed by", W/2, H/3, 0.5, 0.5)
	dc.DrawStringAnchored("Explore the lifecycle of discontinued projects", W/2, H*2/3, 0.5, 0.5)

	return dc.Image()
}

func generateDetailedImage(project Project) image.Image {
	const W = 1200
	const H = 630
	dc := gg.NewContext(W, H)

	// Set background and text colors
	dc.SetRGB(1, 1, 1) // White background for clarity
	dc.Clear()
	dc.SetRGB(0, 0, 0) // Black text for visibility

	// Load and set font
	fontPath := "./classic-stroke/ClassicStroke-Texture.ttf"
	if err := dc.LoadFontFace(fontPath, 24); err != nil {
		log.Fatalf("Error loading font: %v", err)
	}

	// Initialize y-coordinate for text placement
	y := 50.0

	// Function to draw text with wrapping
	drawText := func(text string, wrapWidth float64) {
		lines := dc.WordWrap(text, wrapWidth)
		for _, line := range lines {
			dc.DrawString(line, 50, y)
			y += 30 // Increment y-coordinate for the next line
		}
		y += 20 // Add extra space after a block of text
	}

	// Draw each piece of information
	drawText(fmt.Sprintf("Project: %s", project.Name), W-100)
	drawText(fmt.Sprintf("Description: %s", project.Description), W-100)
	drawText(fmt.Sprintf("Developer/Company: %s", project.Company), W-100)
	drawText(fmt.Sprintf("Released: %s - Discontinued: %s", project.DateOpen, project.DateClose), W-100)

	// Calculate and display lifespan
	lifespan := calculateLifespan(project.DateOpen, project.DateClose)
	drawText(fmt.Sprintf("Lifespan: %s years", lifespan), W-100)

	// Type with color
	typeColor := projectTypes[project.Type] // Ensure this map is well-defined in your application
	dc.SetHexColor(typeColor)
	dc.DrawStringWrapped("Type: "+project.Type, 50, y, 0, 0, W-100, 1.5, gg.AlignLeft)
	y += 40

	// Reset color for any additional text
	dc.SetRGB(0, 0, 0)

	return dc.Image()
}

func calculateLifespan(start, end string) string {
	startDate, _ := time.Parse("2006-01-02", start)
	endDate, _ := time.Parse("2006-01-02", end)
	lifespan := endDate.Sub(startDate).Hours() / 24 / 365
	return fmt.Sprintf("%.2f", lifespan)
}

// Helper function to find a project by name within a slice of projects
func findProjectByName(projects []Project, name string) (Project, bool) {
	for _, project := range projects {
		if project.Name == name {
			return project, true
		}
	}
	return Project{}, false
}
