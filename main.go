package main

import (
    "fmt"
    "io/ioutil"
    "log"
    "os"
    "path/filepath"
)

func main() {
    // Define the directory paths
    contentDir := "content"
    templatesDir := "templates"
    outputDir := "output"

    // Read header and footer templates
    header, err := ioutil.ReadFile(filepath.Join(templatesDir, "header.html"))
    if err != nil {
        log.Fatalf("Error reading header: %v", err)
    }
    
    footer, err := ioutil.ReadFile(filepath.Join(templatesDir, "footer.html"))
    if err != nil {
        log.Fatalf("Error reading footer: %v", err)
    }

    // Create output directory if it doesn't exist
    if _, err := os.Stat(outputDir); os.IsNotExist(err) {
        os.Mkdir(outputDir, os.ModePerm)
    }

    // Read the content directory
    files, err := ioutil.ReadDir(contentDir)
    if err != nil {
        log.Fatalf("Error reading content directory: %v", err)
    }

    for _, file := range files {
        if filepath.Ext(file.Name()) == ".html" {
            content, err := ioutil.ReadFile(filepath.Join(contentDir, file.Name()))
            if err != nil {
                log.Printf("Error reading content file %s: %v", file.Name(), err)
                continue
            }

            outputFilePath := filepath.Join(outputDir, file.Name())
            err = ioutil.WriteFile(outputFilePath, append(append(header, content...), footer...), 0644)
            if err != nil {
                log.Printf("Error writing output file %s: %v", outputFilePath, err)
                continue
            }

            fmt.Printf("Generated %s\n", outputFilePath)
        }
    }
}
