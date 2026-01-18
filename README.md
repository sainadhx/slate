# Slate

A minimal static site generator written in Go.

## Features

- Markdown to HTML
- Simple templates (Home page, Blog Index Page)
- Devserver

## Install

```
go build -o slate .
```

## Usage

### Initialize a new project

```
slate init
```

Creates the following structure:

```
content/
  index.md
  blog/
    hello.md
templates/
  base.html
  blog_index.html
static/
  styles.css
```

### Build the site

```
slate build
```

Outputs HTML to `public/`.

### Serve locally

```
slate serve
```

Serves `public/` at http://localhost:8080

