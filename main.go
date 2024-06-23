/*
Smolblog runs a site from a JSON manifest.
*/
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/yuin/goldmark"
)

type (
	// Manifest is the structure of the data driving the web server.
	//
	// It has two main pieces:
	// - `layouts`, which are any templates that are globbed
	// - `rotues`, which are registered as get routes and served by the handler
	Manifest struct {
		Layouts []string         `json:"layouts"`
		Routes  map[string]Route `json:"routes"`
	}

	// Route is a registered path that is run when a GET request is made to it.
	// TODO: Doc more
	Route struct {
		// The name of the template to execute first
		Template string `json:"template"`
		// Any arbitrary arguments to be used in executing the template
		Args map[string]any `json:"args"`
	}
)

func main() {
	var (
		ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt)

		manifestPath = flag.String("manifest", "", "path to the manifest")
		port         = flag.Int("port", 4444, "port to run the sever on")
	)
	flag.Parse()
	defer cancel()

	if *manifestPath == "" {
		slog.Error("'-manifest' must be provided")
		os.Exit(1)
	}

	if err := realMain(ctx, *manifestPath, *port); err != nil {
		slog.Error("error occurred", "err", err)
		os.Exit(1)
	}
}

// Using this to return an error and `main` can deal with exit codes.
func realMain(ctx context.Context, manPath string, port int) error {
	var (
		h = newHandler(manPath)
		s = http.Server{
			Addr:         fmt.Sprintf("0.0.0.0:%d", port),
			Handler:      h,
			ReadTimeout:  1 * time.Second,
			WriteTimeout: 1. * time.Second,
		}
		errs = make(chan error)
	)

	// Waiting for cancellation signal to stop the server
	go func() {
		<-ctx.Done()
		s.Close()
	}()

	// The server process:
	go func() {
		slog.Info("server started", "on", fmt.Sprintf("0.0.0.0:%d", port))
		if err := s.ListenAndServe(); err != nil {
			errs <- fmt.Errorf("error serving: %s", err)
		}
	}()
	// Waiting for either an error or the ctx to cancel
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}

	return nil
}

// Does the serving of each request and holds dependencies of executing said requests.
//
// When told to, it re-parses the manifest and templates.
// If unable to serve the request, it will return an error code.
type handler struct {
	manifestPath string // Points to the manifest
	// Points to the parent directory of the manifest.
	// This is so paths in the manifest can be relative to the manifest itself.
	manifestDir string
}

// Sets the manifest path on a new handler as well as the manifest directory
// so requests have access to it for resolving relative paths.
func newHandler(manPath string) *handler {
	return &handler{
		manifestPath: manPath,
		manifestDir:  filepath.Dir(manPath),
	}
}

// Returns the manifest and loads any layouts specified in the manifest.
func loadManifest(ctx context.Context, manifestPath, manifestDir string) (*Manifest, *template.Template, error) {
	// Reading and parsing of the manifest.
	// This will determine where the layouts are and what to parse next.
	byts, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading manfiest: %s", err)
	}

	var man Manifest
	if err := json.Unmarshal(byts, &man); err != nil {
		return nil, nil, fmt.Errorf("error unmarshaling manifest: %s", err)
	}

	// Parsing layouts happens here, putting them on the `handler` struct
	// for usage when responding to a request.
	//
	// Filepaths for layouts are relative to the manifest's path, so
	// they must be joined to the manifest path to properly resolve.
	paths := make([]string, 0, len(man.Layouts))
	for _, l := range man.Layouts {
		path := filepath.Join(manifestDir, l)
		paths = append(paths, path)
	}
	tpls, err := template.New("").
		Funcs(templateFuncs(manifestDir)).
		ParseFiles(paths...)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing layouts: %s", err)
	}

	return &man, tpls, nil
}

// Creates the template functions that can be used when executing.
func templateFuncs(manifestDir string) template.FuncMap {
	return template.FuncMap{
		// Creates a closure that opens the given file (relative to the manifest) and parses
		// it as markdown.
		// In the case of any error, it will panic.
		//
		// Returns the rendered HTML, unescaped.
		"renderMarkdown": func(src string) template.HTML {
			path := filepath.Join(manifestDir, src)
			mdByts, err := os.ReadFile(path)
			if err != nil {
				panic(fmt.Sprintf("error opening file to parse markdown: %s", err))
			}

			var buf bytes.Buffer
			if err := goldmark.Convert(mdByts, &buf); err != nil {
				panic(fmt.Sprintf("error converting markdown: %s", err))
			}

			return template.HTML(buf.String())
		},
	}
}

// This holds the data getting passed to the template being executed,
// as well as information about the current path being handled.
type routeArgs struct {
	// The path of the current route
	Path string
	// The args from the manifest
	Args map[string]any
}

// ServeHTTP implements [http.Handler] for each request.
//
// It only serves GET's.
// It looks up the route in the manifest, and if it's present, it executes the logic of the route: If the route is not found, it returns a 404.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slog.Info("request received",
		"method", r.Method,
		"location", r.URL.String(),
	)

	// Only respond to GETs, otherwise respond 405
	if method := r.Method; method != http.MethodGet {
		slog.Error("method not allowed", "method", method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// The manifest is loaded each request in case there were changes to either the manifest
	// itself or one of the templates.
	man, tpls, err := loadManifest(r.Context(), h.manifestPath, h.manifestDir)
	if err != nil {
		slog.Error("error reloading manifest", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check that the route exists, if not: 404
	path := r.URL.Path
	route, ok := man.Routes[path] // Ignore fragments, query string etc
	if !ok {
		slog.Error("route not found", "path", path)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Each route is a template + arguments, so handling the route is just
	// executing the template named in the route's `Template` field with the `Args` field.
	//
	// BUG: If there's an execution error, the write has already received output, so it
	// automatically sends a 200 and the 500 is a superfluous call.
	if err := tpls.ExecuteTemplate(
		w,
		route.Template,
		routeArgs{
			Path: path,
			Args: route.Args,
		},
	); err != nil {
		slog.Error("error executing route's template", "route", route, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
