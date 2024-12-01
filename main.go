package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/gorilla/mux"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/draw"
)

const (
	cacheDir      = "cache"
	cacheDuration = 12 * time.Hour
)

var (
	githubUsername   = os.Getenv("GITHUB_USERNAME")
	githubRepository = os.Getenv("GITHUB_REPOSITORY")
	companyConfig    = make(map[string]Company)
	projectTypes     = make(map[string]string)
	yearsProjects    []YearProjects
	tmpl             *template.Template
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
	CustomFooter  template.HTML
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

	if err := fetchJSON(companiesURL, "companies.json", &companyConfig); err != nil {
		log.Fatalf("Error fetching company config: %v", err)
	}

	if err := fetchJSON(typesURL, "types.json", &projectTypes); err != nil {
		log.Fatalf("Error fetching project type config: %v", err)
	}

	allProjects := fetchAllProjects()

	projectYearMap := groupProjectsByYear(allProjects)

	yearsProjects = sortProjectsByYear(projectYearMap)

	funcMap := template.FuncMap{
		"dict":              createDict,
		"isFutureDate":      isFutureDate,
		"calculateLifespan": calculateLifespan,
		"domainOnly":        domainOnly,
	}

	// Fetch custom footer if available
	customFooter, err := fetchCustomFooter()
	if err == nil {
		if err = os.WriteFile("templates/custom_footer.html", []byte(customFooter), 0644); err != nil {
			log.Printf("Error writing custom footer: %v", err)
		}
	} else {
		log.Printf("Using default footer: %v", err)
	}

	tmpl = parseTemplates(funcMap)

	r := mux.NewRouter()

	r.HandleFunc("/", indexHandler).Methods("GET")
	r.HandleFunc("/company/{companyName}", companyHandler).Methods("GET")
	r.HandleFunc("/project/{companyName}/{projectName}", projectHandler).Methods("GET")
	r.HandleFunc("/og/", ogImageHandler).Methods("GET")

	fmt.Println("Starting server at :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func createDict(values ...interface{}) map[string]interface{} {
	if len(values)%2 != 0 {
		log.Fatalf("invalid dict call")
	}
	dict := make(map[string]interface{})
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			log.Fatalf("dict keys must be strings")
		}
		dict[key] = values[i+1]
	}
	return dict
}

func domainOnly(urlStr string) string {
	urlParsed, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}
	return urlParsed.Hostname()
}

func fetchAllProjects() []Project {
	var allProjects []Project
	for companyName := range companyConfig {
		projectsURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/companies/%s.json", githubUsername, githubRepository, companyName)
		var projects []Project
		cacheFileName := fmt.Sprintf("%s.json", companyName)
		if err := fetchJSON(projectsURL, cacheFileName, &projects); err != nil {
			log.Fatalf("Error fetching projects for %s: %v", companyName, err)
		}
		companyConfig[companyName] = Company{
			Logo:        companyConfig[companyName].Logo,
			Description: companyConfig[companyName].Description,
			Projects:    projects,
		}
		allProjects = append(allProjects, projects...)
	}
	return allProjects
}

func groupProjectsByYear(allProjects []Project) map[string][]Project {
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
	return projectYearMap
}

func sortProjectsByYear(projectYearMap map[string][]Project) []YearProjects {
	var yearsProjects []YearProjects
	for year, projects := range projectYearMap {
		yearsProjects = append(yearsProjects, YearProjects{Year: year, Projects: projects})
	}
	sort.Slice(yearsProjects, func(i, j int) bool {
		return yearsProjects[i].Year > yearsProjects[j].Year
	})
	return yearsProjects
}

func parseTemplates(funcMap template.FuncMap) *template.Template {
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("Error parsing templates: %v", err)
	}
	return tmpl
}

