import { Shield, Server, Settings, Activity } from "lucide-react";
import Link from "next/link";

export default function Home() {
  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-900 to-slate-800">
      {/* Header */}
      <header className="border-b border-slate-700">
        <div className="container mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Shield className="h-8 w-8 text-sentinel-500" />
            <span className="text-xl font-bold text-white">Sentinel Hub</span>
          </div>
          <nav className="flex items-center gap-6">
            <Link
              href="/instances"
              className="text-slate-300 hover:text-white transition"
            >
              Instances
            </Link>
            <Link
              href="/configs"
              className="text-slate-300 hover:text-white transition"
            >
              Configs
            </Link>
            <Link
              href="/deployments"
              className="text-slate-300 hover:text-white transition"
            >
              Deployments
            </Link>
          </nav>
        </div>
      </header>

      {/* Hero */}
      <main className="container mx-auto px-4 py-16">
        <div className="text-center mb-16">
          <h1 className="text-4xl font-bold text-white mb-4">
            Fleet Management for Sentinel
          </h1>
          <p className="text-xl text-slate-400 max-w-2xl mx-auto">
            Configure, deploy, and monitor your Sentinel reverse proxy fleet
            from a single control plane.
          </p>
        </div>

        {/* Stats Grid */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-16">
          <StatCard
            icon={<Server className="h-8 w-8" />}
            title="Instances"
            value="0"
            subtitle="Online"
            href="/instances"
          />
          <StatCard
            icon={<Settings className="h-8 w-8" />}
            title="Configurations"
            value="0"
            subtitle="Active"
            href="/configs"
          />
          <StatCard
            icon={<Activity className="h-8 w-8" />}
            title="Deployments"
            value="0"
            subtitle="This week"
            href="/deployments"
          />
        </div>

        {/* Quick Actions */}
        <div className="bg-slate-800 rounded-lg p-8 border border-slate-700">
          <h2 className="text-xl font-semibold text-white mb-6">
            Getting Started
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <ActionCard
              title="Register an Instance"
              description="Connect a Sentinel proxy to this Hub"
              href="/instances/new"
            />
            <ActionCard
              title="Create Configuration"
              description="Define a new proxy configuration"
              href="/configs/new"
            />
          </div>
        </div>
      </main>

      {/* Footer */}
      <footer className="border-t border-slate-700 py-6">
        <div className="container mx-auto px-4 text-center text-slate-500 text-sm">
          Sentinel Hub — Fleet management control plane
        </div>
      </footer>
    </div>
  );
}

function StatCard({
  icon,
  title,
  value,
  subtitle,
  href,
}: {
  icon: React.ReactNode;
  title: string;
  value: string;
  subtitle: string;
  href: string;
}) {
  return (
    <Link
      href={href}
      className="bg-slate-800 rounded-lg p-6 border border-slate-700 hover:border-sentinel-500 transition group"
    >
      <div className="flex items-start justify-between">
        <div>
          <p className="text-slate-400 text-sm mb-1">{title}</p>
          <p className="text-3xl font-bold text-white">{value}</p>
          <p className="text-slate-500 text-sm">{subtitle}</p>
        </div>
        <div className="text-slate-600 group-hover:text-sentinel-500 transition">
          {icon}
        </div>
      </div>
    </Link>
  );
}

function ActionCard({
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
      className="block p-4 rounded-lg border border-slate-600 hover:border-sentinel-500 hover:bg-slate-700/50 transition"
    >
      <h3 className="text-white font-medium mb-1">{title}</h3>
      <p className="text-slate-400 text-sm">{description}</p>
    </Link>
  );
}
