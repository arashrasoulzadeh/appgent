package temporal

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type GenerateAppInput struct {
	RunID      uuid.UUID
	AppID      uuid.UUID
	AppKind    string
	UserPrompt string
}

type GenerateAppResult struct {
	Status string
	Error  string
}

type PlanInput struct {
	AppKind    string
	UserPrompt string
	RAGContext []DesignPattern
}

type PlanOutput struct {
	Pages          []PageSpec
	Components     []ComponentSpec
	DataModel      []EntitySpec
	StyleDirection string
}

type PageSpec struct {
	Name        string
	Path        string
	Description string
}

type ComponentSpec struct {
	Name        string
	Type        string
	Description string
	Props       map[string]interface{}
}

type EntitySpec struct {
	Name   string
	Fields []FieldSpec
}

type FieldSpec struct {
	Name string
	Type string
}

type DesignPattern struct {
	Kind    string
	Title   string
	Content string
}

type DesignInput struct {
	Spec PlanOutput
}

type DesignOutput struct {
	Tokens     DesignTokens
	CopyTone   string
	LayoutNotes string
}

type DesignTokens struct {
	Colors        ColorPalette
	Spacing       []string
	Typography    Typography
	BorderRadius  []string
}

type ColorPalette struct {
	Primary   ColorVariant
	Secondary ColorVariant
	Accent    ColorVariant
	Neutral   ColorVariant
	Success   ColorVariant
	Warning   ColorVariant
	Error     ColorVariant
}

type ColorVariant struct {
	Light  string
	Main   string
	Dark   string
	Contrast string
}

type Typography struct {
	FontFamily string
	FontSizes  map[string]string
	FontWeights map[string]int
	LineHeights map[string]string
}

type CodeInput struct {
	Spec          PlanOutput
	DesignTokens  *DesignTokens
	PriorFiles    map[string]string
	QAFeedback    []QAIssue
	AppKind       string
}

type CodeOutput struct {
	Files map[string]string
}

type QAInput struct {
	Files   map[string]string
	Spec    PlanOutput
	AppKind string
}

type QAOutput struct {
	Passed bool
	Issues []QAIssue
}

type QAIssue struct {
	File      string
	Line      int
	Severity  string
	Message   string
}

func GenerateAppWorkflow(ctx workflow.Context, in GenerateAppInput) (GenerateAppResult, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
			InitialInterval: 5 * time.Second,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var planOutput PlanOutput
	err := workflow.ExecuteActivity(ctx, PlanActivity, PlanInput{
		AppKind:    in.AppKind,
		UserPrompt: in.UserPrompt,
		RAGContext: []DesignPattern{},
	}).Get(ctx, &planOutput)
	if err != nil {
		return GenerateAppResult{Status: "failed", Error: err.Error()}, err
	}

	// Design and Code run in parallel (as Futures)
	designFuture := workflow.ExecuteActivity(ctx, DesignActivity, DesignInput{Spec: planOutput})
	codeFuture := workflow.ExecuteActivity(ctx, CodeActivity, CodeInput{
		Spec:          planOutput,
		DesignTokens:  nil,
		PriorFiles:    nil,
		QAFeedback:    nil,
		AppKind:       in.AppKind,
	})

	var designOutput DesignOutput
	err = designFuture.Get(ctx, &designOutput)
	if err != nil {
		return GenerateAppResult{Status: "failed", Error: err.Error()}, err
	}

	var codeOutput CodeOutput
	err = codeFuture.Get(ctx, &codeOutput)
	if err != nil {
		return GenerateAppResult{Status: "failed", Error: err.Error()}, err
	}

	// If design finished, re-run Code with design tokens
	if designOutput.Tokens.Colors != (ColorPalette{}) {
		var updatedCodeOutput CodeOutput
		err = workflow.ExecuteActivity(ctx, CodeActivity, CodeInput{
			Spec:          planOutput,
			DesignTokens:  &designOutput.Tokens,
			PriorFiles:    codeOutput.Files,
			QAFeedback:    nil,
			AppKind:       in.AppKind,
		}).Get(ctx, &updatedCodeOutput)
		if err != nil {
			return GenerateAppResult{Status: "failed", Error: err.Error()}, err
		}
		codeOutput = updatedCodeOutput
	}

	// QA loop
	maxQARetries := 3
	for attempt := 1; attempt <= maxQARetries; attempt++ {
		var qaOutput QAOutput
		err = workflow.ExecuteActivity(ctx, QAActivity, QAInput{
			Files:   codeOutput.Files,
			Spec:    planOutput,
			AppKind: in.AppKind,
		}).Get(ctx, &qaOutput)
		if err != nil {
			return GenerateAppResult{Status: "failed", Error: err.Error()}, err
		}

		if qaOutput.Passed {
			return GenerateAppResult{Status: "succeeded", Error: ""}, nil
		}

		if attempt == maxQARetries {
			return GenerateAppResult{Status: "needs_review", Error: "QA failed after max retries"}, nil
		}

		// Re-run Code with QA feedback
		var updatedCodeOutput CodeOutput
		err = workflow.ExecuteActivity(ctx, CodeActivity, CodeInput{
			Spec:          planOutput,
			DesignTokens:  &designOutput.Tokens,
			PriorFiles:    codeOutput.Files,
			QAFeedback:    qaOutput.Issues,
			AppKind:       in.AppKind,
		}).Get(ctx, &updatedCodeOutput)
		if err != nil {
			return GenerateAppResult{Status: "failed", Error: err.Error()}, err
		}
		codeOutput = updatedCodeOutput
	}

	return GenerateAppResult{Status: "needs_review", Error: "QA failed after max retries"}, nil
}

