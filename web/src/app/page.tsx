import { Shield, Server, Settings, Activity, Plus } from "lucide-react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

export default function Home() {
  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
      <header className="border-b">
        <div className="container mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Shield className="h-8 w-8 text-primary" />
            <span className="text-xl font-bold">Sentinel Hub</span>
          </div>
          <nav className="flex items-center gap-6">
            <Link
              href="/instances"
              className="text-muted-foreground hover:text-foreground transition"
            >
              Instances
            </Link>
            <Link
              href="/configs"
              className="text-muted-foreground hover:text-foreground transition"
            >
              Configs
            </Link>
            <Link
              href="/deployments"
              className="text-muted-foreground hover:text-foreground transition"
            >
              Deployments
            </Link>
            <Button size="sm">
              <Plus className="h-4 w-4" />
              New Config
            </Button>
          </nav>
        </div>
      </header>

      {/* Main Content */}
      <main className="container mx-auto px-4 py-8">
        {/* Hero */}
        <div className="text-center mb-12">
          <h1 className="text-4xl font-bold mb-4">Fleet Management for Sentinel</h1>
          <p className="text-xl text-muted-foreground max-w-2xl mx-auto">
            Configure, deploy, and monitor your Sentinel reverse proxy fleet from
            a single control plane.
          </p>
        </div>

        {/* Stats Grid */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-12">
          <StatsCard
            icon={<Server className="h-5 w-5" />}
            title="Instances"
            value="0"
            description="Online"
            href="/instances"
          />
          <StatsCard
            icon={<Settings className="h-5 w-5" />}
            title="Configurations"
            value="0"
            description="Active"
            href="/configs"
          />
          <StatsCard
            icon={<Activity className="h-5 w-5" />}
            title="Deployments"
            value="0"
            description="This week"
            href="/deployments"
          />
        </div>

        {/* Quick Actions */}
        <Card>
          <CardHeader>
            <CardTitle>Getting Started</CardTitle>
            <CardDescription>
              Set up your first Sentinel instance and configuration
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <QuickAction
                title="Register an Instance"
                description="Connect a Sentinel proxy to this Hub"
                href="/instances/new"
              />
              <QuickAction
                title="Create Configuration"
                description="Define a new proxy configuration"
                href="/configs/new"
              />
            </div>
          </CardContent>
        </Card>

        {/* Recent Activity */}
        <div className="mt-8">
          <h2 className="text-xl font-semibold mb-4">Recent Activity</h2>
          <Card>
            <CardContent className="pt-6">
              <div className="text-center py-8 text-muted-foreground">
                <Activity className="h-12 w-12 mx-auto mb-4 opacity-50" />
                <p>No recent activity</p>
                <p className="text-sm">
                  Activity will appear here once you start managing your fleet
                </p>
              </div>
            </CardContent>
          </Card>
        </div>
      </main>

      {/* Footer */}
      <footer className="border-t py-6 mt-12">
        <div className="container mx-auto px-4 text-center text-muted-foreground text-sm">
          Sentinel Hub — Fleet management control plane
        </div>
      </footer>
    </div>
  );
}

function StatsCard({
  icon,
  title,
  value,
  description,
  href,
}: {
  icon: React.ReactNode;
  title: string;
  value: string;
  description: string;
  href: string;
}) {
  return (
    <Link href={href}>
      <Card className="hover:border-primary/50 transition-colors cursor-pointer">
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {title}
          </CardTitle>
          <div className="text-muted-foreground">{icon}</div>
        </CardHeader>
        <CardContent>
          <div className="text-3xl font-bold">{value}</div>
          <p className="text-xs text-muted-foreground">{description}</p>
        </CardContent>
      </Card>
    </Link>
  );
}

function QuickAction({
  title,
  description,
  href,
}: {
  title: string;
  description: string;
  href: string;
}) {
  return (
    <Link
      href={href}
      className="block p-4 rounded-lg border hover:border-primary/50 hover:bg-accent/50 transition-colors"
    >
      <h3 className="font-medium mb-1">{title}</h3>
      <p className="text-sm text-muted-foreground">{description}</p>
    </Link>
  );
}
