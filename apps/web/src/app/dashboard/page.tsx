"use client"

import { useCallback, useEffect, useState } from "react"
import { useRouter } from "next/navigation"
import { api, App } from "@/lib/api"
import { formatDate, getInitials, cn, apiErrorMessage } from "@/lib/utils"
import { Plus, LayoutDashboard } from "lucide-react"
import Link from "next/link"

// Keep in sync with services.MaxAppsPerUser (internal/services/app_service.go).
const MAX_APPS = 10

export default function DashboardPage() {
  const router = useRouter()
  const [apps, setApps] = useState<App[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [createForm, setCreateForm] = useState({ name: "", kind: "website" as "website" | "pwa", prompt: "" })
  const [createLoading, setCreateLoading] = useState(false)
  const [error, setError] = useState("")
  const [loadError, setLoadError] = useState("")

  const loadApps = useCallback(async () => {
    setLoadError("")
    try {
      const { apps } = await api.getApps()
      setApps(apps)
    } catch (err) {
      // A 401 here is already handled globally by the axios response
      // interceptor in lib/api.ts, which redirects to /login. Any other
      // error (network failure, 5xx, etc.) needs its own user-facing state
      // so the page doesn't just look permanently empty/loading.
      console.error("Failed to load apps:", err)
      setLoadError("Failed to load your apps. Please try again.")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    // Standard data-fetch-on-mount pattern; loadApps' own setState calls are safe.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    loadApps()
  }, [loadApps])

  // Polls every 1s while any app is still generating, so status badges
  // (generating -> ready/needs_review/failed) update on their own instead
  // of requiring a manual reload. Stops once nothing's in progress.
  const anyGenerating = apps.some((a) => a.status === "generating")
  useEffect(() => {
    if (!anyGenerating) return
    const interval = setInterval(loadApps, 1000)
    return () => clearInterval(interval)
  }, [anyGenerating, loadApps])

  const handleCreateApp = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreateLoading(true)
    setError("")

    try {
      const { app } = await api.createApp(createForm)
      setShowCreateModal(false)
      setCreateForm({ name: "", kind: "website", prompt: "" })
      router.push(`/dashboard/${app.id}`)
    } catch (err) {
      setError(apiErrorMessage(err, "Failed to create app"))
    } finally {
      setCreateLoading(false)
    }
  }

  const handleLogout = async () => {
    try {
      await api.logout()
    } catch (err) {
      console.error("Failed to log out:", err)
    } finally {
      router.push("/login")
    }
  }

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-neutral-50 dark:bg-neutral-950">
      <header className="bg-white dark:bg-neutral-900 border-b border-neutral-200 dark:border-neutral-800">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between h-16">
            <div className="flex items-center gap-8">
              <Link href="/dashboard" className="flex items-center gap-2">
                <LayoutDashboard className="h-6 w-6 text-primary-600 dark:text-primary-400" />
                <span className="text-xl font-bold text-neutral-900 dark:text-white">Appgent</span>
              </Link>
              <nav className="hidden md:flex items-center gap-6">
                <Link href="/dashboard" className="text-sm font-medium text-neutral-700 dark:text-neutral-300 hover:text-primary-600 dark:hover:text-primary-400">
                  Dashboard
                </Link>
              </nav>
            </div>
            <div className="flex items-center gap-4">
              <button
                onClick={handleLogout}
                className="text-sm text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white"
              >
                Log out
              </button>
            </div>
          </div>
        </div>
      </header>

      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div className="flex items-center justify-between mb-8">
          <h1 className="text-3xl font-bold text-neutral-900 dark:text-white">Your Apps</h1>
          <button
            onClick={() => setShowCreateModal(true)}
            disabled={apps.length >= MAX_APPS}
            title={apps.length >= MAX_APPS ? `You've reached the limit of ${MAX_APPS} apps` : undefined}
            className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-primary-600"
          >
            <Plus className="h-4 w-4" />
            New App
          </button>
        </div>

        {apps.length >= MAX_APPS && (
          <div className="mb-6 p-4 bg-neutral-100 dark:bg-neutral-800 border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 rounded-lg text-sm">
            You&apos;ve reached the limit of {MAX_APPS} apps. Delete one to create another.
          </div>
        )}

        {loadError && (
          <div className="mb-6 p-4 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 rounded-lg text-sm flex items-center justify-between">
            <span>{loadError}</span>
            <button
              onClick={loadApps}
              className="font-medium underline hover:no-underline"
            >
              Retry
            </button>
          </div>
        )}

        {apps.length === 0 && !loadError ? (
          <div className="text-center py-16">
            <LayoutDashboard className="h-16 w-16 mx-auto text-neutral-300 dark:text-neutral-700 mb-4" />
            <h2 className="text-xl font-medium text-neutral-900 dark:text-white mb-2">No apps yet</h2>
            <p className="text-neutral-600 dark:text-neutral-400 mb-6">Create your first app to get started</p>
            <button
              onClick={() => setShowCreateModal(true)}
              className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors"
            >
              <Plus className="h-4 w-4" />
              Create App
            </button>
          </div>
        ) : (
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {apps.map((app) => (
              <Link
                key={app.id}
                href={`/dashboard/${app.id}`}
                className="group bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-800 p-6 hover:border-primary-500 dark:hover:border-primary-500 transition-colors"
              >
                <div className="flex items-start justify-between mb-4">
                  <div className="w-12 h-12 rounded-lg bg-primary-100 dark:bg-primary-900/30 flex items-center justify-center">
                    <span className="text-xl font-bold text-primary-700 dark:text-primary-300">
                      {getInitials(app.name)}
                    </span>
                  </div>
                  <span className={cn(
                    "px-2 py-1 text-xs font-medium rounded-full",
                    app.kind === "pwa"
                      ? "bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300"
                      : "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300"
                  )}>
                    {app.kind.toUpperCase()}
                  </span>
                </div>
                <h3 className="text-lg font-semibold text-neutral-900 dark:text-white group-hover:text-primary-600 dark:group-hover:text-primary-400 transition-colors">
                  {app.name}
                </h3>
                <p className="text-sm text-neutral-600 dark:text-neutral-400 mt-1 line-clamp-2">
                  {app.slug}
                </p>
                <div className="mt-4 flex items-center justify-between text-sm text-neutral-500 dark:text-neutral-500">
                  <span className={cn(
                    "px-2 py-0.5 rounded-full",
                    app.status === "ready" && "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300",
                    app.status === "generating" && "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300",
                    app.status === "needs_review" && "bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-300",
                    app.status === "failed" && "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300",
                    app.status === "draft" && "bg-neutral-100 text-neutral-800 dark:bg-neutral-800 dark:text-neutral-300",
                  )}>
                    {app.status.replace("_", " ")}
                  </span>
                  <span>{formatDate(app.created_at)}</span>
                </div>
              </Link>
            ))}
          </div>
        )}

        {showCreateModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
            <div className="bg-white dark:bg-neutral-900 rounded-xl max-w-md w-full p-6">
              <h2 className="text-xl font-bold text-neutral-900 dark:text-white mb-6">Create New App</h2>
              <form onSubmit={handleCreateApp}>
                {error && (
                  <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 rounded-lg text-sm">
                    {error}
                  </div>
                )}
                <div className="mb-4">
                  <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
                    App Name
                  </label>
                  <input
                    type="text"
                    value={createForm.name}
                    onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
                    className="w-full px-3 py-2 border border-neutral-300 dark:border-neutral-600 rounded-lg bg-white dark:bg-neutral-800 text-neutral-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                    placeholder="My Portfolio"
                    required
                  />
                </div>
                <div className="mb-4">
                  <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
                    Type
                  </label>
                  <select
                    value={createForm.kind}
                    onChange={(e) => setCreateForm({ ...createForm, kind: e.target.value as "website" | "pwa" })}
                    className="w-full px-3 py-2 border border-neutral-300 dark:border-neutral-600 rounded-lg bg-white dark:bg-neutral-800 text-neutral-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                  >
                    <option value="website">Website</option>
                    <option value="pwa">PWA (Offline-capable)</option>
                  </select>
                </div>
                <div className="mb-6">
                  <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
                    Prompt
                  </label>
                  <textarea
                    value={createForm.prompt}
                    onChange={(e) => setCreateForm({ ...createForm, prompt: e.target.value })}
                    rows={4}
                    className="w-full px-3 py-2 border border-neutral-300 dark:border-neutral-600 rounded-lg bg-white dark:bg-neutral-800 text-neutral-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent resize-none"
                    placeholder="A portfolio site for a photographer, dark theme, gallery + contact form"
                    required
                  />
                </div>
                <div className="flex gap-3">
                  <button
                    type="button"
                    onClick={() => setShowCreateModal(false)}
                    className="flex-1 px-4 py-2 border border-neutral-300 dark:border-neutral-600 rounded-lg text-neutral-700 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    disabled={createLoading}
                    className="flex-1 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {createLoading ? "Creating..." : "Create App"}
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}
      </main>
    </div>
  )
}