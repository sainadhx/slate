package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"
)

type Page struct {
	Path    string
	URL     string
	Title   string
	Date    time.Time
	Content template.HTML
}

type Frontmatter struct {
	Title string `yaml:"title"`
	Date  string `yaml:"date"`
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			initProject()
			return
		case "build":
			build()
			return
		case "serve":
			serve()
			return
		default:
			fmt.Println("Unknown command:", os.Args[1])
			fmt.Println("Usage: slate [init|build|serve]")
			return
		}
	} else {
		// Default to build
		build()
	}
}

func initProject() {
	dirs := []string{
		"content",
		"content/blog",
		"templates",
		"static",
	}

	// Create starter directories
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Println("Error creating directory:", err)
			return
		}
		fmt.Println("Created:", dir+"/")
	}

	// Create starter files
	files := map[string]string{
		"content/index.md":          starterIndexMd,
		"content/blog/hello.md":     starterBlogPost,
		"templates/home.html":       starterHomeTemplate,
		"templates/post.html":       starterPostTemplate,
		"templates/blog_index.html": starterBlogIndexTemplate,
		"static/styles.css":         starterCSS,
	}

	for path, content := range files {
		// Don't overwrite existing files
		if _, err := os.Stat(path); err == nil {
			fmt.Println("Skipped (exists):", path)
			continue
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			fmt.Println("Error creating file:", err)
			return
		}
		fmt.Println("Created:", path)
	}

	fmt.Println("\nProject initialized! Run `slate build` to generate your site.")
}

func serve() {
	// Check if public directory exists
	if _, err := os.Stat("public"); os.IsNotExist(err) {
		fmt.Println("Missing public/ directory. Did you run 'slate build'?")
		return
	}

	port := "8080"
	fmt.Printf("Serving public/ at http://localhost:%s\n", port)
	fmt.Println("Press Ctrl+C to stop")

	// Serve files from public/
	http.Handle("/", http.FileServer(http.Dir("public")))
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Println("Server error:", err)
	}
}

func build() {
	// Check if required directories exist
	if _, err := os.Stat("content"); os.IsNotExist(err) {
		fmt.Println("Missing content/ directory. Did you run `slate init`?")
		return
	}
	if _, err := os.Stat("templates"); os.IsNotExist(err) {
		fmt.Println("Missing templates/ directory. Did you run `slate init`?")
		return
	}

	markdownFiles, err := findMarkdownFiles("content")
	if err != nil {
		fmt.Println("Error finding markdown files:", err)
		return
	}

	fmt.Println("Found markdown files:")
	for _, file := range markdownFiles {
		fmt.Println(" -", file)
	}

	pages, err := generateHtml(markdownFiles)
	if err != nil {
		fmt.Println("Error generating HTML:", err)
		return
	}

	homeTmpl, err := template.ParseFiles("templates/home.html")
	if err != nil {
		fmt.Println("Error parsing home.html template:", err)
		return
	}

	postTmpl, err := template.ParseFiles("templates/post.html")
	if err != nil {
		fmt.Println("Error parsing post.html template:", err)
		return
	}

	blogIndexTmpl, err := template.ParseFiles("templates/blog_index.html")
	if err != nil {
		fmt.Println("Error parsing blog index template:", err)
		return
	}

	var blogPosts []Page
	var homePage *Page

	for i, page := range pages {
		if strings.HasSuffix(page.Path, "index.md") {
			homePage = &pages[i]
		} else if strings.Contains(page.Path, "/blog/") {
			blogPosts = append(blogPosts, page)
		}
	}

	// Sort blog posts by date, newest first
	sort.Slice(blogPosts, func(i, j int) bool {
		return blogPosts[i].Date.After(blogPosts[j].Date)
	})

	if homePage != nil {
		homePage.URL = "/index.html"
		if err := renderPage(homeTmpl, *homePage, "public/index.html"); err != nil {
			fmt.Println("Error rendering home page:", err)
			return
		}
	}

	// Render individual blog posts
	for _, post := range blogPosts {
		outputPath := "public" + post.URL
		if err := renderPage(postTmpl, post, outputPath); err != nil {
			fmt.Println("Error rendering blog post:", err)
			return
		}
	}

	// Render blog index
	if err := renderBlogIndex(blogIndexTmpl, blogPosts); err != nil {
		fmt.Println("Error rendering blog index:", err)
		return
	}

	// Copy static files to public
	if content, err := os.ReadFile("static/styles.css"); err == nil {
		os.WriteFile("public/styles.css", content, 0644)
		fmt.Println("Copied:", "public/styles.css")
	}
}