func PlanActivity(ctx context.Context, in PlanInput) (PlanOutput, error) {
	activity.RecordHeartbeat(ctx, "starting")
	// TODO: Implement real Plan activity with OpenRouter
	return PlanOutput{
		Pages: []PageSpec{
			{Name: "Home", Path: "/", Description: "Landing page"},
			{Name: "About", Path: "/about", Description: "About page"},
		},
		Components: []ComponentSpec{
			{Name: "Header", Type: "layout", Description: "Site header with navigation"},
			{Name: "Footer", Type: "layout", Description: "Site footer"},
		},
		DataModel:      []EntitySpec{},
		StyleDirection: "Clean, modern design with good typography",
	}, nil
}

func DesignActivity(ctx context.Context, in DesignInput) (DesignOutput, error) {
	activity.RecordHeartbeat(ctx, "starting")
	// TODO: Implement real Design activity with OpenRouter
	return DesignOutput{
		Tokens: DesignTokens{
			Colors: ColorPalette{
				Primary:   ColorVariant{Light: "#e3f2fd", Main: "#2196f3", Dark: "#1565c0", Contrast: "#ffffff"},
				Secondary: ColorVariant{Light: "#f3e5f5", Main: "#9c27b0", Dark: "#7b1fa2", Contrast: "#ffffff"},
				Accent:    ColorVariant{Light: "#fff3e0", Main: "#ff9800", Dark: "#f57c00", Contrast: "#ffffff"},
				Neutral:   ColorVariant{Light: "#fafafa", Main: "#757575", Dark: "#212121", Contrast: "#ffffff"},
				Success:   ColorVariant{Light: "#e8f5e9", Main: "#4caf50", Dark: "#388e3c", Contrast: "#ffffff"},
				Warning:   ColorVariant{Light: "#fff8e1", Main: "#ffc107", Dark: "#f57f17", Contrast: "#000000"},
				Error:     ColorVariant{Light: "#ffebee", Main: "#f44336", Dark: "#c62828", Contrast: "#ffffff"},
			},
			Spacing:      []string{"0", "4px", "8px", "16px", "24px", "32px", "48px", "64px"},
			Typography: Typography{
				FontFamily: "system-ui, -apple-system, sans-serif",
				FontSizes:  map[string]string{"xs": "0.75rem", "sm": "0.875rem", "base": "1rem", "lg": "1.125rem", "xl": "1.25rem", "2xl": "1.5rem", "3xl": "1.875rem", "4xl": "2.25rem"},
				FontWeights: map[string]int{"normal": 400, "medium": 500, "semibold": 600, "bold": 700},
				LineHeights: map[string]string{"tight": "1.25", "normal": "1.5", "relaxed": "1.75"},
			},
			BorderRadius: []string{"0", "2px", "4px", "8px", "12px", "16px", "9999px"},
		},
		CopyTone:   "Professional, friendly, and concise",
		LayoutNotes: "Standard layout with header, main content, and footer",
	}, nil
}

func CodeActivity(ctx context.Context, in CodeInput) (CodeOutput, error) {
	activity.RecordHeartbeat(ctx, "starting")
	// TODO: Implement real Code activity with OpenRouter
	files := map[string]string{
		"package.json": `{"name": "app", "version": "1.0.0", "scripts": {"dev": "next dev", "build": "next build", "start": "next start"}, "dependencies": {"next": "16.3.5", "react": "19.2.8", "react-dom": "19.2.8"}, "devDependencies": {"typescript": "5.9.3", "@types/react": "19.3.0", "@types/node": "20.19.43", "tailwindcss": "4.3.3"}}`,
		"next.config.js": `/** @type {import('next').NextConfig} */
const nextConfig = { output: 'export' }
module.exports = nextConfig`,
		"tsconfig.json": `{"compilerOptions": {"target": "ES2017", "lib": ["dom", "dom.iterable", "esnext"], "allowJs": true, "skipLibCheck": true, "strict": true, "noEmit": true, "esModuleInterop": true, "module": "esnext", "moduleResolution": "bundler", "resolveJsonModule": true, "isolatedModules": true, "jsx": "preserve", "incremental": true, "plugins": [{"name": "next"}], "paths": {"@/*": ["./src/*"]}}, "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"], "exclude": ["node_modules"]}`,
		"src/app/layout.tsx": `export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  )
}`,
		"src/app/page.tsx": `export default function Home() {
  return (
    <main className="min-h-screen p-8">
      <h1 className="text-4xl font-bold mb-4">Welcome to Your App</h1>
      <p className="text-lg">This is a generated app.</p>
    </main>
  )
}`,
		"src/app/globals.css": `@tailwind base; @tailwind components; @tailwind utilities;`,
	}
	if in.AppKind == "pwa" {
		files["public/manifest.json"] = `{"name": "App", "short_name": "App", "description": "Generated PWA", "start_url": "/", "display": "standalone", "background_color": "#ffffff", "theme_color": "#2196f3", "icons": [{"src": "/icon-192.png", "sizes": "192x192", "type": "image/png"}, {"src": "/icon-512.png", "sizes": "512x512", "type": "image/png"}]}`
		files["public/sw.js"] = `self.addEventListener('install', (e) => { e.waitUntil(caches.open('v1').then((cache) => cache.addAll(['/', '/manifest.json']))) })`
	}
	return CodeOutput{Files: files}, nil
}

func QAActivity(ctx context.Context, in QAInput) (QAOutput, error) {
	activity.RecordHeartbeat(ctx, "starting")
	// TODO: Implement real QA activity with build/lint checks
	return QAOutput{
		Passed: true,
		Issues: []QAIssue{},
	}, nil
}

func PersistRunResultActivity(ctx context.Context, runID uuid.UUID, result GenerateAppResult) error {
	// TODO: Persist final result to database
	return nil
}