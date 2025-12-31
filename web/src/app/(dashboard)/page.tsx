"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import {
  Server,
  FileCode2,
  Rocket,
  Activity,
  ArrowRight,
  AlertTriangle,
  RefreshCw,
} from "lucide-react";

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
import { DeploymentStatusBadge } from "@/components/deployments/deployment-status-badge";
import { MetricsSummaryCards } from "@/components/metrics/metrics-summary";
import { MetricsChart, MultiLineChart } from "@/components/metrics/metrics-chart";
import { PeriodSelector, Period } from "@/components/metrics/period-selector";
import {
  listInstances,
  listConfigs,
  listDeployments,
  getFleetMetrics,
  listAlerts,
  Instance,
  TimeSeriesPoint,
} from "@/lib/api";
import { formatDistanceToNow } from "date-fns";

export default function DashboardPage() {
  const [period, setPeriod] = useState<Period>("1h");

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

  const {
    data: metricsData,
    isLoading: metricsLoading,
    refetch: refetchMetrics,
    isFetching: metricsFetching,
  } = useQuery({
    queryKey: ["fleet-metrics", period],
    queryFn: () => getFleetMetrics({ period }),
    refetchInterval: 30000, // Refresh every 30 seconds
  });

  const { data: alertsData } = useQuery({
    queryKey: ["alerts"],
    queryFn: listAlerts,
  });

  const instances = instancesData?.instances ?? [];
  const configs = configsData?.configs ?? [];
  const deployments = deploymentsData?.deployments ?? [];
  const alerts = alertsData?.alerts ?? [];

  const onlineInstances = instances.filter((i) => i.status === "online").length;
  const recentDeployments = deployments.slice(0, 5);
  const firingAlerts = alerts.filter((a) => a.state === "firing");

  // Combine latency time series for multi-line chart
  const latencyData = metricsData
    ? combineTimeSeries({
        p50: metricsData.timeSeries.latencyP50,
        p95: metricsData.timeSeries.latencyP95,
        p99: metricsData.timeSeries.latencyP99,
      })
    : [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
          <p className="text-muted-foreground">
            Fleet overview and performance metrics
          </p>
        </div>
        <div className="flex items-center gap-2">
          <PeriodSelector value={period} onChange={setPeriod} />
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetchMetrics()}
            disabled={metricsFetching}
          >
            <RefreshCw
              className={`h-4 w-4 ${metricsFetching ? "animate-spin" : ""}`}
            />
          </Button>
        </div>
      </div>

      {/* Alerts Banner */}
      {firingAlerts.length > 0 && (
        <div className="rounded-lg border border-red-500 bg-red-500/10 p-4">
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-red-500" />
            <span className="font-medium text-red-500">
              {firingAlerts.length} active alert{firingAlerts.length > 1 ? "s" : ""}
            </span>
          </div>
          <div className="mt-2 space-y-1">
            {firingAlerts.slice(0, 3).map((alert) => (
              <p key={alert.id} className="text-sm text-red-500/80">
                {alert.name}: {alert.description}
              </p>
            ))}
          </div>
        </div>
      )}

      {/* Metrics Summary */}
      {metricsLoading ? (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-28" />
          ))}
        </div>
      ) : metricsData ? (
        <MetricsSummaryCards data={metricsData.summary} />
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <MetricsPlaceholderCard title="Requests/sec" />
          <MetricsPlaceholderCard title="Error Rate" />
          <MetricsPlaceholderCard title="P95 Latency" />
          <MetricsPlaceholderCard title="Throughput" />
        </div>
      )}

      {/* Charts Grid */}
      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Request Rate</CardTitle>
            <CardDescription>Requests per second over time</CardDescription>
          </CardHeader>
          <CardContent>
            {metricsLoading ? (
              <Skeleton className="h-[200px]" />
            ) : metricsData ? (
              <MetricsChart
                data={metricsData.timeSeries.requests}
                color="hsl(var(--primary))"
                formatValue={(v) => `${v.toFixed(0)}/s`}
                height={200}
              />
            ) : (
              <ChartPlaceholder />
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Error Rate</CardTitle>
            <CardDescription>Errors per second over time</CardDescription>
          </CardHeader>
          <CardContent>
            {metricsLoading ? (
              <Skeleton className="h-[200px]" />
            ) : metricsData ? (
              <MetricsChart
                data={metricsData.timeSeries.errors}
                color="hsl(var(--destructive))"
                formatValue={(v) => `${v.toFixed(0)}/s`}
                height={200}
              />
            ) : (
              <ChartPlaceholder />
            )}
          </CardContent>
        </Card>

        <Card className="md:col-span-2">
          <CardHeader>
            <CardTitle className="text-base">Latency Percentiles</CardTitle>
            <CardDescription>P50, P95, and P99 response times</CardDescription>
          </CardHeader>
          <CardContent>
            {metricsLoading ? (
              <Skeleton className="h-[250px]" />
            ) : metricsData ? (
              <MultiLineChart
                data={latencyData}
                lines={[
                  { dataKey: "p50", color: "hsl(142, 76%, 36%)", name: "P50" },
                  { dataKey: "p95", color: "hsl(48, 96%, 53%)", name: "P95" },
                  { dataKey: "p99", color: "hsl(0, 84%, 60%)", name: "P99" },
                ]}
                formatValue={(v) => `${v.toFixed(0)}ms`}
                height={250}
              />
            ) : (
              <ChartPlaceholder height={250} />
            )}
          </CardContent>
        </Card>
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

function MetricsPlaceholderCard({ title }: { title: string }) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-bold text-muted-foreground">--</div>
        <p className="text-xs text-muted-foreground">No data available</p>
      </CardContent>
    </Card>
  );
}

function ChartPlaceholder({ height = 200 }: { height?: number }) {
  return (
    <div
      className="flex items-center justify-center rounded-lg border border-dashed"
      style={{ height }}
    >
      <p className="text-sm text-muted-foreground">No metrics data available</p>
    </div>
  );
}

function combineTimeSeries(
  series: Record<string, TimeSeriesPoint[]>
): Array<Record<string, string | number>> {
  const keys = Object.keys(series);
  if (keys.length === 0) return [];

  const firstSeries = series[keys[0]];
  return firstSeries.map((point, index) => {
    const combined: Record<string, string | number> = {
      timestamp: point.timestamp,
    };
    keys.forEach((key) => {
      combined[key] = series[key][index]?.value ?? 0;
    });
    return combined;
  });
}
