package main

import (
	"encoding/json"
	"html/template"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	_ "image/jpeg"
	_ "image/png"
	"io"

	"github.com/nfnt/resize"
)

type Type struct {
	Types     map[string]string `json:"types"`
	Companies map[string]string `json:"companies"`
}

type Project struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Link        string `json:"link"`
	Type        string `json:"type"`
	Company     string `json:"company"`
	DateOpen    string `json:"dateOpen"`
	DateClose   string `json:"dateClose"`
}

type Data struct {
	Projects  []Project
	Types     map[string]string
	Companies map[string]string
}

func main() {
	typesFile, err := os.ReadFile("types_and_companies.json")
	if err != nil {
		log.Fatalf("Error reading types and companies file: %v", err)
	}

	var types Type
	err = json.Unmarshal(typesFile, &types)
	if err != nil {
		log.Fatalf("Error unmarshaling types and companies JSON: %v", err)
	}

	data := Data{
		Types:     types.Types,
		Companies: types.Companies,
	}

	// Read company project files
	for company := range types.Companies {
		companyProjectsFile := filepath.Join("companies", company+".json")
		projectsFile, err := os.ReadFile(companyProjectsFile)
		if err != nil {
			log.Fatalf("Error reading projects file for company %s: %v", company, err)
		}

		var companyProjects []Project
		err = json.Unmarshal(projectsFile, &companyProjects)
		if err != nil {
			log.Fatalf("Error unmarshaling projects JSON for company %s: %v", company, err)
		}

		data.Projects = append(data.Projects, companyProjects...)
	}

	indexTmpl := template.Must(template.ParseFiles("templates/indexTemplate.html"))
	projectTmpl := template.Must(template.New("project").Parse(projectTemplate))

	// Create output directory if it doesn't exist
	err = os.MkdirAll("output", os.ModePerm)
	if err != nil {
		log.Fatalf("Error creating output directory: %v", err)
	}

	// Copy and resize assets to output directory
	err = copyAndResizeAssets("assets", "output/assets", 100) // Resize to 100px width
	if err != nil {
		log.Fatalf("Error copying and resizing assets: %v", err)
	}

	f, err := os.Create("output/index.html")
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}
	defer f.Close()

	err = indexTmpl.Execute(f, data)
	if err != nil {
		log.Fatalf("Error executing index template: %v", err)
	}

	for _, project := range data.Projects {
		companyDir := filepath.Join("output", project.Company)
		err = os.MkdirAll(companyDir, os.ModePerm)
		if err != nil {
			log.Fatalf("Error creating company directory: %v", err)
		}

		projectFile, err := os.Create(filepath.Join(companyDir, project.Name+".html"))
		if err != nil {
			log.Fatalf("Error creating project file: %v", err)
		}
		defer projectFile.Close()

		err = projectTmpl.Execute(projectFile, project)
		if err != nil {
			log.Fatalf("Error executing project template: %v", err)
		}
	}
}

func copyAndResizeAssets(src, dst string, width uint) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath := path[len(src):]
		targetPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		} else {
			if filepath.Ext(path) == ".png" || filepath.Ext(path) == ".jpg" || filepath.Ext(path) == ".jpeg" {
				return resizeImage(path, targetPath, width)
			}
			return copyFile(path, targetPath)
		}
	})
}

func resizeImage(src, dst string, width uint) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return err
	}

	m := resize.Resize(width, 0, img, resize.Lanczos3)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	return png.Encode(out, m)
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

const projectTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ .Name }}</title>
    <link rel="stylesheet" href="/assets/css/custom.css">
</head>
<body>
    <header>
        <h1>{{ .Name }}</h1>
    </header>
    <main>
        <p>{{ .Description }}</p>
        <p>Released: {{ .DateOpen }}</p>
		<p>Discontinued: {{ .DateClose }}</p>
		<p>Lifespan: </p>
		<p>Company: {{ .Company }}</p>
		<p>Source: <a href="{{ .Link }}">{{ .Link }}</a></p>
    </main>
    <footer>
        <p>&copy; 2024 {{ .Company }}</p>
    </footer>
</body>
</html>`