func renderPage(tmpl *template.Template, page Page, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := tmpl.Execute(file, page); err != nil {
		return err
	}

	fmt.Println("Generated:", outputPath)
	return nil
}

func renderBlogIndex(tmpl *template.Template, posts []Page) error {
	outputPath := "public/blog/index.html"

	if err := os.MkdirAll("public/blog", 0755); err != nil {
		return err
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := tmpl.Execute(file, posts); err != nil {
		return err
	}

	fmt.Println("Generated:", outputPath)
	return nil
}

func generateHtml(markdownFiles []string) ([]Page, error) {
	var pages []Page
	for _, file := range markdownFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}

		// Parse frontmatter and get remaining markdown
		fm, markdown := parseFrontmatter(content)

		var buf bytes.Buffer
		if err := goldmark.Convert(markdown, &buf); err != nil {
			return nil, err
		}

		// Use frontmatter title if present, otherwise extract from filename
		title := fm.Title
		if title == "" {
			title = extractTitle(file)
		}

		// Parse date from frontmatter
		var date time.Time
		if fm.Date != "" {
			// Try parsing common date formats
			date, _ = time.Parse("2006-01-02", fm.Date)
		}

		pages = append(pages, Page{
			Path:    file,
			URL:     pathToURL(file),
			Title:   title,
			Date:    date,
			Content: template.HTML(buf.String()),
		})
	}
	return pages, nil
}

// findMarkdownFiles finds and returns all .md file paths
func findMarkdownFiles(root string) ([]string, error) {
	var files []string

	// WalkDir traverses the directory tree rooted at "root"
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Println("Warning: could not access", path, "-", err)
			return nil
		}

		if d.IsDir() {
			return nil
		}

		// Check if file ends with .md (case-insensitive)
		if strings.HasSuffix(strings.ToLower(path), ".md") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// extractTitle converts a file path to a readable title
// e.g., "content/blog/my-first-post.md" → "My First Post"
func extractTitle(path string) string {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, ".md")

	// Replace underscores and hyphens with spaces
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	return strings.Title(name)
}

// pathToURL converts a content path to a web URL
// e.g., "content/blog/my-post.md" → "/blog/my-post.html"
func pathToURL(path string) string {
	// Remove "content" prefix and change extension
	url := strings.TrimPrefix(path, "content")
	url = strings.TrimSuffix(url, ".md") + ".html"
	return url
}

// parseFrontmatter extracts YAML frontmatter from markdown content
// Frontmatter is delimited by --- at the start and end
// Returns the parsed frontmatter and the remaining markdown content
func parseFrontmatter(content []byte) (Frontmatter, []byte) {
	var fm Frontmatter

	// Check if content starts with ---
	if !bytes.HasPrefix(content, []byte("---")) {
		return fm, content
	}

	// Find the closing ---
	rest := content[3:]
	endIndex := bytes.Index(rest, []byte("\n---"))
	if endIndex == -1 {
		// No closing ---, return content as-is
		return fm, content
	}

	// Extract the YAML
	yamlContent := rest[:endIndex]
	if yamlContent[0] == '\n' {
		yamlContent = yamlContent[1:]
	}

	yaml.Unmarshal(yamlContent, &fm)

	// Return the content after the closing --- +4 to skip past "\n---" and +1 more to skip the newline after it
	markdown := rest[endIndex+4:]
	if len(markdown) > 0 && markdown[0] == '\n' {
		markdown = markdown[1:]
	}

	return fm, markdown
}

//
// -------------------------- Starter Templates --------------------------
//