func fetchJSON(url, cacheFileName string, v interface{}) error {
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		if err := os.Mkdir(cacheDir, 0755); err != nil {
			return err
		}
	}
	cacheFilePath := filepath.Join(cacheDir, cacheFileName)
	if info, err := os.Stat(cacheFilePath); err == nil && time.Since(info.ModTime()) < cacheDuration {
		bytes, err := os.ReadFile(cacheFilePath)
		if err != nil {
			return err
		}
		return json.Unmarshal(bytes, v)
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

	if err := json.Unmarshal(bytes, v); err != nil {
		return err
	}

	return os.WriteFile(cacheFilePath, bytes, 0644)
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

func indexHandler(w http.ResponseWriter, r *http.Request) {
	customFooter := loadCustomFooter()
	pageData := IndexPageData{
		BasePageData: BasePageData{
			Title:         "Projects by Year",
			Companies:     extractCompanies(),
			OGTitle:       "Killed by - Home",
			OGUrl:         getFullURL(r),
			OGImage:       getOGImageURL(r, "home"),
			OGDescription: "Explore discontinued projects and their histories.",
			CustomFooter:  customFooter,
		},
		YearsProjects: yearsProjects,
		Types:         projectTypes,
	}

	if err := tmpl.ExecuteTemplate(w, "index", pageData); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", 500)

	}
}

func companyHandler(w http.ResponseWriter, r *http.Request) {
	companyName := mux.Vars(r)["companyName"]
	company, ok := companyConfig[companyName]
	if !ok {
		http.NotFound(w, r)
		return
	}

	customFooter := loadCustomFooter()
	companyPageData := CompanyPageData{
		BasePageData: BasePageData{
			Title:         companyName,
			Companies:     extractCompanies(),
			OGTitle:       "Killed by - " + companyName,
			OGUrl:         getFullURL(r),
			OGImage:       getOGImageURL(r, "company/"+companyName),
			OGDescription: company.Description,
			CustomFooter:  customFooter,
		},
		YearsProjects: sortProjectsByYear(groupProjectsByYear(company.Projects)),
		Types:         projectTypes,
	}

	if err := tmpl.ExecuteTemplate(w, "company", companyPageData); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", 500)
	}

}

func projectHandler(w http.ResponseWriter, r *http.Request) {
	companyName, projectName := mux.Vars(r)["companyName"], mux.Vars(r)["projectName"]
	company, ok := companyConfig[companyName]
	if !ok {
		http.NotFound(w, r)
		return
	}

	project, found := findProjectByName(company.Projects, projectName)
	if !found {
		http.NotFound(w, r)
		return
	}

	customFooter := loadCustomFooter()
	projectPageData := ProjectPageData{
		BasePageData: BasePageData{
			Title:         projectName,
			Companies:     extractCompanies(),
			OGTitle:       fmt.Sprintf("%s is a %s being killed by %s in %s", projectName, project.Type, companyName, project.DateClose[:4]),
			OGUrl:         getFullURL(r),
			OGImage:       getOGImageURL(r, fmt.Sprintf("project/%s/%s", companyName, projectName)),
			OGDescription: project.Description,
			CustomFooter:  customFooter,
		},
		Project: project,
	}

	if err := tmpl.ExecuteTemplate(w, "project", projectPageData); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", 500)
	}

}

func loadCustomFooter() template.HTML {
	customFooterFilePath := "templates/custom_footer.html"
	if _, err := os.Stat(customFooterFilePath); err == nil {
		content, err := os.ReadFile(customFooterFilePath)
		if err != nil {
			log.Printf("Error reading custom footer: %v", err)
			return ""
		}
		return template.HTML(content)
	}
	return ""
}

func getFullURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI)
}

func getOGImageURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/og/%s", scheme, r.Host, path)
}

func extractCompanies() map[string]string {
	companies := make(map[string]string)
	for companyName, company := range companyConfig {
		companies[companyName] = company.Logo
	}
	return companies
}

