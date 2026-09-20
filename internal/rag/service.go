package rag

import (
	"context"
	"fmt"

	"github.com/arashrasoulzadeh/appgent/internal/embedding"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db           *pgxpool.Pool
	embedClient  *embedding.Client
}

func NewService(db *pgxpool.Pool, embedClient *embedding.Client) *Service {
	return &Service{
		db:          db,
		embedClient: embedClient,
	}
}

func (s *Service) QuerySimilar(ctx context.Context, queryText string, limit int) ([]temporal.DesignPattern, error) {
	if s.embedClient == nil {
		return []temporal.DesignPattern{}, nil
	}

	queryEmbedding, err := s.embedClient.Embed(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// Convert to pgvector format
	embeddingStr := vectorToString(queryEmbedding)

	rows, err := s.db.Query(ctx, `
		SELECT kind, title, content
		FROM design_patterns
		ORDER BY embedding <=> $1
		LIMIT $2
	`, embeddingStr, limit)
	if err != nil {
		return nil, fmt.Errorf("query patterns: %w", err)
	}
	defer rows.Close()

	var patterns []temporal.DesignPattern
	for rows.Next() {
		var p temporal.DesignPattern
		if err := rows.Scan(&p.Kind, &p.Title, &p.Content); err != nil {
			return nil, err
		}
		patterns = append(patterns, p)
	}

	return patterns, rows.Err()
}

func (s *Service) AddPattern(ctx context.Context, kind, title, content string) error {
	if s.embedClient == nil {
		return fmt.Errorf("embedding client not configured")
	}

	emb, err := s.embedClient.Embed(ctx, content)
	if err != nil {
		return fmt.Errorf("embed content: %w", err)
	}

	embeddingStr := vectorToString(emb)

	_, err = s.db.Exec(ctx, `
		INSERT INTO design_patterns (kind, title, content, embedding)
		VALUES ($1, $2, $3, $4)
	`, kind, title, content, embeddingStr)
	return err
}

func (s *Service) ExtractPatternsFromRun(ctx context.Context, runID string, files map[string]string) error {
	// Extract component patterns from generated code
	for path, content := range files {
		// Simple heuristic: extract component files
		if isComponentFile(path) {
			kind := "component"
			title := path
			if err := s.AddPattern(ctx, kind, title, content); err != nil {
				// Log but continue
				fmt.Printf("Failed to add pattern %s: %v\n", title, err)
			}
		}
	}
	return nil
}

func isComponentFile(path string) bool {
	// Check if it's a React component file
	return len(path) > 0 && (path[0] >= 'A' && path[0] <= 'Z') && 
		(len(path) > 4 && path[len(path)-4:] == ".tsx")
}

func vectorToString(vec []float32) string {
	if len(vec) == 0 {
		return "[]"
	}
	result := "["
	for i, v := range vec {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf("%f", v)
	}
	result += "]"
	return result
}

// SeedInitialPatterns seeds the design_patterns table with initial patterns
func SeedInitialPatterns(ctx context.Context, db *pgxpool.Pool, embedClient *embedding.Client) error {
	service := NewService(db, embedClient)

	patterns := []struct {
		Kind    string
		Title   string
		Content string
	}{
		{
			Kind:  "layout",
			Title: "Standard Header/Main/Footer Layout",
			Content: `export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen flex flex-col">
      <header className="border-b bg-neutral-50 dark:bg-neutral-900">
        <nav className="max-w-7xl mx-auto px-4 py-4 flex justify-between items-center">
          <a href="/" className="text-xl font-bold">App</a>
          <div className="flex gap-4">
            <a href="/about" className="hover:underline">About</a>
            <a href="/contact" className="hover:underline">Contact</a>
          </div>
        </nav>
      </header>
      <main className="flex-1 max-w-7xl mx-auto px-4 py-8 w-full">
        {children}
      </main>
      <footer className="border-t bg-neutral-50 dark:bg-neutral-900 py-8">
        <div className="max-w-7xl mx-auto px-4 text-center text-neutral-600 dark:text-neutral-400">
          © 2024 App. All rights reserved.
        </div>
      </footer>
    </div>
  )
}`,
		},
		{
			Kind:  "component",
			Title: "Responsive Card Component",
			Content: `interface CardProps {
  title: string
  description: string
  image?: string
  href?: string
}

export function Card({ title, description, image, href }: CardProps) {
  const content = (
    <article className="bg-white dark:bg-neutral-800 rounded-xl shadow-sm border border-neutral-200 dark:border-neutral-700 overflow-hidden hover:shadow-md transition-shadow">
      {image && <img src={image} alt="" className="w-full h-48 object-cover" />}
      <div className="p-6">
        <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-2">{title}</h3>
        <p className="text-neutral-600 dark:text-neutral-400">{description}</p>
      </div>
    </article>
  )

  if (href) {
    return <a href={href} className="block">{content}</a>
  }
  return content
}`,
		},
		{
			Kind:  "component",
			Title: "Accessible Form with Validation",
			Content: `"use client"
import { useState } from "react"

interface FormFieldProps {
  label: string
  name: string
  type?: string
  required?: boolean
  error?: string
}

export function FormField({ label, name, type = "text", required, error }: FormFieldProps) {
  return (
    <div className="mb-4">
      <label htmlFor={name} className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
        {label} {required && <span className="text-red-500" aria-hidden="true">*</span>}
      </label>
      <input
        id={name}
        name={name}
        type={type}
        required={required}
        aria-invalid={!!error}
        aria-describedby={error ? "` + "`" + `${name}-error` + "`" + `" : undefined}
        className={"w-full px-3 py-2 border rounded-lg " + (error ? "border-red-500 focus:ring-red-500" : "border-neutral-300 dark:border-neutral-600") + " bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-primary-500 focus:border-transparent"}
      />
      {error && (
        <p id={"` + "`" + `${name}-error` + "`" + `"} className="mt-1 text-sm text-red-600 dark:text-red-400" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}`,
		},
		{
			Kind:  "copy_style",
			Title: "Professional Concise Copy Tone",
			Content: "Voice: Professional, friendly, and concise. Use active voice. Avoid jargon. Keep sentences short. Address user directly with 'you'. Use contractions naturally. Error messages should be helpful, not blaming.",
		},
		{
			Kind:  "spec_example",
			Title: "Portfolio Website Spec",
			Content: `{
  "pages": [
    {"name": "Home", "path": "/", "description": "Hero, featured projects, skills summary"},
    {"name": "Projects", "path": "/projects", "description": "Grid of all projects with filters"},
    {"name": "About", "path": "/about", "description": "Bio, experience, education"},
    {"name": "Contact", "path": "/contact", "description": "Contact form with validation"}
  ],
  "components": [
    {"name": "Header", "type": "layout", "description": "Navigation with logo and links"},
    {"name": "Footer", "type": "layout", "description": "Social links, copyright"},
    {"name": "ProjectCard", "type": "ui", "description": "Project preview with image, title, tags"},
    {"name": "ContactForm", "type": "form", "description": "Name, email, message with validation"}
  ],
  "dataModel": [
    {"name": "Project", "fields": [{"name": "id", "type": "string"}, {"name": "title", "type": "string"}, {"name": "description", "type": "string"}, {"name": "image", "type": "string"}, {"name": "tags", "type": "string[]"}, {"name": "url", "type": "string"}]}
  ],
  "styleDirection": "Clean, minimal, modern with good typography. Dark mode support. Subtle animations."
}`,
		},
	}

	for _, p := range patterns {
		if err := service.AddPattern(ctx, p.Kind, p.Title, p.Content); err != nil {
			return fmt.Errorf("seed pattern %s: %w", p.Title, err)
		}
	}

	return nil
}