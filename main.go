/*
Smolblog generates a static site from a JSON manifest.
*/
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"github.com/yuin/goldmark"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	var (
		// Default to the current director and basic manifest name if not provided:
		man = flag.String("manifest", filepath.Join(wd, "smolmanifest.json"), "The location of the manifest")
		out = flag.String("output", filepath.Join(wd, "/dist"), "The output directory for the rendered pages")
	)
	flag.Parse()

	if err := realMain(*man, *out); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type manifest struct {
	Layouts []string          `json:"layouts"`
	Pages   map[string]page   `json:"pages"`
	Args    map[string]string `json:"args"`
}

// Representation of a page that eventually gets rendered.
type page struct {
	// Pages have arguments that get passed to the rendering function
	Args map[string]string `json:"args"`
	// The main layout to use for the page
	Layout string `json:"layout"`
	// Where to render the page in the output directory
	Path string
	// Optional: Markdown causes the markdown at the given location to be parsed
	// and added to [executionArgs].RenderedMarkdown
	Markdown markdown `json:"markdown"`
}

// Making this into an object so it can be expanded later.
type markdown struct {
	Path string `json:"path"`
}

// Using this to return an error and `main` can deal with exit codes.
//
// Takes in a string pointing to the manifest file.
func realMain(manPath, outDir string) error {
	manPath = filepath.Join(manPath) // Cleans the argument
	outDir = filepath.Join(outDir)   // Cleans the output argument
	var (
		manDir = filepath.Dir(manPath)
	)

	// Parsing of the manifest to drive the rest of the program:
	manFile, err := os.Open(manPath)
	if err != nil {
		return fmt.Errorf("error opening manifest: %s", err)
	}
	defer manFile.Close()

	var man manifest
	if err := json.NewDecoder(manFile).Decode(&man); err != nil {
		return fmt.Errorf("error opening manifest: %s", err)
	}

	// Layouts are defined relative to the manifest
	tpls, err := parseLayouts(manPath, man.Layouts)
	if err != nil {
		return fmt.Errorf("error parsing layouts: %s", err)
	}

	// Render pages one by one
	for name, page := range man.Pages {
		args := executionArgs{
			Args: page.Args,
		}

		// Parse markdown file if needed
		if md := page.Markdown; md.Path != "" {
			path := md.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(manDir, path)
			}
			mdByts, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("error opening markdown for '%s': %s", name, err)
			}

			var buf bytes.Buffer
			if err := goldmark.Convert(mdByts, &buf); err != nil {
				return fmt.Errorf("error converting markdown for '%s': %s", name, err)
			}

			args.RenderedMarkdown = template.HTML(buf.String())
		}

		f, err := createIndex(filepath.Join(outDir, page.Path, "index.html"))
		if err != nil {
			return fmt.Errorf("error creating index.html for page '%s': %s", name, err)
		}
		defer f.Close()

		if err := tpls.ExecuteTemplate(f, "post", args); err != nil {
			return fmt.Errorf("error executing template for page '%s': %s", name, err)
		}
	}

	return nil
}

// Creates the index.html at the given path.
func createIndex(p string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(p), 0770); err != nil {
		return nil, err
	}

	return os.Create(p)
}

type executionArgs struct {
	RenderedMarkdown template.HTML
	Args             map[string]string
}

// Parses the layout paths relative to the manifest path.
func parseLayouts(manifestPath string, layoutPaths []string) (*template.Template, error) {
	var (
		manifestDir = filepath.Dir(manifestPath)
		paths       = make([]string, 0, len(layoutPaths))
	)
	for _, path := range layoutPaths {
		if filepath.IsAbs(path) {
			// If it's an abolute path, don't use the manifest directory
			paths = append(paths, path)
			continue
		}

		paths = append(paths, filepath.Join(manifestDir, path))
	}

	tpls, err := template.ParseFiles(paths...)
	if err != nil {
		return nil, fmt.Errorf("error parsing layouts: %s", err)
	}

	return tpls, nil
}