func fetchCustomFooter() (string, error) {
	customFooterURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/templates/footer.html", githubUsername, githubRepository)
	resp, err := http.Get(customFooterURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("custom footer not found")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func downloadImage(url string) (image.Image, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if strings.HasSuffix(url, ".svg") {
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
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func generateSimpleImage(text, logoURL, description string) image.Image {
	const W, H = 1200, 628
	const LogoMaxWidth, LogoMaxHeight = 300, 150

	dc := gg.NewContext(W, H)
	dc.SetRGB(0, 0, 0)
	dc.Clear()
	dc.SetRGB(1, 1, 1)

	fontPath := "./classic-stroke/ClassicStroke-Texture.ttf"
	if err := dc.LoadFontFace(fontPath, 48); err != nil {
		log.Fatalf("Error loading font: %v", err)
	}

	dc.DrawStringAnchored(text, float64(W)/2, float64(H)/4, 0.5, 0.5)

	dc.LoadFontFace(fontPath, 24)
	dc.DrawStringWrapped(description, float64(W)/2, float64(H)/2, 0.5, 0.5, float64(W)-40, 1.5, gg.AlignCenter)

	if logoURL != "" {
		if logo, err := downloadImage(logoURL); err == nil {
			scale := math.Min(float64(LogoMaxWidth)/float64(logo.Bounds().Dx()), float64(LogoMaxHeight)/float64(logo.Bounds().Dy()))
			if scale < 1 {
				logo = resizeImage(logo, int(float64(logo.Bounds().Dx())*scale), int(float64(logo.Bounds().Dy())*scale))
			}
			dc.DrawImageAnchored(logo, W/2, 3*H/4, 0.5, 0.5)
		} else {
			log.Printf("Error downloading logo: %v", err)
		}
	}

	return dc.Image()
}

func resizeImage(img image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}

func ogImageHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/og/")
	if path == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	var img image.Image
	switch {
	case path == "home":
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

	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, img); err != nil {
		http.Error(w, "Failed to encode image", http.StatusInternalServerError)
	}
}

func generateHomePageImage() image.Image {
	const W, H = 1200, 628
	dc := gg.NewContext(W, H)
	dc.SetRGB(0.9, 0.9, 0.9)
	dc.Clear()

	dc.SetRGB(0, 0, 0)
	dc.LoadFontFace("./classic-stroke/ClassicStroke-Texture.ttf", 48)
	dc.DrawStringAnchored("Welcome to Killed by", W/2, H/3, 0.5, 0.5)
	dc.DrawStringAnchored("Explore the lifecycle of discontinued projects", W/2, H*2/3, 0.5, 0.5)

	return dc.Image()
}

func generateDetailedImage(project Project) image.Image {
	const W, H = 1200, 628
	dc := gg.NewContext(W, H)

	dc.SetRGB(1, 1, 1)
	dc.Clear()
	dc.SetRGB(0, 0, 0)

	fontPath := "./classic-stroke/ClassicStroke-Texture.ttf"
	if err := dc.LoadFontFace(fontPath, 24); err != nil {
		log.Fatalf("Error loading font: %v", err)
	}

	y := 50.0

	drawText := func(text string, wrapWidth float64) {
		lines := dc.WordWrap(text, wrapWidth)
		for _, line := range lines {
			dc.DrawString(line, 50, y)
			y += 30
		}
		y += 20
	}

	drawText(fmt.Sprintf("Project: %s", project.Name), W-100)
	drawText(fmt.Sprintf("Description: %s", project.Description), W-100)
	drawText(fmt.Sprintf("Developer/Company: %s", project.Company), W-100)
	drawText(fmt.Sprintf("Released: %s - Discontinued: %s", project.DateOpen, project.DateClose), W-100)
	drawText(fmt.Sprintf("Lifespan: %s years", calculateLifespan(project.DateOpen, project.DateClose)), W-100)

	typeColor := projectTypes[project.Type]
	dc.SetHexColor(typeColor)
	dc.DrawStringWrapped("Type: "+project.Type, 50, y, 0, 0, W-100, 1.5, gg.AlignLeft)
	y += 40

	dc.SetRGB(0, 0, 0)

	return dc.Image()
}

func calculateLifespan(start, end string) string {
	startDate, _ := time.Parse("2006-01-02", start)
	endDate, _ := time.Parse("2006-01-02", end)
	lifespan := endDate.Sub(startDate).Hours() / 24 / 365
	return fmt.Sprintf("%.2f", lifespan)
}

func findProjectByName(projects []Project, name string) (Project, bool) {
	for _, project := range projects {
		if project.Name == name {
			return project, true
		}
	}
	return Project{}, false
}