// Starter templates and content
const starterIndexMd = `# Welcome to My Site

This is your home page. Edit this file at content/index.md.

Check out my [blog](/blog/).
`

const starterBlogPost = `---
title: Hello World
date: 2025-01-17
---

This is your first blog post. Edit this file or create new .md files in content/blog/.
`

const starterHomeTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/styles.css">
</head>
<body>
    <main>
        {{.Content}}
    </main>
</body>
</html>
`

const starterPostTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/styles.css">
</head>
<body>
    <header>
        <nav>
            <a href="/">Home</a>
            <a href="/blog/">Blog</a>
        </nav>
    </header>
    <main>
        <h1>{{.Title}}</h1>
        {{if not .Date.IsZero}}<p class="post-date">{{.Date.Format "January 2, 2006"}}</p>{{end}}
        {{.Content}}
    </main>
</body>
</html>
`

const starterBlogIndexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Posts</title>
    <link rel="stylesheet" href="/styles.css">
</head>
<body>
    <header>
        <nav>
            <a href="/">Home</a>
        </nav>
    </header>
    <main>
        <h1>Posts</h1>
        <ul class="post-list">
            {{range .}}
            <li>
                <a href="{{.URL}}">{{.Title}}</a>
                {{if not .Date.IsZero}}<span class="post-date">{{.Date.Format "Jan 2, 2006"}}</span>{{end}}
            </li>
            {{end}}
        </ul>
    </main>
</body>
</html>
`

const starterCSS = `
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap');
@import url('https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500&display=swap');

* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

html {
    font-size: 18px;
    line-height: 1.6;
}

body {
    font-family: "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    color: #333;
    background-color: #fff;
    padding: 2rem 1rem;
}

main {
    max-width: 680px;
    margin: 0 auto;
}

header {
    max-width: 680px;
    margin: 0 auto 2rem auto;
}

nav {
    display: flex;
    gap: 1.5rem;
}

nav a {
    color: #666;
    text-decoration: none;
    font-size: 0.9rem;
}

nav a:hover {
    color: #0066cc;
}

.post-date {
    color: #666;
    font-size: 0.9rem;
    margin-bottom: 1.5rem;
}

.post-list .post-date {
    margin-left: 0.5rem;
    margin-bottom: 0;
}

h1, h2, h3, h4, h5, h6 {
    margin-top: 2rem;
    margin-bottom: 1rem;
    font-weight: 600;
    line-height: 1.3;
}

h1 {
    font-size: 2rem;
    margin-top: 0;
}

h2 {
    font-size: 1.5rem;
}

h3 {
    font-size: 1.25rem;
}

p {
    margin-bottom: 1rem;
}

ul, ol {
    margin-bottom: 1rem;
    padding-left: 1.5rem;
}

li {
    margin-bottom: 0.5rem;
}

a {
    color: #0066cc;
    text-decoration: none;
}

a:hover {
    text-decoration: underline;
}

code {
    font-family: "JetBrains Mono", monospace;
    font-size: 0.9rem;
    background-color: #f4f4f4;
    padding: 0.15rem 0.4rem;
    border-radius: 3px;
}

pre {
    background-color: #f4f4f4;
    padding: 1rem;
    border-radius: 5px;
    overflow-x: auto;
    margin-bottom: 1rem;
}

pre code {
    background: none;
    padding: 0;
}

hr {
    border: none;
    border-top: 1px solid #ddd;
    margin: 2rem 0;
}

blockquote {
    border-left: 3px solid #ddd;
    padding-left: 1rem;
    margin-left: 0;
    margin-bottom: 1rem;
    color: #666;
}

img {
    max-width: 100%;
    height: auto;
}

table {
    width: 100%;
    border-collapse: collapse;
    margin-bottom: 1rem;
}

th, td {
    border: 1px solid #ddd;
    padding: 0.5rem;
    text-align: left;
}

th {
    background-color: #f4f4f4;
}

.post-list {
    list-style: none;
    padding-left: 0;
}

.post-list li {
    padding: 0.5rem 0;
    border-bottom: 1px solid #eee;
}

.post-list li:last-child {
    border-bottom: none;
}
`
