"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { Server, FileCode2, Rocket, Activity, ArrowRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { StatusBadge } from "@/components/instances/status-badge";
import { listInstances, listConfigs, listDeployments, Instance } from "@/lib/api";
import { formatDistanceToNow } from "date-fns";

export default function DashboardPage() {
  const { data: instancesData, isLoading: instancesLoading } = useQuery({
    queryKey: ["instances"],
    queryFn: listInstances,
  });

  const { data: configsData, isLoading: configsLoading } = useQuery({
    queryKey: ["configs"],
    queryFn: listConfigs,
  });

  const { data: deploymentsData, isLoading: deploymentsLoading } = useQuery({
    queryKey: ["deployments"],
    queryFn: listDeployments,
  });

  const instances = instancesData?.instances ?? [];
  const configs = configsData?.configs ?? [];
  const deployments = deploymentsData?.deployments ?? [];

  const onlineInstances = instances.filter((i) => i.status === "online").length;
  const recentDeployments = deployments.slice(0, 5);

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
        <p className="text-muted-foreground">
          Fleet overview and recent activity
        </p>
      </div>

      {/* Stats Grid */}
      <div className="grid gap-4 md:grid-cols-3">
        <StatsCard
          icon={<Server className="h-5 w-5" />}
          title="Instances"
          value={instancesLoading ? null : instances.length.toString()}
          description={`${onlineInstances} online`}
          href="/instances"
        />
        <StatsCard
          icon={<FileCode2 className="h-5 w-5" />}
          title="Configurations"
          value={configsLoading ? null : configs.length.toString()}
          description="Active configs"
          href="/configs"
        />
        <StatsCard
          icon={<Rocket className="h-5 w-5" />}
          title="Deployments"
          value={deploymentsLoading ? null : deployments.length.toString()}
          description="Total deployments"
          href="/deployments"
        />
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        {/* Recent Instances */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle>Fleet Status</CardTitle>
              <CardDescription>Recently active instances</CardDescription>
            </div>
            <Button variant="ghost" size="sm" asChild>
              <Link href="/instances">
                View all
                <ArrowRight className="ml-2 h-4 w-4" />
              </Link>
            </Button>
          </CardHeader>
          <CardContent>
            {instancesLoading ? (
              <div className="space-y-3">
                {[...Array(3)].map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : instances.length === 0 ? (
              <div className="py-8 text-center">
                <Server className="mx-auto h-10 w-10 text-muted-foreground/50" />
                <p className="mt-2 text-sm text-muted-foreground">
                  No instances registered yet
                </p>
              </div>
            ) : (
              <div className="space-y-3">
                {instances.slice(0, 5).map((instance) => (
                  <InstanceRow key={instance.id} instance={instance} />
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Recent Deployments */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle>Recent Deployments</CardTitle>
              <CardDescription>Latest deployment activity</CardDescription>
            </div>
            <Button variant="ghost" size="sm" asChild>
              <Link href="/deployments">
                View all
                <ArrowRight className="ml-2 h-4 w-4" />
              </Link>
            </Button>
          </CardHeader>
          <CardContent>
            {deploymentsLoading ? (
              <div className="space-y-3">
                {[...Array(3)].map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : recentDeployments.length === 0 ? (
              <div className="py-8 text-center">
                <Activity className="mx-auto h-10 w-10 text-muted-foreground/50" />
                <p className="mt-2 text-sm text-muted-foreground">
                  No deployments yet
                </p>
              </div>
            ) : (
              <div className="space-y-3">
                {recentDeployments.map((deployment) => (
                  <div
                    key={deployment.id}
                    className="flex items-center justify-between rounded-lg border p-3"
                  >
                    <div>
                      <p className="font-medium text-sm">
                        Config v{deployment.configVersion}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {deployment.targetInstances.length} instance(s)
                      </p>
                    </div>
                    <div className="text-right">
                      <DeploymentStatusBadge status={deployment.status} />
                      <p className="text-xs text-muted-foreground mt-1">
                        {formatDistanceToNow(new Date(deployment.createdAt), {
                          addSuffix: true,
                        })}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Quick Actions */}
      <Card>
        <CardHeader>
          <CardTitle>Quick Actions</CardTitle>
          <CardDescription>Common tasks and workflows</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4 md:grid-cols-3">
            <QuickAction
              title="View Instances"
              description="Monitor your fleet status"
              href="/instances"
            />
            <QuickAction
              title="Configurations"
              description="Manage proxy configurations"
              href="/configs"
            />
            <QuickAction
              title="Deploy"
              description="Roll out configuration changes"
              href="/deployments"
            />
          </div>
        </CardContent>
      </Card>
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
  value: string | null;
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
          {value === null ? (
            <Skeleton className="h-8 w-16" />
          ) : (
            <div className="text-3xl font-bold">{value}</div>
          )}
          <p className="text-xs text-muted-foreground">{description}</p>
        </CardContent>
      </Card>
    </Link>
  );
}

function InstanceRow({ instance }: { instance: Instance }) {
  return (
    <Link href={`/instances/${instance.id}`}>
      <div className="flex items-center justify-between rounded-lg border p-3 hover:bg-accent/50 transition-colors">
        <div>
          <p className="font-medium text-sm">{instance.name}</p>
          <p className="text-xs text-muted-foreground font-mono">
            {instance.hostname}
          </p>
        </div>
        <StatusBadge status={instance.status} />
      </div>
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

function DeploymentStatusBadge({
  status,
}: {
  status: "pending" | "in-progress" | "completed" | "failed" | "cancelled";
}) {
  const variants = {
    pending: "bg-gray-500/10 text-gray-700 border-gray-500",
    "in-progress": "bg-blue-500/10 text-blue-700 border-blue-500",
    completed: "bg-green-500/10 text-green-700 border-green-500",
    failed: "bg-red-500/10 text-red-700 border-red-500",
    cancelled: "bg-yellow-500/10 text-yellow-700 border-yellow-500",
  };

  return (
    <span
      className={`inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${variants[status]}`}
    >
      {status}
    </span>
  );
}
