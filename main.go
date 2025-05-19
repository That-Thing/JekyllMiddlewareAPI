package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/rs/cors"
)

var (
	postsDir     string
	maxFileSize  int64 = 10 << 20 // 10mb | Should never really be that much but just in case.
	apiKeyHeader       = "X-API-Key"
)

type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type FileInfo struct {
	Filename    string    `json:"filename"`
	Size        int64     `json:"size"`
	UploadTime  time.Time `json:"upload_time"`
	ContentType string    `json:"content_type"`
}

type UploadOptions struct {
	Layout     string   `json:"layout"`
	Title      string   `json:"title"`
	Date       string   `json:"date"`
	Categories []string `json:"categories"`
}

func init() {
	// Define command-line flags
	postsDirFlag := flag.String("posts-dir", "", "Path to the Jekyll _posts directory")
	portFlag := flag.String("port", "", "Port to run the server on")
	apiKeyFlag := flag.String("api-key", "", "API key for authentication")

	// Parse flags
	flag.Parse()

	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	// Set values with priority: command-line flags > environment variables > defaults
	postsDir = *postsDirFlag
	if postsDir == "" {
		postsDir = getEnv("POSTS_DIR", filepath.Join(os.Getenv("HOME"), "blog", "_posts"))
	}

	// Set API key if provided via flag
	if *apiKeyFlag != "" {
		os.Setenv("API_KEY", *apiKeyFlag)
	}

	// Set port if provided via flag
	if *portFlag != "" {
		os.Setenv("PORT", *portFlag)
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func processMarkdownFile(content []byte, options UploadOptions, originalFilename string) ([]byte, error) {
	// Get current date
	currentDate := time.Now().Format("2006-01-02")

	// Default values
	if options.Layout == "" {
		options.Layout = "page"
	}
	if options.Date == "" {
		options.Date = currentDate
	}
	if len(options.Categories) == 0 {
		options.Categories = []string{"blog"}
	}
	// Use filename as title if no title provided
	if options.Title == "" {
		// Remove .md extension and clean the filename
		baseName := strings.TrimSuffix(originalFilename, ".md")
		// Convert to title case and replace hyphens with spaces
		options.Title = strings.Title(strings.ReplaceAll(baseName, "-", " "))
	}

	// Check if file already has front matter
	scanner := bufio.NewScanner(bytes.NewReader(content))
	hasFrontMatter := false
	if scanner.Scan() && strings.TrimSpace(scanner.Text()) == "---" {
		hasFrontMatter = true
	}

	var newContent bytes.Buffer
	if !hasFrontMatter {
		// Add front matter
		newContent.WriteString("---\n")
		newContent.WriteString(fmt.Sprintf("layout: %s\n", options.Layout))
		newContent.WriteString(fmt.Sprintf("title: %s\n", options.Title))
		newContent.WriteString(fmt.Sprintf("date: %s\n", options.Date))
		newContent.WriteString(fmt.Sprintf("categories: [%s]\n", strings.Join(options.Categories, ", ")))
		newContent.WriteString("---\n\n")
		newContent.Write(content)
		return newContent.Bytes(), nil
	}

	// File has front matter, check and update if needed
	var frontMatter bytes.Buffer
	var contentAfterFrontMatter bytes.Buffer
	inFrontMatter := true
	hasLayout := false
	hasTitle := false
	hasDate := false
	hasCategories := false

	scanner = bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if inFrontMatter {
			if line == "---" {
				inFrontMatter = false
				frontMatter.WriteString(line + "\n")
				continue
			}
			if strings.HasPrefix(line, "layout:") {
				hasLayout = true
			}
			if strings.HasPrefix(line, "title:") {
				hasTitle = true
			}
			if strings.HasPrefix(line, "date:") {
				hasDate = true
			}
			if strings.HasPrefix(line, "categories:") {
				hasCategories = true
			}
			frontMatter.WriteString(line + "\n")
		} else {
			contentAfterFrontMatter.WriteString(line + "\n")
		}
	}

	// Add missing front matter fields
	if !hasLayout {
		frontMatter.WriteString(fmt.Sprintf("layout: %s\n", options.Layout))
	}
	if !hasTitle {
		frontMatter.WriteString(fmt.Sprintf("title: %s\n", options.Title))
	}
	if !hasDate {
		frontMatter.WriteString(fmt.Sprintf("date: %s\n", options.Date))
	}
	if !hasCategories {
		frontMatter.WriteString(fmt.Sprintf("categories: [%s]\n", strings.Join(options.Categories, ", ")))
	}

	// Combine everything
	newContent.WriteString("---\n")
	newContent.Write(frontMatter.Bytes())
	newContent.Write(contentAfterFrontMatter.Bytes())
	return newContent.Bytes(), nil
}

func formatFilename(originalName string, title string, date string) string {
	// Remove .md extension
	baseName := strings.TrimSuffix(originalName, ".md")

	// If no title provided, use the original filename
	if title == "" {
		title = baseName
	}

	// Clean the title: lowercase, replace spaces with hyphens, remove special characters
	title = strings.ToLower(title)
	title = regexp.MustCompile(`[^a-z0-9\s-]`).ReplaceAllString(title, "")
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, "-")
	title = strings.Trim(title, "-")

	// If no date provided, use current date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	// Combine date and title
	return fmt.Sprintf("%s-%s.md", date, title)
}

