"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import Link from "next/link"
import { api } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { toast, useToast } from "@/hooks/use-toast"
import { Loader2, Zap, Code, LayoutDashboard, Sparkles } from "lucide-react"
import { cn } from "@/lib/utils"

export default function Home() {
  const router = useRouter()
  const { toast } = useToast()
  const [email, setEmail] = useState("admin")
  const [password, setPassword] = useState("admin")
  const [loading, setLoading] = useState(false)

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    try {
      await api.login(email, password)
      toast({ title: "Welcome back!", description: "You have been logged in" })
      router.push("/dashboard")
    } catch (err: any) {
      toast({ title: "Login failed", description: err.response?.data?.error || "Invalid credentials", variant: "destructive" })
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-neutral-50 via-white to-primary-50 dark:from-neutral-950 dark:via-neutral-900 dark:to-neutral-950 flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <Link href="/" className="inline-flex items-center gap-2 mb-6">
            <div className="w-10 h-10 rounded-xl bg-primary-600 flex items-center justify-center">
              <Zap className="h-6 w-6 text-white" />
            </div>
            <span className="text-2xl font-bold text-neutral-900 dark:text-white">Appgent</span>
          </Link>
          <h1 className="text-3xl font-bold text-neutral-900 dark:text-white mb-2">Sign in to Appgent</h1>
          <p className="text-neutral-600 dark:text-neutral-400">
            Generate websites and PWAs with AI-powered multi-agent pipeline
          </p>
        </div>

        <Card className="shadow-xl border-neutral-200 dark:border-neutral-800">
          <CardHeader className="text-center">
            <CardTitle className="text-xl">Welcome back</CardTitle>
            <CardDescription>Enter your credentials to access your dashboard</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleLogin} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="email">Email</Label>
                <Input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="admin"
                  disabled={loading}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="admin"
                  disabled={loading}
                  required
                />
              </div>
              <Button type="submit" className="w-full" size="lg" disabled={loading}>
                {loading ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Signing in...
                  </>
                ) : (
                  "Sign in"
                )}
              </Button>
            </form>
          </CardContent>
          <CardFooter className="flex flex-col gap-4">
            <div className="grid grid-cols-3 gap-2 text-center">
              <div className="p-2 bg-primary-50 dark:bg-primary-900/30 rounded-lg">
                <Sparkles className="h-4 w-4 mx-auto text-primary-600 dark:text-primary-400 mb-1" />
                <p className="text-xs text-neutral-600 dark:text-neutral-400">Plan</p>
              </div>
              <div className="p-2 bg-purple-50 dark:bg-purple-900/30 rounded-lg">
                <Zap className="h-4 w-4 mx-auto text-purple-600 dark:text-purple-400 mb-1" />
                <p className="text-xs text-neutral-600 dark:text-neutral-400">Design</p>
              </div>
              <div className="p-2 bg-green-50 dark:bg-green-900/30 rounded-lg">
                <Code className="h-4 w-4 mx-auto text-green-600 dark:text-green-400 mb-1" />
                <p className="text-xs text-neutral-600 dark:text-neutral-400">Code</p>
              </div>
            </div>
          </CardFooter>
        </Card>

        <div className="mt-8 text-center">
          <p className="text-sm text-neutral-500 dark:text-neutral-400">
            Powered by <span className="font-medium text-primary-600 dark:text-primary-400">Nemotron 3 Ultra</span> via OpenRouter
          </p>
        </div>
      </div>
    </div>
  )
}