"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { useParams, useRouter } from "next/navigation"
import { api, App, Run, AgentStep, Deployment } from "@/lib/api"
import { formatDateTime, truncate, cn, apiErrorMessage } from "@/lib/utils"
import {
  ChevronLeft, RotateCcw, CheckCircle,
  Loader2, FileCode, Zap,
  Eye, Rocket, Settings, MessageSquare, Folder,
} from "lucide-react"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { ConfirmDialog } from "@/components/ui/dialog"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { Accordion, AccordionItem, AccordionTrigger, AccordionContent } from "@/components/ui/accordion"
import { useToast } from "@/hooks/use-toast"

const AGENT_LABELS: Record<string, { label: string; icon: React.ComponentType<{className?: string}> }> = {
  plan: { label: "Plan", icon: MessageSquare },
  design: { label: "Design", icon: Zap },
  code: { label: "Code", icon: FileCode },
  qa: { label: "QA", icon: CheckCircle },
  publish: { label: "Publish (build + upload)", icon: Rocket },
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
  const { toast } = useToast()

  const [app, setApp] = useState<App | null>(null)
  const [runs, setRuns] = useState<Run[]>([])
  const [selectedRun, setSelectedRun] = useState<Run | null>(null)
  const [steps, setSteps] = useState<AgentStep[]>([])
  const [deployments, setDeployments] = useState<Deployment[]>([])
  const [previewUrl, setPreviewUrl] = useState<string>("")
  const [loading, setLoading] = useState(true)
  const [regenerating, setRegenerating] = useState(false)
  const [deploying, setDeploying] = useState(false)
  const [showRegenerateModal, setShowRegenerateModal] = useState(false)
  const [regeneratePrompt, setRegeneratePrompt] = useState("")
  const [activeTab, setActiveTab] = useState("preview")

  // Read-only file browser state. runFiles is keyed by the run whose files
  // are currently loaded (filesForRunId), so switching selectedRun doesn't
  // show stale files from a previous run while the new ones are fetching.
  const [runFiles, setRunFiles] = useState<Record<string, string> | null>(null)
  const [filesForRunId, setFilesForRunId] = useState<string | null>(null)
  const [filesLoading, setFilesLoading] = useState(false)
  const [selectedFilePath, setSelectedFilePath] = useState<string | null>(null)
  const [editPrompt, setEditPrompt] = useState("")
  const [editSubmitting, setEditSubmitting] = useState(false)

  // Tracks the appId that each in-flight fetch was started for, so that
  // responses for a stale appId (e.g. the user navigated to a different
  // app while a request was still in flight) never overwrite state for
  // the currently displayed app.
  const currentAppIdRef = useRef(appId)

  const loadPreview = useCallback(async (runId: string, forAppId: string = appId) => {
    try {
      const { preview_url } = await api.getPreviewUrl(forAppId, runId)
      if (currentAppIdRef.current !== forAppId) return
      setPreviewUrl(preview_url)
    } catch (err) {
      console.error("Failed to load preview:", err)
    }
  }, [appId])

  const loadRunFiles = useCallback(async (runId: string, forAppId: string = appId) => {
    setFilesLoading(true)
    try {
      const { files } = await api.getRunFiles(forAppId, runId)
      if (currentAppIdRef.current !== forAppId) return
      setRunFiles(files)
      setFilesForRunId(runId)
      const paths = Object.keys(files).sort()
      setSelectedFilePath(paths.length > 0 ? paths[0] : null)
    } catch (err) {
      console.error("Failed to load run files:", err)
      if (currentAppIdRef.current === forAppId) {
        setRunFiles(null)
        setFilesForRunId(runId)
      }
    } finally {
      if (currentAppIdRef.current === forAppId) {
        setFilesLoading(false)
      }
    }
  }, [appId])

  const loadRunDetails = useCallback(async (runId: string, forAppId: string = appId) => {
    try {
      const { run, steps: stepsData } = await api.getRun(forAppId, runId)
      if (currentAppIdRef.current !== forAppId) return
      setSelectedRun(run)
      setSteps(stepsData)

      if (run.status === "succeeded" || run.status === "needs_review") {
        loadPreview(runId, forAppId)
      }
    } catch (err) {
      console.error("Failed to load run details:", err)
    }
  }, [appId, loadPreview])

  const loadApp = useCallback(async () => {
    const requestedAppId = appId
    try {
      const { app: appData, latest_run } = await api.getApp(requestedAppId)
      if (currentAppIdRef.current !== requestedAppId) return
      setApp(appData)
      if (latest_run) {
        setSelectedRun(latest_run)
        loadRunDetails(latest_run.id, requestedAppId)
      } else {
        setSelectedRun(null)
        setSteps([])
      }
    } catch (err) {
      console.error("Failed to load app:", err)
      if (currentAppIdRef.current === requestedAppId) {
        router.push("/dashboard")
      }
    } finally {
      if (currentAppIdRef.current === requestedAppId) {
        setLoading(false)
      }
    }
  }, [appId, router, loadRunDetails])

  const loadRuns = useCallback(async () => {
    const requestedAppId = appId
    try {
      const { runs: runsData } = await api.getRuns(requestedAppId)
      if (currentAppIdRef.current !== requestedAppId) return
      setRuns(runsData)
    } catch (err) {
      console.error("Failed to load runs:", err)
    }
  }, [appId])

  const loadDeployments = useCallback(async () => {
    const requestedAppId = appId
    try {
      const { deployments: deps } = await api.getDeployments(requestedAppId)
      if (currentAppIdRef.current !== requestedAppId) return
      setDeployments(deps)
    } catch (err) {
      console.error("Failed to load deployments:", err)
    }
  }, [appId])

  useEffect(() => {
    currentAppIdRef.current = appId
    // Standard data-fetch-on-mount/on-appId-change pattern; each loader
    // guards its setState calls behind currentAppIdRef so stale responses
    // are dropped.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    loadApp()
    loadRuns()
    loadDeployments()
  }, [appId, loadApp, loadRuns, loadDeployments])

  // Polls every 1s while work is actually in progress — the selected run
  // hasn't reached a terminal status yet (still generating/publishing), or
  // a deployment is mid-promote — so the page reflects live progress
  // (agent step statuses, run status, deployment status) without the user
  // needing to manually reload. Stops polling once everything's terminal,
  // rather than an unconditional interval running forever.
  const runInProgress = selectedRun?.status === "queued" || selectedRun?.status === "running"
  const deploymentInProgress = deployments.some((d) => d.status === "deploying")
  useEffect(() => {
    if (!runInProgress && !deploymentInProgress) return
    const interval = setInterval(() => {
      if (runInProgress && selectedRun) {
        loadRunDetails(selectedRun.id)
      }
      loadRuns()
      if (deploymentInProgress) {
        loadDeployments()
      }
    }, 1000)
    return () => clearInterval(interval)
  }, [runInProgress, deploymentInProgress, selectedRun, loadRunDetails, loadRuns, loadDeployments])

  // Lazily fetches files the first time the Files tab is opened for a given
  // run — not on every render, and not for runs the user never inspects.
  useEffect(() => {
    if (activeTab !== "files" || !selectedRun) return
    if (filesForRunId === selectedRun.id) return
    // eslint-disable-next-line react-hooks/set-state-in-effect
    loadRunFiles(selectedRun.id)
  }, [activeTab, selectedRun, filesForRunId, loadRunFiles])

  const handleRegenerate = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setRegenerating(true)
    try {
      const { run } = await api.regenerateApp(appId, regeneratePrompt || undefined)
      setShowRegenerateModal(false)
      setRegeneratePrompt("")
      setSelectedRun(run)
      loadRunDetails(run.id)
      loadRuns()
      toast({ title: "Regeneration started", description: `Version ${run.version} is now queued` })
    } catch (err) {
      toast({ title: "Failed to regenerate", description: apiErrorMessage(err, "Unknown error"), variant: "destructive" })
    } finally {
      setRegenerating(false)
    }
  }

  const handleEditFile = async (e?: React.FormEvent) => {
    e?.preventDefault()
    if (!selectedRun || !selectedFilePath || !editPrompt.trim()) return
    setEditSubmitting(true)
    try {
      const { run } = await api.editFile(appId, selectedRun.id, selectedFilePath, editPrompt.trim())
      setEditPrompt("")
      setSelectedRun(run)
      loadRunDetails(run.id)
      loadRuns()
      setActiveTab("preview")
      toast({ title: "Edit started", description: `Version ${run.version} is now queued — applying your change to ${selectedFilePath}` })
    } catch (err) {
      toast({ title: "Failed to start edit", description: apiErrorMessage(err, "Unknown error"), variant: "destructive" })
    } finally {
      setEditSubmitting(false)
    }
  }

  const handleDeploy = async () => {
    if (!selectedRun || (selectedRun.status !== "succeeded" && selectedRun.status !== "needs_review")) return
    setDeploying(true)
    try {
      await api.deployApp(appId)
      loadDeployments()
      // Deploy dispatches a workflow rather than blocking on it — it
      // rebuilds from the run's already-generated source if needed (no
      // regenerate), then promotes. The Deployments tab polls every 1s
      // and picks up the live/failed transition on its own.
      toast({ title: "Deploying", description: "Redeploying your last generation — check the Deployments tab for progress" })
    } catch (err) {
      toast({ title: "Failed to deploy", description: apiErrorMessage(err, "Unknown error"), variant: "destructive" })
    } finally {
      setDeploying(false)
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
        <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
          <TabsList className="grid w-full grid-cols-4">
            <TabsTrigger value="preview">
              <Eye className="h-4 w-4 mr-2" /> Preview
            </TabsTrigger>
            <TabsTrigger value="files">
              <Folder className="h-4 w-4 mr-2" /> Files
            </TabsTrigger>
            <TabsTrigger value="deployments">
              <Rocket className="h-4 w-4 mr-2" /> Deployments
            </TabsTrigger>
            <TabsTrigger value="history">
              <Settings className="h-4 w-4 mr-2" /> History
            </TabsTrigger>
          </TabsList>

          <TabsContent value="preview" className="mt-6">
            {selectedRun ? (
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
                      {(selectedRun.status === "succeeded" || selectedRun.status === "needs_review") && (
                        <Button
                          onClick={handleDeploy}
                          disabled={deploying}
                          size="sm"
                        >
                          <Rocket className="h-3 w-3 mr-1" /> Deploy
                        </Button>
                      )}
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setShowRegenerateModal(true)}
                        disabled={regenerating}
                      >
                        <RotateCcw className="h-3 w-3 mr-1" /> Regenerate
                      </Button>
                    </div>
                  </div>

                  <div className="border-t border-neutral-200 dark:border-neutral-800 pt-4">
                    <h3 className="text-sm font-medium text-neutral-900 dark:text-white mb-3">Agent Steps</h3>
                    <Accordion type="single" className="w-full">
                      {steps.map((step, idx) => {
                        const agentInfo = AGENT_LABELS[step.agent_type] ?? { label: step.agent_type, icon: FileCode }
                        const isActive = selectedRun.status === "running" &&
                          steps.slice(0, idx).every(s => s.status === "succeeded") &&
                          (step.status === "running" || step.status === "pending")
                        // Code generation runs as one parallel call per page
                        // plus one for shared/root files within a single
                        // attempt, so agent_type+attempt alone isn't unique
                        // — include the array index and target to keep keys
                        // stable and distinct.
                        const stepKey = `${step.agent_type}-${step.attempt}-${step.target ?? ""}-${idx}`
                        return (
                          <AccordionItem
                            key={stepKey}
                            value={stepKey}
                            className={cn(
                              "border border-neutral-200 dark:border-neutral-800",
                              isActive && "ring-2 ring-primary-500"
                            )}
                          >
                            <AccordionTrigger className={cn(
                              "flex items-center gap-3 p-3",
                              step.status === "succeeded" && "bg-green-50 dark:bg-green-900/20",
                              step.status === "failed" && "bg-red-50 dark:bg-red-900/20",
                              step.status === "running" && "bg-yellow-50 dark:bg-yellow-900/20"
                            )}>
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
                                  {step.agent_type === "code" && (
                                    <span className="text-neutral-500 dark:text-neutral-400 font-normal">
                                      {" — "}{step.target || "shared files"}
                                    </span>
                                  )}
                                </span>
                                {step.attempt > 1 && (
                                  <span className="px-1.5 py-0.5 text-xs bg-neutral-200 dark:bg-neutral-700 rounded">
                                    Attempt {step.attempt}
                                  </span>
                                )}
                              </div>
                              <span className={cn(
                                "px-2 py-0.5 text-xs font-medium rounded-full ml-auto",
                                step.status === "succeeded" && "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300",
                                step.status === "failed" && "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300",
                                step.status === "running" && "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300",
                                step.status === "pending" && "bg-neutral-100 text-neutral-800 dark:bg-neutral-800 dark:text-neutral-300",
                              )}>
                                {step.status}
                              </span>
                              <span className="text-sm text-neutral-500 dark:text-neutral-400">
                                {step.model_used}
                              </span>
                              {step.tokens_used && (
                                <span className="text-xs text-neutral-400 dark:text-neutral-500">
                                  {step.tokens_used.toLocaleString()} tokens
                                </span>
                              )}
                            </AccordionTrigger>
                            <AccordionContent className="p-3">
                              {step.output_summary && (
                                <div className="text-sm text-neutral-600 dark:text-neutral-400">
                                  {step.output_summary}
                                </div>
                              )}
                              {step.error && (
                                <div className="mt-2 text-sm text-red-600 dark:text-red-400">
                                  {step.error}
                                </div>
                              )}
                            </AccordionContent>
                          </AccordionItem>
                        )
                      })}
                    </Accordion>
                  </div>
                </div>
              </div>
            ) : runs.length === 0 ? (
              <div className="text-center py-16">
                <MessageSquare className="h-16 w-16 mx-auto text-neutral-300 dark:text-neutral-700 mb-4" />
                <h2 className="text-xl font-medium text-neutral-900 dark:text-white mb-2">No runs yet</h2>
                <p className="text-neutral-600 dark:text-neutral-400">Create your first generation run</p>
              </div>
            ) : (
              <div className="text-center py-8">
                <p className="text-neutral-600 dark:text-neutral-400">Select a run from History to view details</p>
              </div>
            )}
          </TabsContent>

          <TabsContent value="files" className="mt-6">
            {!selectedRun ? (
              <div className="text-center py-8">
                <p className="text-neutral-600 dark:text-neutral-400">Select a run from History to browse its files</p>
              </div>
            ) : filesLoading && filesForRunId !== selectedRun.id ? (
              <div className="flex items-center justify-center py-16">
                <Loader2 className="h-6 w-6 animate-spin text-neutral-400" />
              </div>
            ) : !runFiles || Object.keys(runFiles).length === 0 ? (
              <div className="text-center py-8">
                <Folder className="h-12 w-12 mx-auto text-neutral-300 dark:text-neutral-700 mb-4" />
                <p className="text-neutral-600 dark:text-neutral-400">
                  No files available for this run yet — files appear once generation has produced source.
                </p>
              </div>
            ) : (
              <div className="space-y-3">
                <p className="text-sm text-neutral-500 dark:text-neutral-400">
                  Code is read-only here. To change a file, describe what you want below — it applies to that ONE file and creates a new version to review and deploy.
                </p>
                <div className="flex gap-4 bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden" style={{ height: "70vh" }}>
                  <div className="w-64 shrink-0 overflow-y-auto border-r border-neutral-200 dark:border-neutral-800 py-2">
                    {Object.keys(runFiles).sort().map((path) => (
                      <button
                        key={path}
                        onClick={() => setSelectedFilePath(path)}
                        className={cn(
                          "w-full text-left px-3 py-1.5 text-sm truncate font-mono",
                          selectedFilePath === path
                            ? "bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                            : "text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800/50"
                        )}
                        title={path}
                      >
                        {path}
                      </button>
                    ))}
                  </div>
                  <div className="flex-1 flex flex-col min-w-0">
                    {selectedFilePath && (
                      <>
                        <div className="shrink-0 px-4 py-2 text-xs font-mono text-neutral-500 dark:text-neutral-400 bg-neutral-50 dark:bg-neutral-800/50 border-b border-neutral-200 dark:border-neutral-800">
                          {selectedFilePath}
                        </div>
                        <pre className="flex-1 overflow-auto p-4 text-sm font-mono whitespace-pre-wrap break-words text-neutral-800 dark:text-neutral-200">
                          {runFiles[selectedFilePath]}
                        </pre>
                        <form
                          onSubmit={handleEditFile}
                          className="shrink-0 flex items-center gap-2 p-3 border-t border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-800/50"
                        >
                          <input
                            type="text"
                            value={editPrompt}
                            onChange={(e) => setEditPrompt(e.target.value)}
                            placeholder={`Describe a change to ${selectedFilePath}…`}
                            disabled={editSubmitting}
                            className="flex-1 min-w-0 px-3 py-2 text-sm rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-white placeholder:text-neutral-400"
                          />
                          <Button type="submit" size="sm" disabled={editSubmitting || !editPrompt.trim()}>
                            {editSubmitting ? <Loader2 className="h-3 w-3 animate-spin" /> : "Apply"}
                          </Button>
                        </form>
                      </>
                    )}
                  </div>
                </div>
              </div>
            )}
          </TabsContent>

          <TabsContent value="deployments" className="mt-6">
            <div className="bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-800">
              <div className="p-6 border-b border-neutral-200 dark:border-neutral-800">
                <h3 className="text-lg font-semibold text-neutral-900 dark:text-white">Deployments</h3>
              </div>
              {deployments.length === 0 ? (
                <div className="p-12 text-center">
                  <Rocket className="h-12 w-12 mx-auto text-neutral-300 dark:text-neutral-700 mb-4" />
                  <h4 className="text-lg font-medium text-neutral-900 dark:text-white mb-2">No deployments yet</h4>
                  <p className="text-neutral-600 dark:text-neutral-400 mb-6">Deploy a successful run to make it live</p>
                  {(selectedRun?.status === "succeeded" || selectedRun?.status === "needs_review") && (
                    <Button onClick={handleDeploy} disabled={deploying}>
                      <Rocket className="h-4 w-4 mr-2" /> Deploy Latest Run
                    </Button>
                  )}
                </div>
              ) : (
                <div className="divide-y divide-neutral-200 dark:divide-neutral-800">
                  {deployments.map((deployment) => (
                    <div key={deployment.id} className="p-6 flex items-center justify-between">
                      <div className="flex items-center gap-4">
                        <span className={cn(
                          "px-3 py-1 text-sm font-medium rounded-full",
                          deployment.status === "live" && "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300",
                          deployment.status === "deploying" && "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300",
                          deployment.status === "failed" && "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300",
                        )}>
                          {deployment.status}
                        </span>
                        <div>
                          {deployment.url && (
                            <a href={deployment.url} target="_blank" rel="noopener noreferrer" className="text-primary-600 dark:text-primary-400 hover:underline">
                              {deployment.url}
                            </a>
                          )}
                        </div>
                      </div>
                      <div className="text-sm text-neutral-500 dark:text-neutral-400">
                        {formatDateTime(deployment.deployed_at || deployment.created_at)}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </TabsContent>

          <TabsContent value="history" className="mt-6">
            <div className="bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-800">
              <div className="p-6 border-b border-neutral-200 dark:border-neutral-800">
                <h3 className="text-lg font-semibold text-neutral-900 dark:text-white">Run History</h3>
              </div>
              {runs.length === 0 ? (
                <div className="p-12 text-center">
                  <MessageSquare className="h-12 w-12 mx-auto text-neutral-300 dark:text-neutral-700 mb-4" />
                  <h4 className="text-lg font-medium text-neutral-900 dark:text-white mb-2">No runs yet</h4>
                  <p className="text-neutral-600 dark:text-neutral-400">Create your first generation run from the dashboard</p>
                </div>
              ) : (
                <div className="divide-y divide-neutral-200 dark:divide-neutral-800">
                  {runs.map((run) => (
                    <div key={run.id} className="p-6 flex items-center justify-between hover:bg-neutral-50 dark:hover:bg-neutral-800/50 cursor-pointer"
                      onClick={() => { setSelectedRun(run); loadRunDetails(run.id); setActiveTab("preview") }}>
                      <div className="flex items-center gap-4 flex-1">
                        <span className={cn(
                          "px-3 py-1 text-sm font-medium rounded-full",
                          STATUS_COLORS[run.status]
                        )}>
                          {run.status.replace("_", " ")}
                        </span>
                        <div>
                          <p className="font-medium text-neutral-900 dark:text-white">v{run.version}</p>
                          <p className="text-sm text-neutral-500 dark:text-neutral-400 truncate max-w-xs">
                            {truncate(run.user_prompt, 80)}
                          </p>
                        </div>
                      </div>
                      <div className="flex items-center gap-4 text-sm text-neutral-500 dark:text-neutral-400">
                        <span>{formatDateTime(run.created_at)}</span>
                        {run.finished_at && (
                          <span>Duration: {Math.round((new Date(run.finished_at).getTime() - new Date(run.created_at).getTime()) / 1000)}s</span>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </TabsContent>
        </Tabs>
      </main>

      <ConfirmDialog
        open={showRegenerateModal}
        onOpenChange={setShowRegenerateModal}
        title="Regenerate App"
        description="This will create a new version and start a new generation run. You can provide a new prompt or leave empty to re-run the last one."
        confirmText="Regenerate"
        cancelText="Cancel"
        onConfirm={handleRegenerate}
        loading={regenerating}
      >
        <form onSubmit={handleRegenerate} className="space-y-4">
          <div>
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
          <p className="text-sm text-neutral-500 dark:text-neutral-400">
            This will create version v{runs.length > 0 ? runs[0].version + 1 : 1}
          </p>
        </form>
      </ConfirmDialog>
    </div>
  )
}