func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	// Check API key
	if !validateAPIKey(r) {
		respondWithError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse multipart form
	err := r.ParseMultipartForm(maxFileSize)
	if err != nil {
		respondWithError(w, "File too large", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		respondWithError(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file type
	if !strings.HasSuffix(strings.ToLower(handler.Filename), ".md") {
		respondWithError(w, "Only markdown files are allowed", http.StatusBadRequest)
		return
	}

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, "Error reading file", http.StatusInternalServerError)
		return
	}

	// Parse upload options
	options := UploadOptions{
		Layout:     r.FormValue("layout"),
		Title:      r.FormValue("title"),
		Date:       r.FormValue("date"),
		Categories: strings.Split(r.FormValue("categories"), ","),
	}

	// Process markdown content
	processedContent, err := processMarkdownFile(content, options, handler.Filename)
	if err != nil {
		respondWithError(w, "Error processing file", http.StatusInternalServerError)
		return
	}

	// Format the filename
	newFilename := formatFilename(handler.Filename, options.Title, options.Date)

	// Create destination file
	dst, err := os.Create(filepath.Join(postsDir, newFilename))
	if err != nil {
		respondWithError(w, "Error creating file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Write processed content
	if _, err := dst.Write(processedContent); err != nil {
		respondWithError(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	respondWithSuccess(w, "File uploaded successfully", FileInfo{
		Filename:    newFilename,
		Size:        int64(len(processedContent)),
		UploadTime:  time.Now(),
		ContentType: handler.Header.Get("Content-Type"),
	})
}

func listFiles(w http.ResponseWriter, r *http.Request) {
	if !validateAPIKey(r) {
		respondWithError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	files, err := os.ReadDir(postsDir)
	if err != nil {
		respondWithError(w, "Error reading directory", http.StatusInternalServerError)
		return
	}

	var fileInfos []FileInfo
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".md") {
			info, err := file.Info()
			if err != nil {
				continue
			}
			fileInfos = append(fileInfos, FileInfo{
				Filename:    file.Name(),
				Size:        info.Size(),
				UploadTime:  info.ModTime(),
				ContentType: "text/markdown",
			})
		}
	}

	respondWithSuccess(w, "Files retrieved successfully", fileInfos)
}

func getFile(w http.ResponseWriter, r *http.Request) {
	if !validateAPIKey(r) {
		respondWithError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	filename := vars["filename"]

	filePath := filepath.Join(postsDir, filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		respondWithError(w, "File not found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, filePath)
}

func deleteFile(w http.ResponseWriter, r *http.Request) {
	if !validateAPIKey(r) {
		respondWithError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	filename := vars["filename"]

	filePath := filepath.Join(postsDir, filename)
	if err := os.Remove(filePath); err != nil {
		respondWithError(w, "Error deleting file", http.StatusInternalServerError)
		return
	}

	respondWithSuccess(w, "File deleted successfully", nil)
}

func validateAPIKey(r *http.Request) bool {
	apiKey := r.Header.Get(apiKeyHeader)
	return apiKey == os.Getenv("API_KEY")
}

func respondWithError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(Response{
		Success: false,
		Message: message,
	})
}

func respondWithSuccess(w http.ResponseWriter, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func main() {
	// Create necessary directory
	os.MkdirAll(postsDir, 0755)

	// Initialize router
	r := mux.NewRouter()

	// API routes
	r.HandleFunc("/upload", handleFileUpload).Methods("POST")
	r.HandleFunc("/files", listFiles).Methods("GET")
	r.HandleFunc("/files/{filename}", getFile).Methods("GET")
	r.HandleFunc("/files/{filename}", deleteFile).Methods("DELETE")

	// CORS configuration
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "X-API-Key"},
		AllowCredentials: true,
	})

	// Start server
	port := getEnv("PORT", "8080")

	fmt.Printf("Server starting on port %s...\n", port)
	fmt.Printf("Posts directory: %s\n", postsDir)
	log.Fatal(http.ListenAndServe(":"+port, c.Handler(r)))
}
