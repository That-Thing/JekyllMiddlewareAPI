# Jekyll Middleware API

A simple REST API that serves as a middleman between users and a Jekyll blog, handling markdown file uploads and management.

## Prerequisites

- Go 1.21 or higher
- A Jekyll blog with a `_posts` directory

## Installation

1. Clone the repository:
```bash
git clone git@github.com:That-Thing/JekyllMiddlewareAPI.git
```

2. Install dependencies:
```bash
go mod tidy
```

## Configuration

The API can be configured using either environment variables or command-line flags. Command-line flags take precedence over environment variables.

### Environment Variables

Create a `.env` file in the project root:

```env
API_KEY=your-secret-key-here
POSTS_DIR=/path/to/your/jekyll/blog/_posts
PORT=8080
```

### Command-line Flags

```bash
go run main.go --posts-dir=/path/to/your/jekyll/blog/_posts --port=8080 --api-key=your-secret-key
```

## API Endpoints

### Upload a Post

```bash
POST /upload
```

Upload a markdown file with optional front matter parameters.

**Headers:**
- `X-API-Key`: Your API key

**Form Data:**
- `file`: The markdown file to upload
- `layout` (optional): The layout to use (default: "page")
- `title` (optional): The post title
- `date` (optional): The post date (default: current date)
- `categories` (optional): Comma-separated list of categories (default: ["blog"])

**Example:**
```bash
curl -X POST \
  -H "X-API-Key: your-secret-key" \
  -F "file=@your-post.md" \
  -F "title=My Blog Post" \
  -F "layout=post" \
  -F "categories=blog,tech" \
  http://localhost:8080/upload
```

### List Posts

```bash
GET /files
```

Get a list of all markdown files in the posts directory.

**Headers:**
- `X-API-Key`: Your API key

**Example:**
```bash
curl -H "X-API-Key: your-secret-key" http://localhost:8080/files
```

### Get a Post

```bash
GET /files/{filename}
```

Get the contents of a specific markdown file.

**Headers:**
- `X-API-Key`: Your API key

**Example:**
```bash
curl -H "X-API-Key: your-secret-key" http://localhost:8080/files/2024-03-26-my-post.md
```

### Delete a Post

```bash
DELETE /files/{filename}
```

Delete a specific markdown file.

**Headers:**
- `X-API-Key`: Your API key

**Example:**
```bash
curl -X DELETE -H "X-API-Key: your-secret-key" http://localhost:8080/files/2024-03-26-my-post.md
```

## Front Matter

The API automatically handles front matter for uploaded files. If a file doesn't have front matter, it will be added with the following defaults:

```yaml
---
layout: page
date: YYYY-MM-DD
categories: [blog]
---
```

If the file already has front matter, the API will preserve existing values and only add missing fields.  

If a title is not given, the filename will be used as the post title. 

## Filename Format

Uploaded files are automatically renamed to follow Jekyll's naming convention:
```
YYYY-MM-DD-title-here.md
```

## Error Handling

The API returns JSON responses with the following format:

```json
{
  "success": true|false,
  "message": "Success or error message",
  "data": {} // Optional data object
}
```