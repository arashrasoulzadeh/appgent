"use client"

import { useEffect, useState } from "react"
import { useParams, useRouter } from "next/navigation"
import { api, App, Run, AgentStep, Deployment } from "@/lib/api"
import { formatDate, formatDateTime, truncate, cn } from "@/lib/utils"
import { 
  ChevronLeft, ChevronRight, Play, RotateCcw, 
  ExternalLink, Download, AlertCircle, CheckCircle,
  Loader2, XCircle, FileCode, Clock, Zap,
  Eye, Rocket, Settings, MessageSquare
} from "lucide-react"
import Link from "next/link"

const AGENT_LABELS: Record<string, { label: string; icon: React.ComponentType<{className?: string}> }> = {
  plan: { label: "Plan", icon: MessageSquare },
  design: { label: "Design", icon: Zap },
  code: { label: "Code", icon: FileCode },
  qa: { label: "QA", icon: CheckCircle },
}

const STATUS_COLORS: Record<string, string> = {
  succeeded: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300",
  running: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300",
  queued: "bg-neutral-100 text-neutral-800 dark:bg-neutral-800 dark:text-neutral-300",
  needs_review: "bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-300",
  failed: "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300",
}

export default function AppDetailPage() {
  const params = useParams()
  const router = useRouter()
  const appId = params.appId as string

  const [app, setApp] = useState<App | null>(null)
  const [runs, setRuns] = useState<Run[]>([])
  const [selectedRun, setSelectedRun] = useState<Run | null>(null)
  const [steps, setSteps] = useState<AgentStep[]>([])
  const [deployments, setDeployments] = useState<Deployment[]>([])
  const [previewUrl, setPreviewUrl] = useState<string>("")
  const [previewExpiry, setPreviewExpiry] = useState<string>("")
  const [loading, setLoading] = useState(true)
  const [regenerating, setRegenerating] = useState(false)
  const [deploying, setDeploying] = useState(false)
  const [showRegenerateModal, setShowRegenerateModal] = useState(false)
  const [regeneratePrompt, setRegeneratePrompt] = useState("")

  useEffect(() => {
    loadApp()
    loadRuns()
    loadDeployments()
  }, [appId])

  const loadApp = async () => {
    try {
      const { app: appData, latest_run } = await api.getApp(appId)
      setApp(appData)
      if (latest_run) {
        setSelectedRun(latest_run)
        loadRunDetails(latest_run.id)
      }
    } catch (err) {
      console.error("Failed to load app:", err)
      router.push("/dashboard")
    } finally {
      setLoading(false)
    }
  }

  const loadRuns = async () => {
    try {
      const { runs: runsData } = await api.getRuns(appId)
      setRuns(runsData)
    } catch (err) {
      console.error("Failed to load runs:", err)
    }
  }

  const loadRunDetails = async (runId: string) => {
    try {
      const { run, steps: stepsData } = await api.getRun(appId, runId)
      setSelectedRun(run)
      setSteps(stepsData)
      
      if (run.status === "succeeded" || run.status === "needs_review") {
        loadPreview(runId)
      }
    } catch (err) {
      console.error("Failed to load run details:", err)
    }
  }

  const loadDeployments = async () => {
    try {
      const { deployments: deps } = await api.getDeployments(appId)
      setDeployments(deps)
    } catch (err) {
      console.error("Failed to load deployments:", err)
    }
  }

  const loadPreview = async (runId: string) => {
    try {
      const { preview_url, expires_at } = await api.getPreviewUrl(appId, runId)
      setPreviewUrl(preview_url)
      setPreviewExpiry(expires_at)
    } catch (err) {
      console.error("Failed to load preview:", err)
    }
  }

  const handleRegenerate = async (e: React.FormEvent) => {
    e.preventDefault()
    setRegenerating(true)
    try {
      const { run } = await api.regenerateApp(appId, regeneratePrompt || undefined)
      setShowRegenerateModal(false)
      setRegeneratePrompt("")
      setSelectedRun(run)
      loadRunDetails(run.id)
      loadRuns()
    } catch (err: any) {
      alert(err.response?.data?.error || "Failed to regenerate")
    } finally {
      setRegenerating(false)
    }
  }

  const handleDeploy = async () => {
    if (!selectedRun || selectedRun.status !== "succeeded") return
    setDeploying(true)
    try {
      await api.deployApp(appId)
      loadDeployments()
      alert("Deployment started! Check the Deployments tab.")
    } catch (err: any) {
      alert(err.response?.data?.error || "Failed to deploy")
    } finally {
      setDeploying(false)
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case "succeeded": return <CheckCircle className="h-4 w-4 text-green-600" />
      case "running": return <Loader2 className="h-4 w-4 text-yellow-600 animate-spin" />
      case "queued": return <Clock className="h-4 w-4 text-neutral-600" />
      case "needs_review": return <AlertCircle className="h-4 w-4 text-orange-600" />
      case "failed": return <XCircle className="h-4 w-4 text-red-600" />
      default: return <Clock className="h-4 w-4 text-neutral-600" />
    }
  }

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-primary-600" />
      </div>
    )
  }

  if (!app) return null

  return (
    <div className="min-h-screen bg-neutral-50 dark:bg-neutral-950">
      <header className="bg-white dark:bg-neutral-900 border-b border-neutral-200 dark:border-neutral-800 sticky top-0 z-40">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between h-16">
            <div className="flex items-center gap-4">
              <Link 
                href="/dashboard" 
                className="flex items-center gap-2 text-neutral-600 dark:text-neutral-400 hover:text-primary-600"
              >
                <ChevronLeft className="h-5 w-5" />
                <span className="font-medium">Back to Dashboard</span>
              </Link>
              <div>
                <h1 className="text-xl font-bold text-neutral-900 dark:text-white">{app.name}</h1>
                <p className="text-sm text-neutral-500 dark:text-neutral-400">{app.slug}</p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <span className={cn("px-2 py-1 text-xs font-medium rounded-full", STATUS_COLORS[app.status])}>
                {app.status.replace("_", " ")}
              </span>
              <span className="px-2 py-1 text-xs font-medium rounded-full bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300">
                {app.kind.toUpperCase()}
              </span>
            </div>
          </div>
        </div>
      </header>

      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        <div className="mb-6">
          <nav className="flex gap-1 bg-neutral-100 dark:bg-neutral-800 rounded-lg p-1" role="tablist">
            <button
              onClick={() => setSelectedRun(selectedRun || runs[0] || null)}
              className={cn(
                "px-4 py-2 rounded-md text-sm font-medium transition-colors",
                selectedRun ? "bg-white dark:bg-neutral-900 text-primary-600 dark:text-primary-400 shadow-sm"
                  : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white"
              )}
              role="tab"
            >
              <Eye className="h-4 w-4 inline mr-1" /> Preview
            </button>
            <button
              className={cn(
                "px-4 py-2 rounded-md text-sm font-medium transition-colors",
                "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white"
              )}
              role="tab"
            >
              <Rocket className="h-4 w-4 inline mr-1" /> Deployments
            </button>
            <button
              className={cn(
                "px-4 py-2 rounded-md text-sm font-medium transition-colors",
                "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white"
              )}
              role="tab"
            >
              <Settings className="h-4 w-4 inline mr-1" /> History
            </button>
          </nav>
        </div>

        {selectedRun && (
          <div className="space-y-6">
            <div className="bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-800 p-6">
              <div className="flex items-center justify-between mb-4">
                <div>
                  <h2 className="text-lg font-semibold text-neutral-900 dark:text-white">
                    Run v{selectedRun.version}
                  </h2>
                  <p className="text-sm text-neutral-600 dark:text-neutral-400">
                    {truncate(selectedRun.user_prompt, 100)}
                  </p>
                </div>
                <div className="flex items-center gap-3">
                  <span className={cn("px-3 py-1 rounded-full text-sm font-medium", STATUS_COLORS[selectedRun.status])}>
                    {selectedRun.status.replace("_", " ")}
                    {selectedRun.status === "running" && <Loader2 className="h-3 w-3 ml-1 animate-spin" />}
                  </span>
                  {selectedRun.status === "succeeded" && previewUrl && (
                    <a
                      href={previewUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 px-3 py-1 text-sm text-primary-600 dark:text-primary-400 hover:text-primary-700 dark:hover:text-primary-300"
                    >
                      <Eye className="h-3 w-3" /> View Preview
                    </a>
                  )}
                  {selectedRun.status === "succeeded" && (
                    <button
                      onClick={handleDeploy}
                      disabled={deploying}
                      className="inline-flex items-center gap-1 px-3 py-1 text-sm bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50"
                    >
                      <Rocket className="h-3 w-3" /> Deploy
                    </button>
                  )}
                  <button
                    onClick={() => setShowRegenerateModal(true)}
                    disabled={regenerating}
                    className="inline-flex items-center gap-1 px-3 py-1 text-sm border border-neutral-300 dark:border-neutral-600 rounded-lg text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50"
                  >
                    <RotateCcw className="h-3 w-3" /> Regenerate
                  </button>
                </div>
              </div>

              <div className="border-t border-neutral-200 dark:border-neutral-800 pt-4">
                <h3 className="text-sm font-medium text-neutral-900 dark:text-white mb-3">Agent Steps</h3>
                <div className="space-y-2">
                  {steps.map((step, idx) => {
                    const agentInfo = AGENT_LABELS[step.agent_type]
                    const isActive = selectedRun.status === "running" && 
                      steps.slice(0, idx).every(s => s.status === "succeeded") &&
                      (step.status === "running" || step.status === "pending")
                    return (
                      <div
                        key={`${step.agent_type}-${step.attempt}`}
                        className={cn(
                          "flex items-center gap-3 p-3 rounded-lg",
                          step.status === "succeeded" && "bg-green-50 dark:bg-green-900/20",
                          step.status === "failed" && "bg-red-50 dark:bg-red-900/20",
                          step.status === "running" && "bg-yellow-50 dark:bg-yellow-900/20",
                          isActive && "ring-2 ring-primary-500"
                        )}
                      >
                        <div className="flex items-center gap-2">
                          <agentInfo.icon className={cn(
                            "h-5 w-5",
                            step.status === "succeeded" && "text-green-600",
                            step.status === "failed" && "text-red-600",
                            step.status === "running" && "text-yellow-600 animate-spin",
                            step.status === "pending" && "text-neutral-400"
                          )} />
                          <span className="font-medium text-neutral-900 dark:text-white">
                            {agentInfo.label}
                          </span>
                          {step.attempt > 1 && (
                            <span className="px-1.5 py-0.5 text-xs bg-neutral-200 dark:bg-neutral-700 rounded">
                              Attempt {step.attempt}
                            </span>
                          )}
                        </div>
                        <span className={cn(
                          "px-2 py-0.5 text-xs font-medium rounded-full",
                          step.status === "succeeded" && "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300",
                          step.status === "failed" && "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300",
                          step.status === "running" && "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300",
                          step.status === "pending" && "bg-neutral-100 text-neutral-800 dark:bg-neutral-800 dark:text-neutral-300",
                        )}>
                          {step.status}
                        </span>
                        <span className="text-sm text-neutral-500 dark:text-neutral-400 ml-auto">
                          {step.model_used}
                        </span>
                        {step.tokens_used && (
                          <span className="text-xs text-neutral-400 dark:text-neutral-500">
                            {step.tokens_used.toLocaleString()} tokens
                          </span>
                        )}
                        {step.output_summary && (
                          <div className="text-sm text-neutral-600 dark:text-neutral-400 ml-7 mt-1">
                            {step.output_summary}
                          </div>
                        )}
                      </div>
                    )
                  })}
                </div>
              </div>
            </div>
          </div>
        )}

        {!selectedRun && runs.length === 0 && (
          <div className="text-center py-16">
            <MessageSquare className="h-16 w-16 mx-auto text-neutral-300 dark:text-neutral-700 mb-4" />
            <h2 className="text-xl font-medium text-neutral-900 dark:text-white mb-2">No runs yet</h2>
            <p className="text-neutral-600 dark:text-neutral-400">Create your first generation run</p>
          </div>
        )}
      </main>

      {showRegenerateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="bg-white dark:bg-neutral-900 rounded-xl max-w-md w-full p-6">
            <h2 className="text-xl font-bold text-neutral-900 dark:text-white mb-6">Regenerate App</h2>
            <form onSubmit={handleRegenerate}>
              <div className="mb-4">
                <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
                  New Prompt (optional)
                </label>
                <textarea
                  value={regeneratePrompt}
                  onChange={(e) => setRegeneratePrompt(e.target.value)}
                  rows={4}
                  className="w-full px-3 py-2 border border-neutral-300 dark:border-neutral-600 rounded-lg bg-white dark:bg-neutral-800 text-neutral-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent resize-none"
                  placeholder="Leave empty to re-run last prompt, or enter new instructions..."
                />
              </div>
              <p className="text-sm text-neutral-500 dark:text-neutral-400 mb-4">
                This will create a new version (v{runs.length > 0 ? runs[0].version + 1 : 1}) and start a new generation run.
              </p>
              <div className="flex gap-3">
                <button
                  type="button"
                  onClick={() => setShowRegenerateModal(false)}
                  className="flex-1 px-4 py-2 border border-neutral-300 dark:border-neutral-600 rounded-lg text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={regenerating}
                  className="flex-1 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {regenerating ? <Loader2 className="h-4 w-4 animate-spin mx-auto" /> : "Regenerate"}
                </button>
              </div>
            </form>
          </div>
        )}
      )}
    </div>
  )
}